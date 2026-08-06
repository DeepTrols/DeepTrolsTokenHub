package channel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/deeptrols/api/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository implements Repository with PostgreSQL via pgx/v5.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository creates a new PostgresRepository.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

var _ Repository = (*PostgresRepository)(nil)

// ListByModel returns channels for a model, optionally filtered by tenant.
func (r *PostgresRepository) ListByModel(ctx context.Context, modelID uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
	query := "SELECT " + channelSelectClause + " FROM channels WHERE model_id = $1"
	args := []any{modelID}
	if tenantID != nil {
		query += " AND tenant_id = $2"
		args = append(args, tenantID)
	}
	query += " ORDER BY weight DESC"

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("channel list by model: %w", err)
	}
	defer rows.Close()

	var channels []domain.Channel
	for rows.Next() {
		c, err := scanChannel(rows)
		if err != nil {
			return nil, fmt.Errorf("channel list scan: %w", err)
		}
		channels = append(channels, *c)
	}
	return channels, rows.Err()
}

// FindByID retrieves a channel by its primary key.
func (r *PostgresRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Channel, error) {
	query := "SELECT " + channelSelectClause + " FROM channels WHERE id = $1"
	return scanChannel(r.pool.QueryRow(ctx, query, id))
}

// ListInstances returns all instances belonging to a channel.
func (r *PostgresRepository) ListInstances(ctx context.Context, channelID uuid.UUID) ([]domain.ChannelInstance, error) {
	const query = `
		SELECT id, channel_id, instance_type, base_url, provider_route,
			current_load, max_load,
			COALESCE(config::text, '{}'),
			status, created_at, updated_at
		FROM channel_instances
		WHERE channel_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.pool.Query(ctx, query, channelID)
	if err != nil {
		return nil, fmt.Errorf("channel instances list: %w", err)
	}
	defer rows.Close()

	var instances []domain.ChannelInstance
	for rows.Next() {
		var ci domain.ChannelInstance
		var configJSON string
		var providerRoute *string
		err := rows.Scan(
			&ci.ID, &ci.ChannelID, &ci.InstanceType, &ci.BaseURL, &providerRoute,
			&ci.CurrentLoad, &ci.MaxLoad,
			&configJSON,
			&ci.Status, &ci.CreatedAt, &ci.UpdatedAt,
		)
		if providerRoute != nil {
			ci.ProviderRoute = *providerRoute
		}
		if err != nil {
			return nil, fmt.Errorf("channel instance scan: %w", err)
		}
		ci.Config = unmarshalJSONB(configJSON)
		instances = append(instances, ci)
	}
	return instances, rows.Err()
}

// UpdateHealth updates the health score and status of a channel.
func (r *PostgresRepository) UpdateHealth(ctx context.Context, id uuid.UUID, score int, status domain.HealthStatus) error {
	const query = `
		UPDATE channels SET health_score = $1, health_status = $2, updated_at = NOW()
		WHERE id = $3
	`
	tag, err := r.pool.Exec(ctx, query, score, status, id)
	if err != nil {
		return fmt.Errorf("channel update health: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("channel update health: channel %s not found", id)
	}
	return nil
}

// UpdateInstanceLoad updates the current load of a channel instance.
func (r *PostgresRepository) UpdateInstanceLoad(ctx context.Context, id uuid.UUID, load int) error {
	const query = `
		UPDATE channel_instances SET current_load = $1, updated_at = NOW()
		WHERE id = $2
	`
	tag, err := r.pool.Exec(ctx, query, load, id)
	if err != nil {
		return fmt.Errorf("channel instance update load: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("channel instance update load: instance %s not found", id)
	}
	return nil
}

// FindRoutePolicy retrieves the best matching route policy.
func (r *PostgresRepository) FindRoutePolicy(ctx context.Context, tenantID *uuid.UUID, modelID uuid.UUID, userLevel string) (*domain.RoutePolicy, error) {
	query := `
		SELECT id, name, tenant_id, user_level, model_id, priority,
			candidate_channel_ids, fallback_policy, is_active, created_at, updated_at
		FROM route_policies
		WHERE is_active = TRUE
		  AND model_id = $1
		  AND (user_level = $2 OR user_level IS NULL)
		  AND tenant_id IS NOT DISTINCT FROM $3
		ORDER BY priority DESC
		LIMIT 1
	`
	return scanRoutePolicy(r.pool.QueryRow(ctx, query, modelID, userLevel, tenantID))
}

// --- helpers ---

const channelSelectClause = `
	id, name, model_id, tenant_id, pool_type, health_score, health_status,
	status, weight, max_concurrency, created_at, updated_at
`

func scanChannel(row pgx.Row) (*domain.Channel, error) {
	var c domain.Channel
	err := row.Scan(
		&c.ID, &c.Name, &c.ModelID, &c.TenantID, &c.PoolType,
		&c.HealthScore, &c.HealthStatus,
		&c.Status, &c.Weight, &c.MaxConcurrency,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("channel scan: %w", err)
	}
	return &c, nil
}

func scanRoutePolicy(row pgx.Row) (*domain.RoutePolicy, error) {
	var rp domain.RoutePolicy
	err := row.Scan(
		&rp.ID, &rp.Name, &rp.TenantID, &rp.UserLevel, &rp.ModelID,
		&rp.Priority, &rp.CandidateChannelIDs, &rp.FallbackPolicy,
		&rp.IsActive, &rp.CreatedAt, &rp.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("route policy scan: %w", err)
	}
	return &rp, nil
}

func unmarshalJSONB(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" || raw == "null" {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return map[string]any{}
	}
	return m
}
