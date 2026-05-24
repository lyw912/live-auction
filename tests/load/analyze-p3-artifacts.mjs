import { existsSync, readFileSync, readdirSync, statSync, writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../..', import.meta.url));
const rawRoot = resolve(root, process.argv[2] || 'docs/perf/raw');

function readJSON(path) {
  return JSON.parse(readFileSync(path, 'utf8'));
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
  if (workload.environment_signals?.possible_env_limit) return 'ENV_LIMIT';
  if (workload.k6?.dropped_iterations > 0) return 'ENV_LIMIT';
  return 'NO_REGRESSION_OR_NEEDS_DOMAIN_METRICS';
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
      iterations: workload.k6?.iterations || 0,
      dropped_iterations: workload.k6?.dropped_iterations || 0,
      p99_ms: workload.k6?.http_req_duration_ms?.p99 || 0,
      checks_rate: workload.k6?.checks?.rate || 0,
      env_signals: workload.environment_signals?.signals || [],
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
  console.log(`${workload.workload}: ${workload.status} ${workload.verdict_hint} p99=${workload.p99_ms} dropped=${workload.dropped_iterations} signals=${signals}`);
}
