// PostgreSQL persistence for the billing synchronization module.
// Implements billingsync.Repository using pgx (no GORM dependency).
package persistence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/deeptrols/api/internal/billing_sync"
	"github.com/deeptrols/api/internal/pkg/encrypt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository implements billingsync.Repository on PostgreSQL.
// Credentials are encrypted with the platform ENCRYPTION_KEY and never
// returned unless includeCredentials is requested.
type PostgresRepository struct {
	pool          *pgxpool.Pool
	credentialKey []byte
}

func NewPostgresRepository(pool *pgxpool.Pool, credentialKey []byte) *PostgresRepository {
	return &PostgresRepository{pool: pool, credentialKey: credentialKey}
}

var _ billingsync.Repository = (*PostgresRepository)(nil)

func (r *PostgresRepository) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 15*time.Second)
}

// ---------------------------------------------------------------------------
// Connector management
// ---------------------------------------------------------------------------

func (r *PostgresRepository) CreateBillingConnector(connector billingsync.Connector) (billingsync.Connector, error) {
	ctx, cancel := r.ctx()
	defer cancel()

	if strings.TrimSpace(connector.ID) == "" {
		connector.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if connector.CreatedAt.IsZero() {
		connector.CreatedAt = now
	}
	connector.UpdatedAt = now
	connector.NextSyncAt = nextSyncAt(connector, now)

	ciphertext, err := r.encryptCredentials(connector.Credentials)
	if err != nil {
		return billingsync.Connector{}, err
	}
	connector.CredentialCiphertext = ciphertext
	connector.Credentials = nil

	configJSON, err := json.Marshal(stringMapOrEmpty(connector.Config))
	if err != nil {
		return billingsync.Connector{}, err
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO billing_connectors (
			id, name, type, base_url, status, schedule_interval_minutes, config,
			credential_ciphertext, checkpoint, last_synced_through, last_sync_status,
			last_sync_message, last_sync_at, next_sync_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
		connector.ID, connector.Name, connector.Type, connector.BaseURL, connector.Status,
		connector.ScheduleIntervalMinutes, configJSON, connector.CredentialCiphertext,
		connector.Checkpoint, connector.LastSyncedThrough, connector.LastSyncStatus,
		connector.LastSyncMessage, connector.LastSyncAt, connector.NextSyncAt,
		connector.CreatedAt, connector.UpdatedAt)
	if err != nil {
		return billingsync.Connector{}, r.wrapWriteErr(err, "billing_connector_conflict", "Billing connector already exists")
	}
	return connectorSummary(connector), nil
}

func (r *PostgresRepository) ListBillingConnectors() []billingsync.Connector {
	ctx, cancel := r.ctx()
	defer cancel()

	rows, err := r.pool.Query(ctx,
		`SELECT id, name, type, base_url, status, schedule_interval_minutes, config,
		        credential_ciphertext, checkpoint, last_synced_through, last_sync_status,
		        last_sync_message, last_sync_at, next_sync_at, created_at, updated_at
		 FROM billing_connectors ORDER BY created_at ASC`)
	if err != nil {
		log.Printf("billing_sync: list connectors: %v", err)
		return []billingsync.Connector{}
	}
	defer rows.Close()

	connectors := make([]billingsync.Connector, 0)
	for rows.Next() {
		c, scanErr := scanConnector(rows)
		if scanErr != nil {
			log.Printf("billing_sync: scan connector: %v", scanErr)
			continue
		}
		connectors = append(connectors, connectorSummary(c))
	}
	return connectors
}

func (r *PostgresRepository) GetBillingConnector(id string, includeCredentials bool) (billingsync.Connector, error) {
	ctx, cancel := r.ctx()
	defer cancel()

	row := r.pool.QueryRow(ctx,
		`SELECT id, name, type, base_url, status, schedule_interval_minutes, config,
		        credential_ciphertext, checkpoint, last_synced_through, last_sync_status,
		        last_sync_message, last_sync_at, next_sync_at, created_at, updated_at
		 FROM billing_connectors WHERE id = $1`, strings.TrimSpace(id))
	connector, err := scanConnector(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return billingsync.Connector{}, billingsync.NewError(billingsync.ErrorNotFound, "billing_connector_not_found", "Billing connector not found")
		}
		return billingsync.Connector{}, err
	}
	if includeCredentials && connector.CredentialCiphertext != "" {
		credentials, derr := r.decryptCredentials(connector.CredentialCiphertext)
		if derr != nil {
			return billingsync.Connector{}, derr
		}
		connector.Credentials = credentials
	}
	return connectorSummaryWithCredentials(connector, includeCredentials), nil
}

func (r *PostgresRepository) UpdateBillingConnector(id string, patch billingsync.Connector) (billingsync.Connector, error) {
	ctx, cancel := r.ctx()
	defer cancel()

	current, err := r.getRawBillingConnector(ctx, strings.TrimSpace(id))
	if err != nil {
		return billingsync.Connector{}, err
	}
	if patch.Name != "" {
		current.Name = patch.Name
	}
	if patch.BaseURL != "" {
		current.BaseURL = patch.BaseURL
	}
	if patch.Status != "" {
		current.Status = patch.Status
	}
	if patch.ScheduleIntervalMinutes >= 0 {
		current.ScheduleIntervalMinutes = patch.ScheduleIntervalMinutes
	}
	if patch.Config != nil {
		current.Config = cloneStringMap(patch.Config)
	}
	if patch.Credentials != nil {
		ciphertext, cerr := r.encryptCredentials(patch.Credentials)
		if cerr != nil {
			return billingsync.Connector{}, cerr
		}
		current.CredentialCiphertext = ciphertext
	}
	current.UpdatedAt = time.Now().UTC()
	current.NextSyncAt = nextSyncAt(current, current.UpdatedAt)

	configJSON, err := json.Marshal(stringMapOrEmpty(current.Config))
	if err != nil {
		return billingsync.Connector{}, err
	}
	_, err = r.pool.Exec(ctx,
		`UPDATE billing_connectors SET
			name=$2, base_url=$3, status=$4, schedule_interval_minutes=$5, config=$6,
			credential_ciphertext=$7, next_sync_at=$8, updated_at=$9
		 WHERE id=$1`,
		current.ID, current.Name, current.BaseURL, current.Status,
		current.ScheduleIntervalMinutes, configJSON, current.CredentialCiphertext,
		current.NextSyncAt, current.UpdatedAt)
	if err != nil {
		return billingsync.Connector{}, err
	}
	return connectorSummary(current), nil
}

// getRawBillingConnector loads the persisted row without applying the summary
// transform, so callers that need to persist credentials keep the ciphertext.
func (r *PostgresRepository) getRawBillingConnector(ctx context.Context, id string) (billingsync.Connector, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, name, type, base_url, status, schedule_interval_minutes, config,
		        credential_ciphertext, checkpoint, last_synced_through, last_sync_status,
		        last_sync_message, last_sync_at, next_sync_at, created_at, updated_at
		 FROM billing_connectors WHERE id = $1`, id)
	connector, err := scanConnector(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return billingsync.Connector{}, billingsync.NewError(billingsync.ErrorNotFound, "billing_connector_not_found", "Billing connector not found")
		}
		return billingsync.Connector{}, err
	}
	return connector, nil
}

func (r *PostgresRepository) DeleteBillingConnector(id string) error {
	ctx, cancel := r.ctx()
	defer cancel()

	id = strings.TrimSpace(id)
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Deleting a connector removes its synchronized history (raw snapshots,
	// records, and sync runs) along with the configuration.
	for _, table := range []string{"billing_raw_snapshots", "billing_records", "billing_sync_runs"} {
		if _, err := tx.Exec(ctx, `DELETE FROM `+table+` WHERE connector_id = $1`, id); err != nil {
			return err
		}
	}
	tag, err := tx.Exec(ctx, `DELETE FROM billing_connectors WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return billingsync.NewError(billingsync.ErrorNotFound, "billing_connector_not_found", "Billing connector not found")
	}
	return tx.Commit(ctx)
}

