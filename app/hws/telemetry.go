package hws

import "time"

// Operational telemetry accepts only closed enums and durations. Audit data,
// prompts, identifiers, URLs, errors and context baggage are not accepted here.
type OperationKind uint8
type OperationalOutcome uint8

const (
	APIRequest OperationKind = iota
	ProviderCall
	CognitiveCommit
	RecoveryCycle
	OperationKinds
)
const (
	OperationalSuccess OperationalOutcome = iota
	BehavioralWait
	ProviderOutage
	SafetyDenied
	BudgetDenied
	OperationalUncertain
	OperationalFailure
	OperationalOutcomes
)

type OperationalObserver interface {
	Observe(OperationKind, OperationalOutcome, time.Duration)
}
