package guardrails

import "context"

// PolicySource loads persisted guardrail policies for gateway evaluation.
type PolicySource interface {
	LoadPolicies(ctx context.Context) ([]Policy, error)
}

// PolicyManager adds persistence operations for the admin editor.
type PolicyManager interface {
	PolicySource
	SavePolicy(ctx context.Context, policy Policy) error
	DeletePolicy(ctx context.Context, id string) error
}
