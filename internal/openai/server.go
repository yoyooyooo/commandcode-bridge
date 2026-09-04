package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/yoyooyooo/commandcode-bridge/internal/auth"
	"github.com/yoyooyooo/commandcode-bridge/internal/catalog"
	"github.com/yoyooyooo/commandcode-bridge/internal/protocol"
	"github.com/yoyooyooo/commandcode-bridge/internal/runtime"
	"github.com/yoyooyooo/commandcode-bridge/internal/secret"
)

type Server struct {
	cfg     runtime.Config
	catalog *catalog.Catalog
	client  *http.Client
}

func New(cfg runtime.Config, models *catalog.Catalog, client *http.Client) *Server {
	if client == nil {
		client = http.DefaultClient
	}
	if models == nil {
		models = catalog.New(catalog.MustStatic())
	}
	return &Server{cfg: cfg, catalog: models, client: client}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /health", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)
	mux.HandleFunc("GET /v1/models", s.withClientAuth(s.handleModels))
	mux.HandleFunc("POST /v1/chat/completions", s.withClientAuth(s.handleChatCompletions))
	mux.HandleFunc("POST /v1/responses", s.handleUnsupportedResponses)
	return mux
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) handleReadyz(w http.ResponseWriter, _ *http.Request) {
	snap := s.catalog.Snapshot()
	status := s.catalog.ReadyState()
	code := http.StatusOK
	if s.cfg.UpstreamKey == "" {
		status = "not_ready"
		code = http.StatusServiceUnavailable
	}
	writeJSON(w, code, map[string]any{
		"status":               status,
		"catalog":              snap.Source,
		"listen":               s.cfg.ListenAddr,
		"command_code_version": s.cfg.CommandCodeVersion,
	})
}

func (s *Server) handleUnsupportedResponses(w http.ResponseWriter, _ *http.Request) {
	writeOpenAIError(w, http.StatusNotFound, "unsupported_endpoint", "Use /v1/chat/completions")
}

func (s *Server) handleModels(w http.ResponseWriter, _ *http.Request, _ auth.Client) {
	snap := s.catalog.Snapshot()
	models := make([]map[string]any, 0, len(snap.Models))
	for _, model := range snap.Models {
		models = append(models, map[string]any{
			"id": model.ID, "object": "model", "created": 0, "owned_by": firstNonEmpty(model.OwnedBy, "commandcode"),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": models})
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request, client auth.Client) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBodyBytes))
	if err != nil {
		writeOpenAIError(w, http.StatusRequestEntityTooLarge, "request_too_large", "Request body is too large")
		return
	}
	var input ChatRequest
	if err := json.Unmarshal(body, &input); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "Invalid JSON request")
		return
	}
	if strings.TrimSpace(input.Model) == "" || len(input.Messages) == 0 {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "model and messages are required")
		return
	}
	upstreamBody, err := buildCommandCodeRequest(input, s.cfg.WorkingDir)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	payload, err := json.Marshal(upstreamBody)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, "proxy_error", "Could not encode upstream request")
		return
	}

	upstreamReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, s.cfg.BaseURL+"/alpha/generate", bytes.NewReader(payload))
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, "proxy_error", "Could not create upstream request")
		return
	}
	upstreamReq.Header.Set("Authorization", "Bearer "+s.cfg.UpstreamKey)
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Accept", "application/x-ndjson")
	upstreamReq.Header.Set("x-cli-environment", "production")
	upstreamReq.Header.Set("x-command-code-version", s.cfg.CommandCodeVersion)
	upstreamReq.Header.Set("x-session-id", firstNonEmpty(r.Header.Get("x-session-id"), r.Header.Get("session_id"), randomID("session_")))
	upstreamReq.Header.Set("x-commandcode-proxy-client", client.Name)
	if s.cfg.ZDR {
		upstreamReq.Header.Set("x-cmd-zdr", "1")
	}

	resp, err := s.client.Do(upstreamReq)
	if err != nil {
		writeOpenAIError(w, http.StatusBadGateway, "upstream_error", "Command Code request failed")
		return
	}
	defer resp.Body.Close()
	copyUpstreamHeaders(w.Header(), resp.Header)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		s.writeUpstreamError(w, resp)
		return
	}

	if input.Stream {
		s.streamChat(w, r.Context(), resp.Body, input)
		return
	}
	s.bufferedChat(w, resp.Body, input.Model)
}

