import { existsSync, readFileSync, readdirSync, statSync, writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../..', import.meta.url));
const rawRoot = resolve(root, process.argv[2] || 'artifacts/perf/raw');

function readJSON(path) {
  return JSON.parse(readFileSync(path, 'utf8'));
}

function metricValues(metric) {
  return metric?.values || metric || {};
}

function metricCount(metric) {
  const values = metricValues(metric);
  const value = values.count ?? values.passes;
  return Number.isFinite(Number(value)) ? Number(value) : 0;
}

function findReports(dir) {
  if (!existsSync(dir)) return [];
  const out = [];
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry);
    const stat = statSync(full);
    if (stat.isDirectory()) {
      const report = join(full, 'analysis-compact.json');
      if (existsSync(report)) out.push(report);
    }
  }
  return out.sort();
}

function workloadKey(workload) {
  return workload.workload || '';
}

function classify(workload) {
  if (workload.status !== 'PASS') return 'HARNESS_GAP';
  if (workload.admission_proof?.polluted) return 'HARNESS_GAP';
  if (workload.environment_signals?.possible_env_limit) return 'ENV_LIMIT';
  if (workload.k6?.dropped_iterations > 0) return 'ENV_LIMIT';
  return 'NO_REGRESSION_OR_NEEDS_DOMAIN_METRICS';
}

function subsystemHint(workload) {
  const name = workloadKey(workload);
  if (name.includes('outbox') || name.includes('relay')) return 'outbox';
  if (name.includes('bid') || name.includes('final-second')) return 'pg-hot-row';
  if (name.includes('fanout') || name.includes('healthy-vs-slow') || name.includes('slow-consumer')) return 'websocket-fanout';
  if (name.includes('connection-storm')) return 'environment-or-ws-admission';
  if (name.includes('reconnect')) return 'reconnect-snapshot';
  if (name.includes('multi-room')) return 'room-isolation';
  if (name.includes('abuse')) return 'admission-protection';
  return 'unknown';
}

function artifactHints(workload) {
  const verdict = classify(workload);
  const subsystem = subsystemHint(workload);
  if (verdict === 'ENV_LIMIT') {
    return ['compact report', 'k6 summary dropped_iterations/vus_max', 'workload log excerpt', 'Windows/Docker resource check'];
  }
  if (verdict === 'HARNESS_GAP') {
    return ['compact report', 'admission_proof reject_delta', 'failed workload log if present', 'readyz dump', 'before/after metrics'];
  }
  if (subsystem === 'pg-hot-row') {
    return ['compact report', 'before/after metrics', 'pg_locks/pg_stat_activity only if p99 or retry-later moved'];
  }
  if (subsystem === 'outbox') {
    return ['compact report', 'before/after metrics', 'outbox backlog/lag query', 'claim EXPLAIN only if backlog grows'];
  }
  if (subsystem === 'websocket-fanout') {
    return ['compact report', 'before/after metrics', 'WS counters/goroutine/heap only if fanout or slow-close moved'];
  }
  if (subsystem === 'reconnect-snapshot') {
    return ['compact report', 'before/after metrics', 'snapshot source/rebuild counters only if recovery degraded'];
  }
  if (subsystem === 'admission-protection') {
    return ['compact report', 'admission counters', '429/retry-after distribution'];
  }
  return ['compact report first'];
}

function needsFullDrilldown(workload) {
  if (workload.status !== 'PASS') return true;
  if (workload.environment_signals?.possible_env_limit) return false;
  const p99 = workload.k6?.http_req_duration_ms?.p99 || 0;
  const dropped = workload.k6?.dropped_iterations || 0;
  return dropped === 0 && p99 > 1000;
}

function summarizeReport(path) {
  const report = readJSON(path);
  return {
    path,
    generated_at: report.generated_at || '',
    profile: report.profile || '',
    admission_enabled: report.admission_enabled || '',
    artifact_mode: report.artifact_mode || '',
    workloads: (report.workloads || []).map((workload) => ({
      workload: workloadKey(workload),
      status: workload.status,
      verdict_hint: classify(workload),
      subsystem_hint: subsystemHint(workload),
      iterations: workload.k6?.iterations || 0,
      dropped_iterations: workload.k6?.dropped_iterations || 0,
      p99_ms: workload.k6?.http_req_duration_ms?.p99 || 0,
      checks_rate: workload.k6?.checks?.rate || 0,
      admission_proof: workload.admission_proof || {},
      env_signals: workload.environment_signals?.signals || [],
      next_artifacts: artifactHints(workload),
      needs_full_drilldown: needsFullDrilldown(workload),
      error: workload.validationError || '',
    })),
  };
}

function latestByWorkload(summaries) {
  const out = new Map();
  for (const summary of summaries) {
    for (const workload of summary.workloads) {
      const prev = out.get(workload.workload);
      if (!prev || summary.generated_at >= prev.generated_at) {
        out.set(workload.workload, { generated_at: summary.generated_at, ...workload });
      }
    }
  }
  return [...out.values()].sort((a, b) => a.workload.localeCompare(b.workload));
}

const reports = findReports(rawRoot);
const summaries = reports.map(summarizeReport);
const latest = latestByWorkload(summaries);
const output = {
  raw_root: rawRoot,
  report_count: reports.length,
  generated_at: new Date().toISOString(),
  latest_by_workload: latest,
};

const outPath = join(rawRoot, 'p3-artifact-index.json');
writeFileSync(outPath, JSON.stringify(output, null, 2));

console.log(`reports=${reports.length}`);
console.log(`index=${outPath}`);
for (const workload of latest) {
  const signals = workload.env_signals.length > 0 ? workload.env_signals.join(',') : '-';
  const full = workload.needs_full_drilldown ? ' full=maybe' : ' full=no';
  console.log(`${workload.workload}: ${workload.status} ${workload.verdict_hint} subsystem=${workload.subsystem_hint} p99=${workload.p99_ms} dropped=${workload.dropped_iterations} signals=${signals}${full}`);
  console.log(`  read: ${workload.next_artifacts.join('; ')}`);
}
