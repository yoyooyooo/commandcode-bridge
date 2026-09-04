package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

var (
	ErrIncompleteStream     = errors.New("command code stream ended without a terminal event")
	ErrAborted              = errors.New("command code stream aborted")
	ErrInvalidToolArgs      = errors.New("authoritative tool arguments must be a JSON object")
	ErrUnconfirmedToolCalls = errors.New("stream claimed tool calls without an authoritative tool-call")
)

type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	CachedTokens     int
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

type Outcome struct {
	Text         string
	Reasoning    string
	Tools        []ToolCall
	FinishReason string
	Usage        Usage
	TerminalKind string
	Complete     bool
}

type Reducer struct {
	text         strings.Builder
	reasoning    strings.Builder
	tools        []ToolCall
	toolIndex    map[string]int
	finishReason string
	usage        Usage
	terminalKind string
	complete     bool
	err          error
}

type StreamUpdate struct {
	Kind      string
	Text      string
	Reasoning string
	Tool      *ToolCall
	ToolIndex int
}

func NewReducer() *Reducer {
	return &Reducer{
		toolIndex:    make(map[string]int),
		finishReason: "stop",
	}
}

func Reduce(r io.Reader) (Outcome, error) {
	reducer := NewReducer()
	err := ScanEvents(r, func(event Event) error {
		_, reduceErr := reducer.Apply(event)
		return reduceErr
	})
	if err != nil {
		return Outcome{}, err
	}
	return reducer.Finish()
}

func (r *Reducer) Apply(event Event) (StreamUpdate, error) {
	if r.err != nil {
		return StreamUpdate{}, r.err
	}
	switch event.Type {
	case "reasoning-delta":
		text := stringField(event.Data, "text")
		r.reasoning.WriteString(text)
		return StreamUpdate{Kind: "reasoning", Reasoning: text}, nil
	case "text-delta":
		text := stringField(event.Data, "text")
		r.text.WriteString(text)
		return StreamUpdate{Kind: "text", Text: text}, nil
	case "tool-input-start", "tool-input-delta", "tool-input-end":
		return StreamUpdate{Kind: "provisional-tool"}, nil
	case "tool-call":
		tool, err := authoritativeTool(event)
		if err != nil {
			r.err = err
			return StreamUpdate{}, err
		}
		if _, exists := r.toolIndex[tool.ID]; exists {
			return StreamUpdate{Kind: "duplicate-tool"}, nil
		}
		r.toolIndex[tool.ID] = len(r.tools)
		r.tools = append(r.tools, tool)
		idx := len(r.tools) - 1
		return StreamUpdate{Kind: "tool", Tool: &r.tools[idx], ToolIndex: idx}, nil
	case "finish-step":
		r.complete = true
		r.terminalKind = "finish-step"
		r.applyFinish(event)
		return StreamUpdate{Kind: "finish-step"}, nil
	case "finish":
		r.complete = true
		r.terminalKind = "finish"
		r.applyFinish(event)
		return StreamUpdate{Kind: "finish"}, nil
	case "abort":
		r.err = ErrAborted
		return StreamUpdate{}, r.err
	case "error":
		r.err = errors.New(firstNonEmpty(stringField(event.Data, "message"), nestedString(event.Data, "error", "message"), "command code stream error"))
		return StreamUpdate{}, r.err
	default:
		return StreamUpdate{Kind: event.Type}, nil
	}
}

func (r *Reducer) Finish() (Outcome, error) {
	if r.err != nil {
		return Outcome{}, r.err
	}
	if !r.complete {
		return Outcome{}, ErrIncompleteStream
	}
	reason := r.finishReason
	if len(r.tools) > 0 && reason != "length" {
		reason = "tool_calls"
	}
	if reason == "tool_calls" && len(r.tools) == 0 {
		return Outcome{}, ErrUnconfirmedToolCalls
	}
	return Outcome{
		Text:         r.text.String(),
		Reasoning:    r.reasoning.String(),
		Tools:        append([]ToolCall(nil), r.tools...),
		FinishReason: reason,
		Usage:        r.usage,
		TerminalKind: r.terminalKind,
		Complete:     true,
	}, nil
}

func (r *Reducer) applyFinish(event Event) {
	reason := firstNonEmpty(stringField(event.Data, "finishReason"), stringField(event.Data, "rawFinishReason"))
	r.finishReason = normalizeFinishReason(reason, len(r.tools) > 0)
	usage, _ := event.Data["usage"].(map[string]any)
	if usage == nil {
		usage, _ = event.Data["totalUsage"].(map[string]any)
	}
	if usage != nil {
		r.usage = parseUsage(usage)
	}
}

func authoritativeTool(event Event) (ToolCall, error) {
	id := firstNonEmpty(stringField(event.Data, "toolCallId"), stringField(event.Data, "id"))
	name := firstNonEmpty(stringField(event.Data, "toolName"), stringField(event.Data, "name"))
	raw, ok := rawObjectField(event.Raw, "input")
	if !ok {
		raw, ok = rawObjectField(event.Raw, "args")
	}
	if !ok {
		return ToolCall{}, ErrInvalidToolArgs
	}
	if id == "" {
		id = "call_missing"
	}
	return ToolCall{ID: id, Name: name, Arguments: raw}, nil
}

func rawObjectField(raw json.RawMessage, key string) (json.RawMessage, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var obj map[string]json.RawMessage
	if err := dec.Decode(&obj); err != nil {
		return nil, false
	}
	value, ok := obj[key]
	if !ok || len(value) == 0 || string(value) == "null" {
		return nil, false
	}
	trimmed := bytes.TrimSpace(value)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, false
	}
	probe := json.NewDecoder(bytes.NewReader(trimmed))
	probe.UseNumber()
	var asMap map[string]any
	if err := probe.Decode(&asMap); err != nil {
		return nil, false
	}
	return json.RawMessage(append([]byte(nil), trimmed...)), true
}

func parseUsage(usage map[string]any) Usage {
	prompt := intField(usage, "inputTokens", "input_tokens")
	completion := intField(usage, "outputTokens", "output_tokens")
	total := intField(usage, "totalTokens", "total_tokens")
	if total == 0 {
		total = prompt + completion
	}
	cached := intField(usage, "cachedInputTokens")
	if cached == 0 {
		if details, ok := usage["inputTokenDetails"].(map[string]any); ok {
			cached = intField(details, "cacheReadTokens")
		}
	}
	return Usage{PromptTokens: prompt, CompletionTokens: completion, TotalTokens: total, CachedTokens: cached}
}

func normalizeFinishReason(reason string, hasTools bool) string {
	switch reason {
	case "length":
		return "length"
	case "tool-calls", "tool_calls", "tool_use":
		return "tool_calls"
	}
	if hasTools {
		return "tool_calls"
	}
	return "stop"
}

func stringField(data map[string]any, key string) string {
	text, _ := data[key].(string)
	return text
}

func nestedString(data map[string]any, parent, key string) string {
	nested, _ := data[parent].(map[string]any)
	return stringField(nested, key)
}

func intField(data map[string]any, keys ...string) int {
	for _, key := range keys {
		switch value := data[key].(type) {
		case int:
			return value
		case json.Number:
			parsed, _ := value.Int64()
			return int(parsed)
		case float64:
			return int(value)
		}
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func EncodeArguments(raw json.RawMessage) string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "{}"
	}
	return string(raw)
}

func FormatError(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprint(err)
}
