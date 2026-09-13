package hws

import "context"

// ModelUsage separates reservations, known measurements, and uncertain attempts.
// Reserved ceilings are cumulative across restarts and never refunded. Known
// usage is a lower bound: malformed/outage/crashed and pre-v6 calls may be billed.
type ModelUsage struct {
	ReservedTokens         int64 `json:"reserved_tokens"`
	ReservedSpendMicros    int64 `json:"reserved_spend_micros"`
	KnownInputTokens       int64 `json:"known_input_tokens"`
	KnownOutputTokens      int64 `json:"known_output_tokens"`
	KnownAttempts          int64 `json:"known_attempts"`
	SettledUnknownAttempts int64 `json:"settled_unknown_attempts"`
	RunningAttempts        int64 `json:"running_requests"`
	UncertainRequests      int64 `json:"uncertain_requests"`
}

type ModelUsageReader interface {
	ReadModelUsage(context.Context, Scope) (ModelUsage, error)
}