// ---------------------------------------------------------------------------
// Synchronization
// ---------------------------------------------------------------------------

func (r *PostgresRepository) StartBillingSyncRun(run billingsync.SyncRun) (billingsync.SyncRun, error) {
	ctx, cancel := r.ctx()
	defer cancel()

	if strings.TrimSpace(run.ID) == "" {
		run.ID = uuid.New().String()
	}
	if run.StartedAt.IsZero() {
		run.StartedAt = time.Now().UTC()
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO billing_sync_runs (
			id, connector_id, trigger, status, range_start, range_end, cursor_start,
			cursor_end, pages_fetched, attempts, records_seen, records_inserted,
			records_updated, error_code, error_message, started_at, finished_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
		run.ID, run.ConnectorID, run.Trigger, run.Status, run.RangeStart, run.RangeEnd,
		run.CursorStart, run.CursorEnd, run.PagesFetched, run.Attempts, run.RecordsSeen,
		run.RecordsInserted, run.RecordsUpdated, nullableString(run.ErrorCode),
		nullableString(run.ErrorMessage), run.StartedAt, run.FinishedAt)
	if err != nil {
		return billingsync.SyncRun{}, err
	}
	return run, nil
}

func (r *PostgresRepository) SaveBillingPage(connectorID, checkpoint string, records []billingsync.Record) (int, int, error) {
	ctx, cancel := r.ctx()
	defer cancel()

	inserted, updated := 0, 0
	for i := range records {
		record := &records[i]
		if strings.TrimSpace(record.ID) == "" {
			record.ID = uuid.New().String()
		}
		now := time.Now().UTC()
		if record.CreatedAt.IsZero() {
			record.CreatedAt = now
		}
		record.UpdatedAt = now

		var snapshotID *string
		if strings.TrimSpace(record.RawPayload) != "" {
			sid, serr := r.saveRawSnapshot(ctx, connectorID, record.ExternalID, record.RawPayload)
			if serr != nil {
				return 0, 0, serr
			}
			snapshotID = &sid
		}
		if snapshotID != nil {
			record.RawSnapshotID = *snapshotID
		}

		metadataJSON, err := json.Marshal(stringMapOrEmpty(record.Metadata))
		if err != nil {
			return 0, 0, err
		}
		var wasInserted bool
		err = r.pool.QueryRow(ctx,
			`INSERT INTO billing_records (
				id, connector_id, external_id, source_type, account_id, service, product,
				model, currency, gross_amount, discount_amount, tax_amount, refund_amount,
				net_amount, usage_quantity, usage_unit, usage_start_at, usage_end_at,
				source_timezone, billing_period, external_request_id, raw_snapshot_id,
				metadata, created_at, updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25)
			ON CONFLICT (connector_id, external_id) DO UPDATE SET
				source_type=EXCLUDED.source_type, account_id=EXCLUDED.account_id,
				service=EXCLUDED.service, product=EXCLUDED.product, model=EXCLUDED.model,
				currency=EXCLUDED.currency, gross_amount=EXCLUDED.gross_amount,
				discount_amount=EXCLUDED.discount_amount, tax_amount=EXCLUDED.tax_amount,
				refund_amount=EXCLUDED.refund_amount, net_amount=EXCLUDED.net_amount,
				usage_quantity=EXCLUDED.usage_quantity, usage_unit=EXCLUDED.usage_unit,
				usage_start_at=EXCLUDED.usage_start_at, usage_end_at=EXCLUDED.usage_end_at,
				source_timezone=EXCLUDED.source_timezone, billing_period=EXCLUDED.billing_period,
				external_request_id=EXCLUDED.external_request_id, raw_snapshot_id=EXCLUDED.raw_snapshot_id,
				metadata=EXCLUDED.metadata, updated_at=EXCLUDED.updated_at
			RETURNING (xmax = 0)`,
			record.ID, connectorID, record.ExternalID, record.SourceType, record.AccountID,
			record.Service, record.Product, record.Model, record.Currency, record.GrossAmount,
			record.DiscountAmount, record.TaxAmount, record.RefundAmount, record.NetAmount,
			record.UsageQuantity, record.UsageUnit, record.UsageStartAt, record.UsageEndAt,
			record.SourceTimezone, record.BillingPeriod, record.ExternalRequestID, snapshotID,
			metadataJSON, record.CreatedAt, record.UpdatedAt).Scan(&wasInserted)
		if err != nil {
			return 0, 0, err
		}
		if wasInserted {
			inserted++
		} else {
			updated++
		}
	}

	if _, err := r.pool.Exec(ctx,
		`UPDATE billing_connectors SET checkpoint=$2, updated_at=NOW() WHERE id=$1`,
		connectorID, checkpoint); err != nil {
		return 0, 0, err
	}
	return inserted, updated, nil
}

func (r *PostgresRepository) FinishBillingSyncRun(run billingsync.SyncRun) (billingsync.SyncRun, error) {
	ctx, cancel := r.ctx()
	defer cancel()

	if run.FinishedAt == nil {
		finished := time.Now().UTC()
		run.FinishedAt = &finished
	}
	_, err := r.pool.Exec(ctx,
		`UPDATE billing_sync_runs SET
			status=$2, cursor_end=$3, pages_fetched=$4, attempts=$5, records_seen=$6,
			records_inserted=$7, records_updated=$8, error_code=$9, error_message=$10,
			finished_at=$11
		 WHERE id=$1`,
		run.ID, run.Status, run.CursorEnd, run.PagesFetched, run.Attempts, run.RecordsSeen,
		run.RecordsInserted, run.RecordsUpdated, nullableString(run.ErrorCode),
		nullableString(run.ErrorMessage), run.FinishedAt)
	if err != nil {
		return billingsync.SyncRun{}, err
	}

	checkpoint := run.Status == billingsync.SyncSucceeded
	if _, err := r.pool.Exec(ctx,
		`UPDATE billing_connectors SET
			checkpoint = CASE WHEN $2 THEN '' ELSE checkpoint END,
			last_synced_through = CASE WHEN $2 THEN $3 ELSE last_synced_through END,
			last_sync_status = $4, last_sync_message = $5, last_sync_at = $6,
			next_sync_at = CASE WHEN status = 'active' AND schedule_interval_minutes > 0
			                    THEN $6::timestamptz + make_interval(mins => schedule_interval_minutes)
			                    ELSE NULL END,
			updated_at = $6::timestamptz
		 WHERE id = $1`,
		run.ConnectorID, checkpoint, run.RangeEnd, run.Status, run.ErrorMessage,
		run.FinishedAt); err != nil {
		return billingsync.SyncRun{}, err
	}
	return run, nil
}

func (r *PostgresRepository) ListDueBillingConnectors(now time.Time, limit int) []billingsync.Connector {
	ctx, cancel := r.ctx()
	defer cancel()

	if limit <= 0 {
		limit = 25
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, type, base_url, status, schedule_interval_minutes, config,
		        credential_ciphertext, checkpoint, last_synced_through, last_sync_status,
		        last_sync_message, last_sync_at, next_sync_at, created_at, updated_at
		 FROM billing_connectors
		 WHERE status = 'active' AND (next_sync_at IS NULL OR next_sync_at <= $1)
		 ORDER BY next_sync_at NULLS FIRST
		 LIMIT $2`, now.UTC(), limit)
	if err != nil {
		log.Printf("billing_sync: list due connectors: %v", err)
		return []billingsync.Connector{}
	}
	defer rows.Close()

	connectors := make([]billingsync.Connector, 0)
	for rows.Next() {
		c, scanErr := scanConnector(rows)
		if scanErr != nil {
			log.Printf("billing_sync: scan due connector: %v", scanErr)
			continue
		}
		connectors = append(connectors, connectorSummary(c))
	}
	return connectors
}

// RecordScheduledBillingAudit records a lightweight audit entry for scheduled
// sync runs so the platform audit trail explains why a connector synced.
func (r *PostgresRepository) RecordScheduledBillingAudit(run billingsync.SyncRun) {
	ctx, cancel := r.ctx()
	defer cancel()

	payload, _ := json.Marshal(map[string]any{
		"status":      run.Status,
		"error_code":  run.ErrorCode,
		"error_msg":   run.ErrorMessage,
		"trigger":     run.Trigger,
		"sync_run_id": run.ID,
	})
	if _, err := r.pool.Exec(ctx,
		`INSERT INTO audit_logs (actor_type, action, resource_type, resource_id, new_value, created_at)
		 VALUES ('system', 'billing_sync_scheduled', 'billing_connector', $1, $2, NOW())`,
		run.ConnectorID, payload); err != nil {
		log.Printf("billing_sync: scheduled audit: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Management list queries
// ---------------------------------------------------------------------------

func (r *PostgresRepository) ListBillingRecords(connectorID string, limit int) []billingsync.Record {
	ctx, cancel := r.ctx()
	defer cancel()

	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	query := `SELECT id, connector_id, external_id, source_type, account_id, service, product,
	              model, currency, gross_amount, discount_amount, tax_amount, refund_amount,
	              net_amount, usage_quantity, usage_unit, usage_start_at, usage_end_at,
	              source_timezone, billing_period, external_request_id, raw_snapshot_id,
	              metadata, created_at, updated_at
	          FROM billing_records`
	args := []any{}
	if strings.TrimSpace(connectorID) != "" {
		query += ` WHERE connector_id = $1`
		args = append(args, connectorID)
	}
	query += ` ORDER BY usage_start_at DESC LIMIT ` + fmt.Sprintf("%d", limit)
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		log.Printf("billing_sync: list records: %v", err)
		return []billingsync.Record{}
	}
	defer rows.Close()
	records := make([]billingsync.Record, 0)
	for rows.Next() {
		rec, scanErr := scanRecord(rows)
		if scanErr != nil {
			log.Printf("billing_sync: scan record: %v", scanErr)
			continue
		}
		records = append(records, rec)
	}
	return records
}

func (r *PostgresRepository) ListBillingSyncRuns(connectorID string, limit int) []billingsync.SyncRun {
	ctx, cancel := r.ctx()
	defer cancel()

	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `SELECT id, connector_id, trigger, status, range_start, range_end, cursor_start,
	              cursor_end, pages_fetched, attempts, records_seen, records_inserted,
	              records_updated, error_code, error_message, started_at, finished_at
	          FROM billing_sync_runs`
	args := []any{}
	if strings.TrimSpace(connectorID) != "" {
		query += ` WHERE connector_id = $1`
		args = append(args, connectorID)
	}
	query += ` ORDER BY started_at DESC LIMIT ` + fmt.Sprintf("%d", limit)
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		log.Printf("billing_sync: list runs: %v", err)
		return []billingsync.SyncRun{}
	}
	defer rows.Close()
	runs := make([]billingsync.SyncRun, 0)
	for rows.Next() {
		run, scanErr := scanSyncRun(rows)
		if scanErr != nil {
			log.Printf("billing_sync: scan run: %v", scanErr)
			continue
		}
		runs = append(runs, run)
	}
	return runs
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (r *PostgresRepository) saveRawSnapshot(ctx context.Context, connectorID, externalID, payload string) (string, error) {
	sum := sha256.Sum256([]byte(payload))
	hash := hex.EncodeToString(sum[:])
	id := uuid.New().String()
	_, err := r.pool.Exec(ctx,
		`INSERT INTO billing_raw_snapshots (id, connector_id, external_id, payload_hash, payload, captured_at)
		 VALUES ($1,$2,$3,$4,$5,NOW())
		 ON CONFLICT (connector_id, external_id, payload_hash) DO NOTHING`,
		id, connectorID, externalID, hash, payload)
	if err != nil {
		return "", err
	}
	var existing string
	err = r.pool.QueryRow(ctx,
		`SELECT id::text FROM billing_raw_snapshots WHERE connector_id=$1 AND external_id=$2 AND payload_hash=$3`,
		connectorID, externalID, hash).Scan(&existing)
	if err != nil {
		return "", err
	}
	return existing, nil
}

func (r *PostgresRepository) encryptCredentials(credentials map[string]string) (string, error) {
	if len(credentials) == 0 {
		return "", nil
	}
	plaintext, err := json.Marshal(credentials)
	if err != nil {
		return "", err
	}
	ciphertext, err := encrypt.Encrypt(string(plaintext), r.credentialKey)
	if err != nil {
		return "", billingsync.WrapError(err, billingsync.ErrorInvalidInput, "billing_credential_encryption_failed", "Failed to encrypt billing connector credentials")
	}
	return ciphertext, nil
}

func (r *PostgresRepository) decryptCredentials(ciphertext string) (map[string]string, error) {
	plaintext, err := encrypt.Decrypt(ciphertext, r.credentialKey)
	if err != nil {
		return nil, billingsync.WrapError(err, billingsync.ErrorInvalidInput, "billing_credential_decryption_failed", "Failed to decrypt billing connector credentials")
	}
	var credentials map[string]string
	if err := json.Unmarshal([]byte(plaintext), &credentials); err != nil {
		return nil, err
	}
	return credentials, nil
}

func (r *PostgresRepository) wrapWriteErr(err error, code, message string) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return billingsync.WrapError(err, billingsync.ErrorConflict, code, message)
	}
	return err
}

func connectorSummary(connector billingsync.Connector) billingsync.Connector {
	return connectorSummaryWithCredentials(connector, false)
}

func connectorSummaryWithCredentials(connector billingsync.Connector, includeCredentials bool) billingsync.Connector {
	fields := make([]string, 0)
	if includeCredentials {
		for key := range connector.Credentials {
			fields = append(fields, key)
		}
	}
	sort.Strings(fields)
	connector.CredentialsConfigured = connector.CredentialCiphertext != "" || len(connector.Credentials) > 0
	connector.CredentialFields = fields
	connector.CredentialCiphertext = ""
	if !includeCredentials {
		connector.Credentials = nil
	}
	return connector
}

func nextSyncAt(connector billingsync.Connector, from time.Time) *time.Time {
	if connector.Status != billingsync.StatusActive || connector.ScheduleIntervalMinutes <= 0 {
		return nil
	}
	next := from.Add(time.Duration(connector.ScheduleIntervalMinutes) * time.Minute)
	return &next
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func stringMapOrEmpty(in map[string]string) map[string]string {
	if in == nil {
		return map[string]string{}
	}
	return in
}

func nullableString(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}

type connectorScanner interface {
	Scan(dest ...any) error
}

func scanConnector(row connectorScanner) (billingsync.Connector, error) {
	var c billingsync.Connector
	var configJSON string
	err := row.Scan(
		&c.ID, &c.Name, &c.Type, &c.BaseURL, &c.Status, &c.ScheduleIntervalMinutes,
		&configJSON, &c.CredentialCiphertext, &c.Checkpoint, &c.LastSyncedThrough,
		&c.LastSyncStatus, &c.LastSyncMessage, &c.LastSyncAt, &c.NextSyncAt,
		&c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return c, err
	}
	_ = json.Unmarshal([]byte(configJSON), &c.Config)
	if c.Config == nil {
		c.Config = map[string]string{}
	}
	return c, nil
}

type recordScanner interface {
	Scan(dest ...any) error
}

func scanRecord(row recordScanner) (billingsync.Record, error) {
	var rec billingsync.Record
	var metadataJSON string
	var rawSnapshotID *string
	err := row.Scan(
		&rec.ID, &rec.ConnectorID, &rec.ExternalID, &rec.SourceType, &rec.AccountID,
		&rec.Service, &rec.Product, &rec.Model, &rec.Currency, &rec.GrossAmount,
		&rec.DiscountAmount, &rec.TaxAmount, &rec.RefundAmount, &rec.NetAmount,
		&rec.UsageQuantity, &rec.UsageUnit, &rec.UsageStartAt, &rec.UsageEndAt,
		&rec.SourceTimezone, &rec.BillingPeriod, &rec.ExternalRequestID,
		&rawSnapshotID, &metadataJSON, &rec.CreatedAt, &rec.UpdatedAt)
	if err != nil {
		return rec, err
	}
	if rawSnapshotID != nil {
		rec.RawSnapshotID = *rawSnapshotID
	}
	_ = json.Unmarshal([]byte(metadataJSON), &rec.Metadata)
	if rec.Metadata == nil {
		rec.Metadata = map[string]string{}
	}
	return rec, nil
}

type syncRunScanner interface {
	Scan(dest ...any) error
}

func scanSyncRun(row syncRunScanner) (billingsync.SyncRun, error) {
	var run billingsync.SyncRun
	var errorCode, errorMessage *string
	err := row.Scan(
		&run.ID, &run.ConnectorID, &run.Trigger, &run.Status, &run.RangeStart, &run.RangeEnd,
		&run.CursorStart, &run.CursorEnd, &run.PagesFetched, &run.Attempts, &run.RecordsSeen,
		&run.RecordsInserted, &run.RecordsUpdated, &errorCode, &errorMessage,
		&run.StartedAt, &run.FinishedAt)
	if err != nil {
		return run, err
	}
	if errorCode != nil {
		run.ErrorCode = *errorCode
	}
	if errorMessage != nil {
		run.ErrorMessage = *errorMessage
	}
	return run, nil
}
