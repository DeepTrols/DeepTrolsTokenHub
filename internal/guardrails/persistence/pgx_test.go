package persistence

import (
	"context"
	"testing"

	"github.com/deeptrols/api/internal/guardrails"
	"github.com/deeptrols/api/internal/repository/testutil"
)

func TestPostgresRepository_LoadPoliciesWithItemsAndBindings(t *testing.T) {
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)
	ctx := context.Background()
	repo := NewPostgresRepository(pool)

	policy := guardrails.Policy{
		ID:            "policy-1",
		Name:          "敏感词拦截",
		Status:        guardrails.StatusActive,
		ConfigVersion: guardrails.CurrentConfigVersion,
		DetectionItems: []guardrails.DetectionItem{
			{
				ID:            "item-1",
				PolicyID:      "policy-1",
				Name:          "测试敏感词",
				DetectorType:  guardrails.DetectorPattern,
				Action:        guardrails.ActionBlock,
				ConfigVersion: guardrails.CurrentConfigVersion,
				Config: map[string]any{
					"keywords": []any{"机密", "secret"},
				},
			},
		},
		Bindings: []guardrails.Binding{
			{
				ID:            "binding-1",
				PolicyID:      "policy-1",
				ScopeType:     guardrails.ScopeAllProjects,
				Checkpoint:    guardrails.CheckpointBeforeProvider,
				Protocol:      guardrails.ProtocolAll,
				ConfigVersion: guardrails.CurrentConfigVersion,
			},
		},
	}
	normalized, err := guardrails.NormalizePolicy(policy)
	if err != nil {
		t.Fatalf("normalize policy: %v", err)
	}
	if err := repo.SavePolicy(ctx, normalized); err != nil {
		t.Fatalf("save policy: %v", err)
	}

	policies, err := repo.LoadPolicies(ctx)
	if err != nil {
		t.Fatalf("load policies: %v", err)
	}
	if len(policies) != 1 {
		t.Fatalf("policies = %d, want 1", len(policies))
	}
	got := policies[0]
	if len(got.DetectionItems) != 1 || len(got.Bindings) != 1 {
		t.Fatalf("policy items=%d bindings=%d, want 1/1",
			len(got.DetectionItems), len(got.Bindings))
	}
	// The loaded policy must actually block a matching fragment.
	engine := guardrails.NewEngine(nil)
	decision, err := engine.Evaluate(ctx, guardrails.EvaluationRequest{
		ProjectID:  "project-1",
		Checkpoint: guardrails.CheckpointBeforeProvider,
		Protocol:   guardrails.ProtocolAll,
		Fragments:  []guardrails.Fragment{{ID: "f1", Text: "这是机密内容"}},
		Policies:   policies,
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if decision.Action != guardrails.ActionBlock {
		t.Errorf("action = %s, want block", decision.Action)
	}

	if err := repo.DeletePolicy(ctx, "policy-1"); err != nil {
		t.Fatalf("delete policy: %v", err)
	}
	after, err := repo.LoadPolicies(ctx)
	if err != nil {
		t.Fatalf("load after delete: %v", err)
	}
	if len(after) != 0 {
		t.Errorf("policies after delete = %d, want 0", len(after))
	}
}
