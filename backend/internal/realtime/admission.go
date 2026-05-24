package realtime

import (
	"net/http"
	"strconv"
	"time"

	"live-auction/backend/internal/observability"
)

type Admission struct {
	ticketInFlight  chan struct{}
	connectInFlight chan struct{}
	retryAfter      time.Duration
}

func NewAdmission(ticketMaxInFlight int, connectMaxInFlight int, retryAfter time.Duration) *Admission {
	if ticketMaxInFlight < 0 {
		ticketMaxInFlight = 0
	}
	if connectMaxInFlight < 0 {
		connectMaxInFlight = 0
	}
	if retryAfter <= 0 {
		retryAfter = time.Second
	}
	return &Admission{
		ticketInFlight:  make(chan struct{}, ticketMaxInFlight),
		connectInFlight: make(chan struct{}, connectMaxInFlight),
		retryAfter:      retryAfter,
	}
}

func (a *Admission) TryTicket() (func(), bool) {
	return a.try(a.ticketInFlight, "ticket")
}

func (a *Admission) TryConnect() (func(), bool) {
	return a.try(a.connectInFlight, "connect")
}

func (a *Admission) try(ch chan struct{}, stage string) (func(), bool) {
	if a == nil || cap(ch) == 0 {
		return func() {}, true
	}
	select {
	case ch <- struct{}{}:
		observability.AddGauge("auction_ws_admission_in_flight", 1, map[string]string{"stage": stage})
		return func() {
			<-ch
			observability.AddGauge("auction_ws_admission_in_flight", -1, map[string]string{"stage": stage})
		}, true
	default:
		observability.Inc("auction_ws_admission_rejected_total", map[string]string{"stage": stage})
		return nil, false
	}
}

func (a *Admission) RetryAfterHeader() string {
	if a == nil || a.retryAfter <= 0 {
		return "1"
	}
	seconds := int(a.retryAfter.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	return strconv.Itoa(seconds)
}

func (a *Admission) WriteRejected(w http.ResponseWriter) {
	w.Header().Set("Retry-After", a.RetryAfterHeader())
	http.Error(w, "ws admission retry later", http.StatusTooManyRequests)
}
