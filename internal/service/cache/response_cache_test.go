package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestService(t *testing.T, cfg ServiceConfig) (*Service, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })
	return New(client, cfg), mr
}

func TestBuildKey_DeterministicAndSensitive(t *testing.T) {
	body := map[string]any{"messages": []any{map[string]any{"role": "user", "content": "hi"}}}
	k1 := BuildKey("m1", body, "tenant:user")
	k2 := BuildKey("m1", body, "tenant:user")
	if k1 != k2 {
		t.Fatalf("BuildKey not deterministic: %q vs %q", k1, k2)
	}
	if BuildKey("m2", body, "tenant:user") == k1 {
		t.Error("model change must change the key")
	}
	if BuildKey("m1", body, "tenant:other") == k1 {
		t.Error("scope change must change the key")
	}
	bodyWithTemp := map[string]any{"messages": body["messages"], "temperature": 0.7}
	if BuildKey("m1", bodyWithTemp, "tenant:user") == k1 {
		t.Error("sampling parameter change must change the key")
	}
}

func TestService_DisabledWithoutClient(t *testing.T) {
	s := New(nil, ServiceConfig{})
	if s.IsEnabled() {
		t.Fatal("nil client must disable caching")
	}
	if got, err := s.Get(context.Background(), "cache:response:x"); got != nil || err != nil {
		t.Fatalf("Get on disabled service = (%v, %v), want (nil, nil)", got, err)
	}
	if err := s.Set(context.Background(), "cache:response:x", &CachedResponse{Body: "{}"}); err != nil {
		t.Fatalf("Set on disabled service: %v", err)
	}
}

func TestService_SetGetRoundTrip(t *testing.T) {
	s, _ := newTestService(t, ServiceConfig{TTL: time.Hour})

	cr := &CachedResponse{StatusCode: 200, Body: `{"choices":[]}`, InputTokens: 10, OutputTokens: 20, Model: "m1"}
	if err := s.Set(context.Background(), "cache:response:abc", cr); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := s.Get(context.Background(), "cache:response:abc")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil on hit")
	}
	if *got != *cr {
		t.Fatalf("round trip mismatch: %+v vs %+v", got, cr)
	}
}

func TestService_GetMiss(t *testing.T) {
	s, _ := newTestService(t, ServiceConfig{})
	got, err := s.Get(context.Background(), "cache:response:missing")
	if err != nil || got != nil {
		t.Fatalf("Get on miss = (%v, %v), want (nil, nil)", got, err)
	}
}

func TestService_DefaultTTLWhenUnset(t *testing.T) {
	s, mr := newTestService(t, ServiceConfig{})
	if err := s.Set(context.Background(), "cache:response:ttl", &CachedResponse{Body: "{}"}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	ttl := mr.TTL("cache:response:ttl")
	if ttl < 55*time.Minute || ttl > time.Hour {
		t.Fatalf("default TTL = %v, want ~1h", ttl)
	}
}

func TestService_IsModelAccepted(t *testing.T) {
	s := New(nil, ServiceConfig{})
	if !s.IsModelAccepted("anything") {
		t.Fatal("empty AcceptedModels must accept all models")
	}
	s2 := New(nil, ServiceConfig{AcceptedModels: map[string]bool{"m1": true}})
	if !s2.IsModelAccepted("m1") {
		t.Fatal("configured model must be accepted")
	}
	if s2.IsModelAccepted("m2") {
		t.Fatal("unconfigured model must be rejected")
	}
}
