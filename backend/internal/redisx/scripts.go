package redisx

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"live-auction/backend/internal/observability"
)

const (
	ScriptBidAdmissionGCRA     = "bid_admission_gcra"
	ScriptBidRedisGuard        = "bid_redis_guard"
	ScriptBidRedisGuardRefresh = "bid_redis_guard_refresh"
	ScriptBidRedisLedger       = "bid_redis_ledger"
	ScriptWSTicketConsume      = "ws_ticket_consume"
)

const (
	OutcomeAllowed     = "allowed"
	OutcomeRejected    = "rejected"
	OutcomeConsumed    = "consumed"
	OutcomeMissing     = "missing"
	OutcomeError       = "error"
	OutcomeParseError  = "parse_error"
	OutcomeUnavailable = "unavailable"
	OutcomeTimeout     = "timeout"
	OutcomeBusy        = "busy"
	OutcomeNoScript    = "noscript"
)

type ScriptRunner struct {
	name   string
	script *redis.Script
}

func NewScriptRunner(name string, source string) ScriptRunner {
	return ScriptRunner{name: name, script: redis.NewScript(source)}
}

func (r ScriptRunner) Hash() string {
	return r.script.Hash()
}

func (r ScriptRunner) Name() string {
	return r.name
}

func (r ScriptRunner) Load(ctx context.Context, client redis.Scripter) error {
	start := time.Now()
	err := r.script.Load(ctx, client).Err()
	if err != nil {
		recordScript(r.name, ClassifyScriptError(err), time.Since(start))
		return err
	}
	recordScript(r.name, "loaded", time.Since(start))
	return nil
}

func (r ScriptRunner) Run(ctx context.Context, client redis.Scripter, keys []string, args ...interface{}) *redis.Cmd {
	start := time.Now()
	cmd := r.script.Run(ctx, client, keys, args...)
	if err := cmd.Err(); err != nil {
		recordScript(r.name, ClassifyScriptError(err), time.Since(start))
	}
	return cmd
}

func (r ScriptRunner) Record(outcome string, elapsed time.Duration) {
	recordScript(r.name, outcome, elapsed)
}

func ClassifyScriptError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return OutcomeTimeout
	}
	if errors.Is(err, redis.Nil) {
		return OutcomeMissing
	}
	msg := strings.ToUpper(err.Error())
	switch {
	case strings.Contains(msg, "BUSY"):
		return OutcomeBusy
	case strings.Contains(msg, "NOSCRIPT"):
		return OutcomeNoScript
	case strings.Contains(msg, "I/O TIMEOUT"),
		strings.Contains(msg, "CONNECTION REFUSED"),
		strings.Contains(msg, "NO SUCH HOST"),
		strings.Contains(msg, "NETWORK IS UNREACHABLE"),
		strings.Contains(msg, "CONNECTION RESET"):
		return OutcomeUnavailable
	case strings.Contains(msg, "CONTEXT DEADLINE EXCEEDED"):
		return OutcomeTimeout
	default:
		return OutcomeError
	}
}

func recordScript(scriptName string, outcome string, elapsed time.Duration) {
	observability.Inc("redis_lua_script_total", map[string]string{"script": scriptName, "outcome": outcome})
	observability.Observe("redis_lua_script_latency_seconds", elapsed.Seconds(), map[string]string{"script": scriptName, "outcome": outcome}, observability.DefaultLatencyBuckets)
}
