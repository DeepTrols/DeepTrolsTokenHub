// PostgreSQL persistence for guardrail policies (pgx, no GORM).
package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/deeptrols/api/internal/guardrails"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository loads guardrail policies with their detection items and
// bindings. The engine filters policies by project/checkpoint/protocol via the
// bindings, so LoadPolicies returns every persisted policy.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) LoadPolicies(ctx context.Context) ([]guardrails.Policy, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, description, status, config_version, created_at, updated_at
		 FROM guardrail_policies ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("guardrails load policies: %w", err)
	}
	defer rows.Close()

	policies := make([]guardrails.Policy, 0)
	byID := map[string]*guardrails.Policy{}
	for rows.Next() {
		var p guardrails.Policy
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Status,
			&p.ConfigVersion, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.DetectionItems = []guardrails.DetectionItem{}
		p.Bindings = []guardrails.Binding{}
		policies = append(policies, p)
		byID[p.ID] = &policies[len(policies)-1]
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(policies) == 0 {
		return policies, nil
	}

	if err := r.loadItems(ctx, byID); err != nil {
		return nil, err
	}
	if err := r.loadBindings(ctx, byID); err != nil {
		return nil, err
	}
	return policies, nil
}

func (r *PostgresRepository) loadItems(ctx context.Context, byID map[string]*guardrails.Policy) error {
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, policy_id, name, detector_type, action, config_version, config, created_at, updated_at
		 FROM guardrail_detection_items WHERE policy_id = ANY($1) ORDER BY created_at ASC`, ids)
	if err != nil {
		return fmt.Errorf("guardrails load items: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item guardrails.DetectionItem
		var configJSON []byte
		if err := rows.Scan(&item.ID, &item.PolicyID, &item.Name, &item.DetectorType,
			&item.Action, &item.ConfigVersion, &configJSON, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return err
		}
		if err := json.Unmarshal(configJSON, &item.Config); err != nil {
			item.Config = map[string]any{}
		}
		if p, ok := byID[item.PolicyID]; ok {
			p.DetectionItems = append(p.DetectionItems, item)
		}
	}
	return rows.Err()
}

func (r *PostgresRepository) loadBindings(ctx context.Context, byID map[string]*guardrails.Policy) error {
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, policy_id, scope_type, scope_id, checkpoint, protocol, config_version, created_at, updated_at
		 FROM guardrail_policy_bindings WHERE policy_id = ANY($1) ORDER BY created_at ASC`, ids)
	if err != nil {
		return fmt.Errorf("guardrails load bindings: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var b guardrails.Binding
		if err := rows.Scan(&b.ID, &b.PolicyID, &b.ScopeType, &b.ScopeID,
			&b.Checkpoint, &b.Protocol, &b.ConfigVersion, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return err
		}
		if p, ok := byID[b.PolicyID]; ok {
			p.Bindings = append(p.Bindings, b)
		}
	}
	return rows.Err()
}

// SavePolicy upserts a policy with its detection items and bindings in one
// transaction. Exists so the Phase 1 admin editor can persist policies.
func (r *PostgresRepository) SavePolicy(ctx context.Context, policy guardrails.Policy) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if policy.CreatedAt.IsZero() {
		policy.CreatedAt = time.Now().UTC()
	}
	policy.UpdatedAt = time.Now().UTC()
	_, err = tx.Exec(ctx,
		`INSERT INTO guardrail_policies (id, name, description, status, config_version, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)
		 ON CONFLICT (id) DO UPDATE SET
		   name=EXCLUDED.name, description=EXCLUDED.description, status=EXCLUDED.status,
		   config_version=EXCLUDED.config_version, updated_at=EXCLUDED.updated_at`,
		policy.ID, policy.Name, policy.Description, policy.Status,
		policy.ConfigVersion, policy.CreatedAt, policy.UpdatedAt)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM guardrail_detection_items WHERE policy_id = $1`, policy.ID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM guardrail_policy_bindings WHERE policy_id = $1`, policy.ID); err != nil {
		return err
	}
	for _, item := range policy.DetectionItems {
		configJSON, err := json.Marshal(item.Config)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO guardrail_detection_items (id, policy_id, name, detector_type, action, config_version, config, created_at, updated_at)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			item.ID, policy.ID, item.Name, item.DetectorType, item.Action,
			item.ConfigVersion, configJSON, policy.CreatedAt, policy.UpdatedAt); err != nil {
			return err
		}
	}
	for _, b := range policy.Bindings {
		if _, err := tx.Exec(ctx,
			`INSERT INTO guardrail_policy_bindings (id, policy_id, scope_type, scope_id, checkpoint, protocol, config_version, created_at, updated_at)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			b.ID, policy.ID, b.ScopeType, b.ScopeID, b.Checkpoint, b.Protocol,
			b.ConfigVersion, policy.CreatedAt, policy.UpdatedAt); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) DeletePolicy(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM guardrail_policies WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("guardrail policy not found")
	}
	return nil
}
