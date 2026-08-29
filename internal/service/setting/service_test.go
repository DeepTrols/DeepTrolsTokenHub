package setting

import (
	"context"
	"testing"

	settingrepo "github.com/deeptrols/api/internal/repository/setting"
	"github.com/shopspring/decimal"
)

type fakeRepo struct {
	entries []settingrepo.Entry
}

func (f *fakeRepo) All(ctx context.Context) ([]settingrepo.Entry, error) {
	return f.entries, nil
}

func (f *fakeRepo) Get(ctx context.Context, keys ...string) ([]settingrepo.Entry, error) {
	return f.entries, nil
}

func (f *fakeRepo) Upsert(ctx context.Context, entries []settingrepo.Entry) error {
	f.entries = append(f.entries, entries...)
	return nil
}

func TestPublicSiteDefaults(t *testing.T) {
	s := NewService(&fakeRepo{})
	site, err := s.PublicSite(context.Background())
	if err != nil {
		t.Fatalf("PublicSite: %v", err)
	}
	if site.SiteName != "DeepTrols" {
		t.Fatalf("expected default site_name DeepTrols, got %q", site.SiteName)
	}
	if site.FooterText != "" {
		t.Fatalf("expected empty footer default, got %q", site.FooterText)
	}
}

func TestPublicSiteOverrides(t *testing.T) {
	repo := &fakeRepo{entries: []settingrepo.Entry{
		{Key: KeySiteName, Value: []byte(`"Acme"`)},
		{Key: KeyUserAgreement, Value: []byte(`"https://acme/terms"`)},
	}}
	site, err := NewService(repo).PublicSite(context.Background())
	if err != nil {
		t.Fatalf("PublicSite: %v", err)
	}
	if site.SiteName != "Acme" {
		t.Fatalf("expected overridden site_name Acme, got %q", site.SiteName)
	}
	if site.Legal.UserAgreement != "https://acme/terms" {
		t.Fatalf("expected user agreement override, got %q", site.Legal.UserAgreement)
	}
}

func TestUpdateRejectsUnknownKey(t *testing.T) {
	s := NewService(&fakeRepo{})
	if err := s.Update(context.Background(), map[string]string{"not_a_key": "x"}); err == nil {
		t.Fatal("expected error for unknown key")
	}
}

func TestUpdatePersistsKnownKey(t *testing.T) {
	repo := &fakeRepo{}
	s := NewService(repo)
	if err := s.Update(context.Background(), map[string]string{KeySiteName: "Stellar"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(repo.entries) != 1 || string(repo.entries[0].Value) != `"Stellar"` {
		t.Fatalf("unexpected persisted entry: %+v", repo.entries)
	}
}

func TestCheckinConfigDefaults(t *testing.T) {
	s := NewService(&fakeRepo{})
	cfg, err := s.CheckinConfig(context.Background())
	if err != nil {
		t.Fatalf("CheckinConfig: %v", err)
	}
	if !cfg.Enabled {
		t.Fatal("expected checkin enabled by default")
	}
	if !cfg.MinQuota.Equal(decimal.NewFromInt(1)) || !cfg.MaxQuota.Equal(decimal.NewFromInt(5)) {
		t.Fatalf("unexpected default reward range %s..%s", cfg.MinQuota, cfg.MaxQuota)
	}
}

func TestCheckinConfigOverrides(t *testing.T) {
	repo := &fakeRepo{entries: []settingrepo.Entry{
		{Key: KeyCheckinEnabled, Value: []byte(`false`)},
		{Key: KeyCheckinMinQuota, Value: []byte(`"2"`)},
		{Key: KeyCheckinMaxQuota, Value: []byte(`"8"`)},
	}}
	cfg, err := NewService(repo).CheckinConfig(context.Background())
	if err != nil {
		t.Fatalf("CheckinConfig: %v", err)
	}
	if cfg.Enabled {
		t.Fatal("expected checkin disabled after override")
	}
	if !cfg.MinQuota.Equal(decimal.NewFromInt(2)) || !cfg.MaxQuota.Equal(decimal.NewFromInt(8)) {
		t.Fatalf("unexpected reward range %s..%s", cfg.MinQuota, cfg.MaxQuota)
	}
}

func TestCheckinConfigClampsInvertedRange(t *testing.T) {
	repo := &fakeRepo{entries: []settingrepo.Entry{
		{Key: KeyCheckinMinQuota, Value: []byte(`"10"`)},
		{Key: KeyCheckinMaxQuota, Value: []byte(`"3"`)},
	}}
	cfg, err := NewService(repo).CheckinConfig(context.Background())
	if err != nil {
		t.Fatalf("CheckinConfig: %v", err)
	}
	if !cfg.MaxQuota.Equal(decimal.NewFromInt(10)) {
		t.Fatalf("expected max clamped to min, got %s", cfg.MaxQuota)
	}
}

func TestUpdatePersistsCheckinKeys(t *testing.T) {
	repo := &fakeRepo{}
	s := NewService(repo)
	if err := s.Update(context.Background(), map[string]string{
		KeyCheckinEnabled:  "false",
		KeyCheckinMinQuota: "3",
		KeyCheckinMaxQuota: "9",
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(repo.entries) != 3 {
		t.Fatalf("expected 3 persisted entries, got %d", len(repo.entries))
	}
	cfg, err := s.CheckinConfig(context.Background())
	if err != nil {
		t.Fatalf("CheckinConfig: %v", err)
	}
	if cfg.Enabled || !cfg.MinQuota.Equal(decimal.NewFromInt(3)) || !cfg.MaxQuota.Equal(decimal.NewFromInt(9)) {
		t.Fatalf("unexpected config after update: %+v", cfg)
	}
}

func TestPublicSite_OAuthProviders(t *testing.T) {
	repo := &fakeRepo{entries: []settingrepo.Entry{
		{Key: KeyOAuthGithubEnabled, Value: []byte(`"true"`)},
		{Key: KeyOAuthWechatEnabled, Value: []byte(`true`)},
		{Key: KeyOAuthGoogleEnabled, Value: []byte(`true`)},
	}}
	site, err := NewService(repo).PublicSite(context.Background())
	if err != nil {
		t.Fatalf("PublicSite: %v", err)
	}
	if len(site.OAuthProviders) != 3 || site.OAuthProviders[0] != "github" || site.OAuthProviders[1] != "wechat" || site.OAuthProviders[2] != "google" {
		t.Fatalf("oauth providers: %v", site.OAuthProviders)
	}

	// Disabled (native false + string form) must expose no providers.
	for _, raw := range []string{`false`, `"false"`} {
		off := NewService(&fakeRepo{entries: []settingrepo.Entry{
			{Key: KeyOAuthGithubEnabled, Value: []byte(raw)},
		}})
		offSite, err := off.PublicSite(context.Background())
		if err != nil {
			t.Fatalf("PublicSite: %v", err)
		}
		if len(offSite.OAuthProviders) != 0 {
			t.Fatalf("expected no providers for %s, got %v", raw, offSite.OAuthProviders)
		}
	}
}
