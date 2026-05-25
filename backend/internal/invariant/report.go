package invariant

import "time"

type Status string

const (
	StatusPass Status = "PASS"
	StatusWarn Status = "WARN"
	StatusFail Status = "FAIL"
	StatusSkip Status = "SKIP"
)

type Severity string

const (
	SeverityP0 Severity = "P0"
	SeverityP1 Severity = "P1"
	SeverityP2 Severity = "P2"
)

type Scope struct {
	AuctionID string `json:"auction_id,omitempty"`
	RoomID    string `json:"room_id,omitempty"`
}

type Options struct {
	AuctionID  string
	RoomID     string
	MaxDetails int
	Now        time.Time
}

type Report struct {
	Status      Status        `json:"status"`
	GeneratedAt time.Time     `json:"generated_at"`
	Scope       Scope         `json:"scope"`
	Summary     Summary       `json:"summary"`
	Checks      []CheckResult `json:"checks"`
}

type Summary struct {
	Passed  int `json:"passed"`
	Warned  int `json:"warned"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}

type CheckResult struct {
	Name        string            `json:"name"`
	Severity    Severity          `json:"severity"`
	Status      Status            `json:"status"`
	Count       int               `json:"count"`
	Description string            `json:"description"`
	Details     []ViolationDetail `json:"details,omitempty"`
}

type ViolationDetail map[string]any

func (r *Report) add(check CheckResult) {
	r.Checks = append(r.Checks, check)
	switch check.Status {
	case StatusPass:
		r.Summary.Passed++
	case StatusWarn:
		r.Summary.Warned++
	case StatusFail:
		r.Summary.Failed++
	case StatusSkip:
		r.Summary.Skipped++
	}
	if check.Status == StatusFail {
		r.Status = StatusFail
		return
	}
	if check.Status == StatusWarn && r.Status != StatusFail {
		r.Status = StatusWarn
		return
	}
	if r.Status == "" {
		r.Status = StatusPass
	}
}

func failOrPass(name string, severity Severity, description string, count int, details []ViolationDetail) CheckResult {
	status := StatusPass
	if count > 0 {
		status = StatusFail
	}
	return CheckResult{
		Name:        name,
		Severity:    severity,
		Status:      status,
		Count:       count,
		Description: description,
		Details:     details,
	}
}

func warnOrPass(name string, severity Severity, description string, count int, details []ViolationDetail) CheckResult {
	status := StatusPass
	if count > 0 {
		status = StatusWarn
	}
	return CheckResult{
		Name:        name,
		Severity:    severity,
		Status:      status,
		Count:       count,
		Description: description,
		Details:     details,
	}
}
