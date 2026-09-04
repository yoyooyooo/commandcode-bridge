package openai

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/yoyooyooo/commandcode-bridge/internal/auth"
	"github.com/yoyooyooo/commandcode-bridge/internal/catalog"
	rt "github.com/yoyooyooo/commandcode-bridge/internal/runtime"
)

func testServer(t *testing.T, upstream http.Handler) *httptest.Server {
	t.Helper()
	up := httptest.NewServer(upstream)
	t.Cleanup(up.Close)
	store, err := auth.NewStore([]auth.Client{{Name: "scripts", Key: "sk_scripts"}})
	if err != nil {
		t.Fatal(err)
	}
	proxy := New(rt.Config{
		ListenAddr:         "127.0.0.1:8788",
		BaseURL:            up.URL,
		CommandCodeVersion: "0.52.1",
		UpstreamKey:        "sk_upstream",
		ClientStore:        store,
		WorkingDir:         t.TempDir(),
	}, catalog.New(catalog.MustStatic()), up.Client())
	server := httptest.NewServer(proxy.Handler())
	t.Cleanup(server.Close)
	return server
}

func TestHealthzUnauthenticated(t *testing.T) {
	server := testServer(t, http.NotFoundHandler())
	resp, err := http.Get(server.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func TestModelsRequiresClientKey(t *testing.T) {
	server := testServer(t, http.NotFoundHandler())
	resp, err := http.Get(server.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/models", nil)
	req.Header.Set("Authorization", "Bearer sk_scripts")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), "sk_upstream") || strings.Contains(string(body), "sk_scripts") {
		t.Fatalf("secret leaked in models: %s", body)
	}
}

func TestChatUsesUpstreamKeyNotClientKey(t *testing.T) {
	var gotAuth, gotVersion string
	server := testServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotVersion = r.Header.Get("x-command-code-version")
		w.Header().Set("Retry-After", "7")
		w.Header().Set("X-Request-Id", "req_123")
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, fixture(t, "text-stop.ndjson"))
	}))
	resp := postChat(t, server.URL, false)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	if gotAuth != "Bearer sk_upstream" {
		t.Fatalf("upstream auth=%q", gotAuth)
	}
	if gotVersion != "0.52.1" {
		t.Fatalf("version=%q", gotVersion)
	}
	if resp.Header.Get("Retry-After") != "7" || resp.Header.Get("X-Request-Id") != "req_123" {
		t.Fatalf("headers=%v", resp.Header)
	}
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	choice := result["choices"].([]any)[0].(map[string]any)
	message := choice["message"].(map[string]any)
	if message["content"] != "PONG" {
		t.Fatalf("message=%#v", message)
	}
}

func TestStreamingAuthoritativeToolCallAndNoDoneOnFailure(t *testing.T) {
	server := testServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, fixture(t, "tool-call-authoritative.ndjson"))
	}))
	resp := postChat(t, server.URL, true)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	text := string(body)
	if !strings.Contains(text, `"tool_calls"`) || !strings.Contains(text, "data: [DONE]") {
		t.Fatalf("body=%s", text)
	}
	if !strings.Contains(text, `"q":"x"`) && !strings.Contains(text, `"q\":\"x\"`) {
		t.Fatalf("missing args in %s", text)
	}

	failing := testServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, fixture(t, "eof-no-terminal.ndjson"))
	}))
	resp = postChat(t, failing.URL, true)
	defer resp.Body.Close()
	body, _ = io.ReadAll(resp.Body)
	if strings.Contains(string(body), "data: [DONE]") {
		t.Fatalf("incomplete stream emitted [DONE]: %s", body)
	}
}

func TestProvisionalToolsAreNotExecutable(t *testing.T) {
	server := testServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, fixture(t, "tool-input-only.ndjson"))
	}))
	resp := postChat(t, server.URL, false)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
}

func TestResponsesUnsupported(t *testing.T) {
	server := testServer(t, http.NotFoundHandler())
	resp, err := http.Post(server.URL+"/v1/responses", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func TestRemoteImageRejected(t *testing.T) {
	server := testServer(t, http.NotFoundHandler())
	payload := `{"model":"deepseek/deepseek-v4-flash","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.com/x.png"}}]}]}`
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer sk_scripts")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func TestMaxCompletionTokensWins(t *testing.T) {
	var maxTokens float64
	server := testServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(bufio.NewReader(r.Body)).Decode(&payload); err != nil {
			t.Errorf("decode: %v", err)
		}
		params := payload["params"].(map[string]any)
		maxTokens, _ = params["max_tokens"].(float64)
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, fixture(t, "text-stop.ndjson"))
	}))
	payload := `{"model":"deepseek/deepseek-v4-flash","max_tokens":11,"max_completion_tokens":22,"messages":[{"role":"user","content":"hi"}]}`
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer sk_scripts")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if maxTokens != 22 {
		t.Fatalf("max_tokens=%v", maxTokens)
	}
}

func TestUpstreamErrorRedactsSecrets(t *testing.T) {
	server := testServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"bad key sk_upstream"}}`)
	}))
	resp := postChat(t, server.URL, false)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), "sk_upstream") {
		t.Fatalf("upstream key leaked: %s", body)
	}
}

func postChat(t *testing.T, baseURL string, stream bool) *http.Response {
	t.Helper()
	payload := map[string]any{
		"model": "deepseek/deepseek-v4-flash",
		"messages": []any{
			map[string]any{"role": "system", "content": "system prompt"},
			map[string]any{"role": "user", "content": "Reply PONG"},
		},
		"tools": []any{map[string]any{
			"type":     "function",
			"function": map[string]any{"name": "lookup", "parameters": map[string]any{"type": "object"}},
		}},
		"stream": stream,
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/v1/chat/completions", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer sk_scripts")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "protocol", "fixtures", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
