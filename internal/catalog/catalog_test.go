package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEmptyLiveListUsesStaticFallback(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/provider/v1/models" {
			t.Errorf("path=%s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
	}))
	defer upstream.Close()

	cat := New([]Model{{ID: "deepseek/deepseek-v4-flash"}})
	if err := cat.Refresh(context.Background(), upstream.Client(), upstream.URL, "upstream-key"); err == nil {
		t.Fatal("empty live catalog should fail")
	}
	snap := cat.Snapshot()
	if snap.Source != "static-fallback" || len(snap.Models) != 1 || snap.Models[0].ID != "deepseek/deepseek-v4-flash" {
		t.Fatalf("%+v", snap)
	}
	if cat.ReadyState() != "degraded" {
		t.Fatalf("ready=%s", cat.ReadyState())
	}
}

func TestLiveSuccessThenFailureKeepsLastKnownGood(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			_, _ = w.Write([]byte(`{"data":[{"id":"live-model"}]}`))
			return
		}
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"nope"}`))
	}))
	defer upstream.Close()

	cat := New([]Model{{ID: "static-model"}})
	if err := cat.Refresh(context.Background(), upstream.Client(), upstream.URL, "upstream-key"); err != nil {
		t.Fatal(err)
	}
	if src := cat.Snapshot().Source; src != "live" {
		t.Fatalf("source=%s", src)
	}
	if err := cat.Refresh(context.Background(), upstream.Client(), upstream.URL, "upstream-key"); err == nil {
		t.Fatal("expected refresh failure")
	}
	snap := cat.Snapshot()
	if snap.Source != "last-known-good" || snap.Models[0].ID != "live-model" {
		t.Fatalf("%+v", snap)
	}
}

func TestLoadStaticRoster(t *testing.T) {
	models := MustStatic()
	if len(models) < 20 {
		t.Fatalf("static roster too small: %d", len(models))
	}
	onDisk, err := LoadStatic(DefaultStaticPath())
	if err != nil {
		t.Fatal(err)
	}
	if len(onDisk) != len(models) {
		t.Fatalf("embedded static catalog drifted from protocol SSoT")
	}
}
