package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/billing_sync"
	"github.com/deeptrols/api/internal/repository/testutil"
)

var testCredentialKey = []byte("0123456789abcdef0123456789abcdef")

func newTestRepo(t *testing.T) *PostgresRepository {
	t.Helper()
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)
	return NewPostgresRepository(pool, testCredentialKey)
}

func sampleConnector() billingsync.Connector {
	return billingsync.Connector{
		Name:                    "aliyun-billing",
		Type:                    billingsync.ConnectorAliyun,
		BaseURL:                 "https://billing.aliyuncs.com",
		Status:                  billingsync.StatusActive,
		ScheduleIntervalMinutes: 60,
		Config: map[string]string{
			"product_code":    "dbaudit",
			"source_timezone": "Asia/Shanghai",
		},
		Credentials: map[string]string{
			"access_key_id":     "ak-test",
			"access_key_secret": "sk-test",
		},
	}
}

func TestPostgresRepository_ConnectorLifecycleAndCredentials(t *testing.T) {
	repo := newTestRepo(t)

	created, err := repo.CreateBillingConnector(sampleConnector())
	if err != nil {
		t.Fatalf("create connector: %v", err)
	}
	if created.ID == "" {
		t.Fatal("connector id must be assigned")
	}
	if created.CredentialsConfigured != true {
		t.Error("CredentialsConfigured should be true after create with credentials")
	}
	if created.Credentials != nil {
		t.Error("credentials must not be returned in summary")
	}
	if created.NextSyncAt == nil {
		t.Error("NextSyncAt should be set for active scheduled connector")
	}

	// Without credentials: decrypted secrets must stay hidden.
	got, err := repo.GetBillingConnector(created.ID, false)
	if err != nil {
		t.Fatalf("get connector: %v", err)
	}
	if got.Credentials != nil {
		t.Error("credentials leaked without includeCredentials")
	}
	if got.CredentialsConfigured != true {
		t.Error("CredentialsConfigured should reflect stored credentials")
	}

	// With credentials: decrypted secrets are returned.
	full, err := repo.GetBillingConnector(created.ID, true)
	if err != nil {
		t.Fatalf("get connector with credentials: %v", err)
	}
	if full.Credentials["access_key_id"] != "ak-test" || full.Credentials["access_key_secret"] != "sk-test" {
		t.Errorf("credentials round-trip failed: %#v", full.Credentials)
	}

	// Update keeps credentials unless replaced.
	updated, err := repo.UpdateBillingConnector(created.ID, billingsync.Connector{Name: "aliyun-prod"})
	if err != nil {
		t.Fatalf("update connector: %v", err)
	}
	if updated.Name != "aliyun-prod" {
		t.Errorf("name = %q, want aliyun-prod", updated.Name)
	}
	full2, _ := repo.GetBillingConnector(created.ID, true)
	if full2.Credentials["access_key_id"] != "ak-test" {
		t.Error("credentials lost on unrelated update")
	}

	connectors := repo.ListBillingConnectors()
	if len(connectors) != 1 {
		t.Fatalf("list connectors = %d, want 1", len(connectors))
	}

	if err := repo.DeleteBillingConnector(created.ID); err != nil {
		t.Fatalf("delete connector: %v", err)
	}
	if _, err := repo.GetBillingConnector(created.ID, false); err == nil {
		t.Error("connector should be gone after delete")
	}
}

