package openai

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConvertDeveloperAndToolHistory(t *testing.T) {
	input := ChatRequest{
		Model: "deepseek/deepseek-v4-flash",
		Messages: []ChatMessage{
			{Role: "developer", Content: json.RawMessage(`"dev rules"`)},
			{Role: "user", Content: json.RawMessage(`"hi"`)},
			{
				Role: "assistant",
				ToolCalls: []ChatToolCall{{
					ID: "call_1", Type: "function", Function: struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					}{Name: "lookup", Arguments: `{"q":"x"}`},
				}},
			},
			{Role: "tool", ToolCallID: "call_1", Content: json.RawMessage(`"ok"`)},
		},
		MaxCompletionTokens: 32,
	}
	req, err := buildCommandCodeRequest(input, "/tmp")
	if err != nil {
		t.Fatal(err)
	}
	if req.Params.System != "dev rules" || req.Params.MaxTokens != 32 || !req.Params.Stream {
		t.Fatalf("%+v", req.Params)
	}
	if len(req.Params.Messages) != 3 {
		t.Fatalf("messages=%d", len(req.Params.Messages))
	}
}

func TestConvertRejectsRemoteImage(t *testing.T) {
	input := ChatRequest{
		Model: "m",
		Messages: []ChatMessage{{
			Role:    "user",
			Content: json.RawMessage(`[{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}]`),
		}},
	}
	_, err := buildCommandCodeRequest(input, "/tmp")
	if err == nil || !strings.Contains(err.Error(), "remote images") {
		t.Fatalf("err=%v", err)
	}
}

func TestConvertAcceptsDataImage(t *testing.T) {
	input := ChatRequest{
		Model: "m",
		Messages: []ChatMessage{{
			Role:    "user",
			Content: json.RawMessage(`[{"type":"image_url","image_url":{"url":"data:image/png;base64,aaaa"}}]`),
		}},
	}
	req, err := buildCommandCodeRequest(input, "/tmp")
	if err != nil {
		t.Fatal(err)
	}
	content := req.Params.Messages[0]["content"].([]map[string]any)
	if content[0]["type"] != "image" || content[0]["mediaType"] != "image/png" {
		t.Fatalf("%#v", content)
	}
}
