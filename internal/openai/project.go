package openai

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/yoyooyooo/commandcode-bridge/internal/protocol"
)

type completionState struct {
	ID      string
	Model   string
	Created int64
}

func newCompletionState(model string) completionState {
	return completionState{ID: randomID("chatcmpl_"), Model: model, Created: time.Now().Unix()}
}

func bufferedChatResponse(state completionState, out protocol.Outcome) map[string]any {
	message := map[string]any{"role": "assistant", "content": out.Text}
	if out.Reasoning != "" {
		message["reasoning_content"] = out.Reasoning
	}
	if len(out.Tools) > 0 {
		calls := make([]map[string]any, 0, len(out.Tools))
		for _, tool := range out.Tools {
			calls = append(calls, map[string]any{
				"id": tool.ID, "type": "function",
				"function": map[string]any{"name": tool.Name, "arguments": protocol.EncodeArguments(tool.Arguments)},
			})
		}
		message["tool_calls"] = calls
	}
	return map[string]any{
		"id": state.ID, "object": "chat.completion", "created": state.Created, "model": state.Model,
		"choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": out.FinishReason}},
		"usage":   usageMap(out.Usage),
	}
}

func chatChunk(state completionState, delta map[string]any, finishReason *string, usage any) map[string]any {
	choice := map[string]any{"index": 0, "delta": delta, "finish_reason": finishReason}
	chunk := map[string]any{
		"id": state.ID, "object": "chat.completion.chunk", "created": state.Created, "model": state.Model,
		"choices": []any{choice},
	}
	if usage != nil {
		chunk["usage"] = usage
	}
	return chunk
}

func usageMap(usage protocol.Usage) map[string]any {
	out := map[string]any{
		"prompt_tokens":     usage.PromptTokens,
		"completion_tokens": usage.CompletionTokens,
		"total_tokens":      usage.TotalTokens,
	}
	if usage.CachedTokens > 0 {
		out["prompt_tokens_details"] = map[string]any{"cached_tokens": usage.CachedTokens}
	}
	return out
}

func emitSSE(w io.Writer, flusher http.Flusher, value any) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", encoded)
	flusher.Flush()
}

func emitDone(w io.Writer, flusher http.Flusher) {
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func copyUpstreamHeaders(dst http.Header, src http.Header) {
	for _, key := range []string{"Retry-After", "X-Request-Id", "Request-Id", "X-Command-Code-Request-Id"} {
		if value := src.Get(key); value != "" {
			dst.Set(key, value)
		}
	}
}

func randomID(prefix string) string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%s%d", prefix, time.Now().UnixNano())
	}
	return prefix + hex.EncodeToString(buf)
}