func TestPostgresRepository_SyncRunLifecycle(t *testing.T) {
	repo := newTestRepo(t)
	connector, err := repo.CreateBillingConnector(sampleConnector())
	if err != nil {
		t.Fatalf("create connector: %v", err)
	}

	due := repo.ListDueBillingConnectors(time.Now().UTC().Add(time.Hour), 10)
	if len(due) != 1 || due[0].ID != connector.ID {
		t.Fatalf("due connectors = %d, want the fresh connector", len(due))
	}

	from := time.Now().UTC().Add(-time.Hour)
	to := time.Now().UTC()
	run, err := repo.StartBillingSyncRun(billingsync.SyncRun{
		ConnectorID: connector.ID,
		Trigger:     "scheduled",
		Status:      billingsync.SyncRunning,
		RangeStart:  from,
		RangeEnd:    to,
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if run.ID == "" {
		t.Fatal("run id must be assigned")
	}

	records := []billingsync.Record{
		{
			ConnectorID:   connector.ID,
			ExternalID:    "rec-1",
			SourceType:    "aliyun",
			AccountID:     "acct-1",
			Service:       "dbaudit",
			Model:         "deepseek-chat",
			Currency:      "CNY",
			GrossAmount:   "10.00",
			NetAmount:     "10.00",
			UsageQuantity: 1000,
			UsageUnit:     "token",
			UsageStartAt:  from,
			UsageEndAt:    to,
			RawPayload:    `{"request_id":"req-1"}`,
		},
		{
			ConnectorID:   connector.ID,
			ExternalID:    "rec-2",
			SourceType:    "aliyun",
			AccountID:     "acct-1",
			Currency:      "CNY",
			GrossAmount:   "5.00",
			NetAmount:     "5.00",
			UsageQuantity: 500,
			UsageUnit:     "token",
			UsageStartAt:  from,
			UsageEndAt:    to,
		},
	}
	inserted, updated, err := repo.SaveBillingPage(connector.ID, `{"cursor":"c1"}`, records)
	if err != nil {
		t.Fatalf("save page: %v", err)
	}
	if inserted != 2 || updated != 0 {
		t.Errorf("first page inserted=%d updated=%d, want 2/0", inserted, updated)
	}

	inserted, updated, err = repo.SaveBillingPage(connector.ID, `{"cursor":"c2"}`, records)
	if err != nil {
		t.Fatalf("save page again: %v", err)
	}
	if inserted != 0 || updated != 2 {
		t.Errorf("second page inserted=%d updated=%d, want 0/2", inserted, updated)
	}

	finished, err := repo.FinishBillingSyncRun(billingsync.SyncRun{
		ID:              run.ID,
		ConnectorID:     connector.ID,
		Status:          billingsync.SyncSucceeded,
		RangeEnd:        to,
		RecordsInserted: 2,
		RecordsSeen:     2,
		PagesFetched:    1,
	})
	if err != nil {
		t.Fatalf("finish run: %v", err)
	}
	if finished.FinishedAt == nil {
		t.Error("FinishedAt must be set")
	}

	after, _ := repo.GetBillingConnector(connector.ID, false)
	if after.LastSyncStatus != billingsync.SyncSucceeded {
		t.Errorf("LastSyncStatus = %q", after.LastSyncStatus)
	}
	if after.LastSyncedThrough == nil || !after.LastSyncedThrough.UTC().Truncate(time.Second).Equal(to.UTC().Truncate(time.Second)) {
		t.Errorf("LastSyncedThrough = %v, want %v", after.LastSyncedThrough, to)
	}
	if after.Checkpoint != "" {
		t.Error("checkpoint should be cleared on success")
	}
	if after.NextSyncAt == nil {
		t.Error("NextSyncAt should be scheduled after success")
	}

	recordsOut := repo.ListBillingRecords(connector.ID, 10)
	if len(recordsOut) != 2 {
		t.Fatalf("list records = %d, want 2", len(recordsOut))
	}
	var foundSnapshot bool
	for _, rec := range recordsOut {
		if rec.ExternalID == "rec-1" && rec.RawSnapshotID != "" {
			foundSnapshot = true
		}
	}
	if !foundSnapshot {
		t.Error("record with raw payload should carry raw_snapshot_id")
	}

	runs := repo.ListBillingSyncRuns(connector.ID, 10)
	if len(runs) != 1 || runs[0].Status != billingsync.SyncSucceeded {
		t.Fatalf("list runs = %+v, want one succeeded run", runs)
	}

	repo.RecordScheduledBillingAudit(finished)
	var auditCount int
	if err := repo.pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM audit_logs WHERE resource_type = 'billing_connector' AND action = 'billing_sync_scheduled'`).Scan(&auditCount); err != nil {
		t.Fatalf("audit count: %v", err)
	}
	if auditCount != 1 {
		t.Errorf("scheduled audit rows = %d, want 1", auditCount)
	}
}

func TestPostgresRepository_GetMissingConnector(t *testing.T) {
	repo := newTestRepo(t)
	_, err := repo.GetBillingConnector("00000000-0000-0000-0000-000000000000", false)
	kind, code, _, ok := billingsync.ErrorInfo(err)
	if err == nil || !ok || kind != billingsync.ErrorNotFound || code != "billing_connector_not_found" {
		t.Fatalf("expected not_found billing_connector_not_found, got %v", err)
	}
}