func (s *Server) bufferedChat(w http.ResponseWriter, body io.Reader, model string) {
	out, err := protocol.Reduce(body)
	if err != nil {
		writeOpenAIError(w, http.StatusBadGateway, "upstream_error", s.redact(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, bufferedChatResponse(newCompletionState(model), out))
}

func (s *Server) streamChat(w http.ResponseWriter, ctx context.Context, body io.Reader, input ChatRequest) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeOpenAIError(w, http.StatusInternalServerError, "proxy_error", "Streaming is unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	state := newCompletionState(input.Model)
	emitSSE(w, flusher, chatChunk(state, map[string]any{"role": "assistant"}, nil, nil))
	reducer := protocol.NewReducer()
	err := protocol.ScanEvents(body, func(event protocol.Event) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		update, applyErr := reducer.Apply(event)
		if applyErr != nil {
			return applyErr
		}
		switch update.Kind {
		case "reasoning":
			emitSSE(w, flusher, chatChunk(state, map[string]any{"reasoning_content": update.Reasoning}, nil, nil))
		case "text":
			emitSSE(w, flusher, chatChunk(state, map[string]any{"content": update.Text}, nil, nil))
		case "tool":
			if update.Tool == nil {
				return nil
			}
			delta := map[string]any{"tool_calls": []any{map[string]any{
				"index": update.ToolIndex, "id": update.Tool.ID, "type": "function",
				"function": map[string]any{"name": update.Tool.Name, "arguments": protocol.EncodeArguments(update.Tool.Arguments)},
			}}}
			emitSSE(w, flusher, chatChunk(state, delta, nil, nil))
		}
		return nil
	})
	if err != nil {
		emitSSE(w, flusher, map[string]any{"error": map[string]any{"type": "upstream_error", "code": "upstream_error", "message": s.redact(err.Error())}})
		return
	}
	out, err := reducer.Finish()
	if err != nil {
		emitSSE(w, flusher, map[string]any{"error": map[string]any{"type": "upstream_error", "code": "incomplete_stream", "message": s.redact(err.Error())}})
		return
	}
	var usage any
	if input.StreamOptions == nil || input.StreamOptions.IncludeUsage {
		usage = usageMap(out.Usage)
	}
	emitSSE(w, flusher, chatChunk(state, map[string]any{}, &out.FinishReason, usage))
	emitDone(w, flusher)
}

func (s *Server) writeUpstreamError(w http.ResponseWriter, resp *http.Response) {
	detail := readUpstreamError(resp.Body)
	status := resp.StatusCode
	if status < 400 {
		status = http.StatusBadGateway
	}
	if retry := resp.Header.Get("Retry-After"); retry != "" {
		w.Header().Set("Retry-After", retry)
	}
	writeOpenAIError(w, status, "upstream_error", s.redact(detail))
}

func (s *Server) withClientAuth(next func(http.ResponseWriter, *http.Request, auth.Client)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client, err := s.cfg.ClientStore.Authenticate(r)
		if err != nil {
			writeOpenAIError(w, http.StatusUnauthorized, "invalid_api_key", "A valid Proxy Client Key is required")
			return
		}
		next(w, r, client)
	}
}

func (s *Server) redact(message string) string {
	secrets := []string{s.cfg.UpstreamKey}
	if s.cfg.ClientStore != nil {
		secrets = append(secrets, s.cfg.ClientStore.Secrets()...)
	}
	return secret.Redact(message, secrets...)
}

func readUpstreamError(body io.Reader) string {
	raw, _ := io.ReadAll(io.LimitReader(body, 1<<20))
	var value map[string]any
	if json.Unmarshal(raw, &value) == nil {
		if message := firstNonEmpty(stringField(value, "message"), nestedString(value, "error", "message"), stringField(value, "detail")); message != "" {
			return message
		}
	}
	if strings.TrimSpace(string(raw)) != "" {
		return "Command Code rejected the request"
	}
	return "Command Code rejected the request"
}

func writeOpenAIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"message": message, "type": "invalid_request_error", "code": code}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func stringField(value map[string]any, key string) string {
	text, _ := value[key].(string)
	return text
}

func nestedString(value map[string]any, parent, key string) string {
	nested, _ := value[parent].(map[string]any)
	return stringField(nested, key)
}
