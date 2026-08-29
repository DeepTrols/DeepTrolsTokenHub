package rankings

import (
	"math"
	"testing"
	"time"
)

func testConfig() Config {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	return Config{
		ID:          "week",
		Start:       now.Add(-7 * 24 * time.Hour),
		End:         now,
		PrevStart:   now.Add(-14 * 24 * time.Hour),
		PrevEnd:     now.Add(-7 * 24 * time.Hour),
		BucketTrunc: "day",
		LabelLayout: "01-02",
	}
}

func TestBuildSnapshot_RanksAndGrowth(t *testing.T) {
	totals := []ModelTotal{
		{Model: "deepseek-chat", Provider: "deepseek", TotalTokens: 1000},
		{Model: "glm-4", Provider: "zhipu", TotalTokens: 500},
	}
	previous := []ModelTotal{
		{Model: "deepseek-chat", Provider: "deepseek", TotalTokens: 800},
		{Model: "glm-4", Provider: "zhipu", TotalTokens: 700},
	}
	snap := BuildSnapshot(testConfig(), totals, previous, nil)

	if len(snap.Models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(snap.Models))
	}
	top := snap.Models[0]
	if top.ModelName != "deepseek-chat" || top.Rank != 1 || top.TotalTokens != 1000 {
		t.Fatalf("unexpected top model: %+v", top)
	}
	if top.PreviousRank == nil || *top.PreviousRank != 1 {
		t.Fatalf("previous rank: %+v", top.PreviousRank)
	}
	if math.Abs(top.Share-1000.0/1500.0) > 0.0001 {
		t.Fatalf("share: %v", top.Share)
	}
	if math.Abs(top.GrowthPct-25.0) > 0.0001 {
		t.Fatalf("growth: %v", top.GrowthPct)
	}
	second := snap.Models[1]
	if second.Rank != 2 || second.PreviousRank == nil || *second.PreviousRank != 2 {
		t.Fatalf("second model: %+v", second)
	}
	if math.Abs(second.GrowthPct-(-200.0/7.0)) > 0.0001 {
		t.Fatalf("second growth: %v", second.GrowthPct)
	}
}

func TestBuildSnapshot_VendorsAggregate(t *testing.T) {
	totals := []ModelTotal{
		{Model: "deepseek-chat", Provider: "deepseek", TotalTokens: 1000},
		{Model: "deepseek-reasoner", Provider: "deepseek", TotalTokens: 300},
		{Model: "glm-4", Provider: "zhipu", TotalTokens: 200},
	}
	snap := BuildSnapshot(testConfig(), totals, nil, nil)

	if len(snap.Vendors) != 2 {
		t.Fatalf("expected 2 vendors, got %d: %+v", len(snap.Vendors), snap.Vendors)
	}
	top := snap.Vendors[0]
	if top.Vendor != "deepseek" || top.TotalTokens != 1300 || top.ModelsCount != 2 || top.TopModel != "deepseek-chat" {
		t.Fatalf("unexpected vendor: %+v", top)
	}
}

func TestBuildSnapshot_MoversAndDroppers(t *testing.T) {
	totals := []ModelTotal{
		{Model: "a", Provider: "p1", TotalTokens: 100},
		{Model: "b", Provider: "p1", TotalTokens: 90},
		{Model: "c", Provider: "p2", TotalTokens: 80},
	}
	previous := []ModelTotal{
		{Model: "c", Provider: "p2", TotalTokens: 100},
		{Model: "a", Provider: "p1", TotalTokens: 10},
		{Model: "b", Provider: "p1", TotalTokens: 50},
	}
	snap := BuildSnapshot(testConfig(), totals, previous, nil)

	if len(snap.TopMovers) == 0 {
		t.Fatal("expected movers")
	}
	if snap.TopMovers[0].ModelName != "a" || snap.TopMovers[0].RankDelta != 2 {
		t.Fatalf("expected a to climb 2 ranks, got %+v", snap.TopMovers[0])
	}
	if len(snap.TopDroppers) == 0 || snap.TopDroppers[0].ModelName != "c" || snap.TopDroppers[0].RankDelta != -2 {
		t.Fatalf("expected c to drop, got %+v", snap.TopDroppers)
	}
}

func TestBuildSnapshot_HistoryBuckets(t *testing.T) {
	cfg := testConfig()
	base := cfg.Start.Add(24 * time.Hour)
	buckets := []BucketPoint{
		{Bucket: base, Model: "deepseek-chat", Provider: "deepseek", Tokens: 100},
		{Bucket: base, Model: "glm-4", Provider: "zhipu", Tokens: 50},
		{Bucket: base.Add(24 * time.Hour), Model: "deepseek-chat", Provider: "deepseek", Tokens: 200},
	}
	totals := []ModelTotal{
		{Model: "deepseek-chat", Provider: "deepseek", TotalTokens: 300},
		{Model: "glm-4", Provider: "zhipu", TotalTokens: 50},
	}
	snap := BuildSnapshot(cfg, totals, nil, buckets)

	if snap.ModelsHistory.Buckets != 2 {
		t.Fatalf("expected 2 buckets, got %d", snap.ModelsHistory.Buckets)
	}
	if len(snap.ModelsHistory.Points) != 3 {
		t.Fatalf("expected 3 history points, got %d: %+v", len(snap.ModelsHistory.Points), snap.ModelsHistory.Points)
	}
	if len(snap.VendorShareHistory.Points) != 3 {
		t.Fatalf("expected 3 vendor share points, got %d", len(snap.VendorShareHistory.Points))
	}
}

func TestGrowthPct_ZeroPrevious(t *testing.T) {
	if g := growthPct(100, 0); math.Abs(g-100) > 0.0001 {
		t.Fatalf("expected 100 growth for new model, got %v", g)
	}
	if g := growthPct(0, 0); g != 0 {
		t.Fatalf("expected 0 growth for empty model, got %v", g)
	}
}
