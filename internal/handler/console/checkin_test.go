package console

import (
	"testing"

	"github.com/deeptrols/api/internal/service/setting"
	"github.com/shopspring/decimal"
)

func TestRandomCheckinReward_WithinRange(t *testing.T) {
	cfg := setting.CheckinConfig{
		Enabled:  true,
		MinQuota: decimal.NewFromInt(2),
		MaxQuota: decimal.NewFromInt(8),
	}
	seen := map[int64]bool{}
	for i := 0; i < 200; i++ {
		got := randomCheckinReward(cfg)
		if got.LessThan(cfg.MinQuota) || got.GreaterThan(cfg.MaxQuota) {
			t.Fatalf("reward %s outside range %s..%s", got, cfg.MinQuota, cfg.MaxQuota)
		}
		seen[got.IntPart()] = true
	}
	// The uniform range should hit more than one value over 200 draws.
	if len(seen) < 2 {
		t.Fatalf("expected varied rewards, got only %v", seen)
	}
}

func TestRandomCheckinReward_InvertedRangeUsesMin(t *testing.T) {
	cfg := setting.CheckinConfig{
		Enabled:  true,
		MinQuota: decimal.NewFromInt(5),
		MaxQuota: decimal.NewFromInt(3),
	}
	for i := 0; i < 20; i++ {
		if got := randomCheckinReward(cfg); !got.Equal(decimal.NewFromInt(5)) {
			t.Fatalf("expected min reward 5, got %s", got)
		}
	}
}

func TestRandomCheckinReward_FlatRange(t *testing.T) {
	cfg := setting.CheckinConfig{
		Enabled:  true,
		MinQuota: decimal.NewFromInt(4),
		MaxQuota: decimal.NewFromInt(4),
	}
	if got := randomCheckinReward(cfg); !got.Equal(decimal.NewFromInt(4)) {
		t.Fatalf("expected 4, got %s", got)
	}
}

func TestResolveCheckinConfig_NilSettingsFallsBack(t *testing.T) {
	cfg := resolveCheckinConfig(nil, nil)
	if !cfg.Enabled || !cfg.MinQuota.Equal(decimal.NewFromInt(1)) || !cfg.MaxQuota.Equal(decimal.NewFromInt(5)) {
		t.Fatalf("unexpected fallback config: %+v", cfg)
	}
}
