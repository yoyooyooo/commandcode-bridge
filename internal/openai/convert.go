package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime"
	"strings"
	"time"

	"github.com/yoyooyooo/commandcode-bridge/internal/protocol"
)

const maxRequestBodyBytes = 16 << 20

type ChatRequest struct {
	Model               string         `json:"model"`
	Messages            []ChatMessage  `json:"messages"`
	Tools               []ChatTool     `json:"tools"`
	MaxTokens           int            `json:"max_tokens"`
	MaxCompletionTokens int            `json:"max_completion_tokens"`
	Temperature         *float64       `json:"temperature"`
	Stream              bool           `json:"stream"`
	StreamOptions       *StreamOptions `json:"stream_options"`
}

type StreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type ChatMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	Name       string          `json:"name,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	ToolCalls  []ChatToolCall  `json:"tool_calls,omitempty"`
}

type ChatToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type ChatTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description,omitempty"`
		Parameters  json.RawMessage `json:"parameters,omitempty"`
	} `json:"function"`
}

type commandCodeRequest struct {
	Config         commandCodeConfig `json:"config"`
	Memory         string            `json:"memory"`
	Taste          any               `json:"taste"`
	Skills         any               `json:"skills"`
	PermissionMode string            `json:"permissionMode"`
	Params         commandCodeParams `json:"params"`
}

type commandCodeConfig struct {
	WorkingDir    string   `json:"workingDir"`
	Date          string   `json:"date"`
	Environment   string   `json:"environment"`
	Structure     []string `json:"structure"`
	IsGitRepo     bool     `json:"isGitRepo"`
	CurrentBranch string   `json:"currentBranch"`
	MainBranch    string   `json:"mainBranch"`
	GitStatus     string   `json:"gitStatus"`
	RecentCommits []string `json:"recentCommits"`
}

type commandCodeParams struct {
	Model       string           `json:"model"`
	System      string           `json:"system"`
	Messages    []map[string]any `json:"messages"`
	Tools       []map[string]any `json:"tools"`
	MaxTokens   int              `json:"max_tokens"`
	Temperature *float64         `json:"temperature,omitempty"`
	Stream      bool             `json:"stream"`
}

func buildCommandCodeRequest(input ChatRequest, workingDir string) (commandCodeRequest, error) {
	system, messages, err := convertChatMessages(input.Messages)
	if err != nil {
		return commandCodeRequest{}, err
	}
	tools := make([]map[string]any, 0, len(input.Tools))
	for _, tool := range input.Tools {
		if tool.Type != "" && tool.Type != "function" {
			continue
		}
		if strings.TrimSpace(tool.Function.Name) == "" {
			continue
		}
		params := map[string]any{"type": "object", "properties": map[string]any{}}
		if len(bytes.TrimSpace(tool.Function.Parameters)) > 0 {
			dec := json.NewDecoder(bytes.NewReader(tool.Function.Parameters))
			dec.UseNumber()
			if err := dec.Decode(&params); err != nil {
				return commandCodeRequest{}, fmt.Errorf("invalid tool parameters for %s", tool.Function.Name)
			}
		}
		tools = append(tools, map[string]any{
			"name": tool.Function.Name, "description": tool.Function.Description, "input_schema": params,
		})
	}
	maxTokens := input.MaxCompletionTokens
	if maxTokens <= 0 {
		maxTokens = input.MaxTokens
	}
	if maxTokens <= 0 {
		maxTokens = 8192
	}
	return commandCodeRequest{
		Config: commandCodeConfig{
			WorkingDir: workingDir, Date: time.Now().UTC().Format("2006-01-02"), Environment: "production",
			Structure: []string{}, RecentCommits: []string{},
		},
		Memory: "", Taste: nil, Skills: nil, PermissionMode: "standard",
		Params: commandCodeParams{
			Model: input.Model, System: system, Messages: messages, Tools: tools,
			MaxTokens: maxTokens, Temperature: input.Temperature, Stream: true,
		},
	}, nil
}

func convertChatMessages(input []ChatMessage) (string, []map[string]any, error) {
	var systemParts []string
	messages := make([]map[string]any, 0, len(input))
	toolNames := make(map[string]string)

	for _, message := range input {
		switch message.Role {
		case "system", "developer":
			text, err := textFromContent(message.Content)
			if err != nil {
				return "", nil, err
			}
			if text != "" {
				systemParts = append(systemParts, text)
			}
		case "user":
			content, err := userContent(message.Content)
			if err != nil {
				return "", nil, err
			}
			messages = append(messages, map[string]any{"role": "user", "content": content})
		case "assistant":
			content := make([]map[string]any, 0, len(message.ToolCalls)+1)
			text, err := textFromContent(message.Content)
			if err != nil {
				return "", nil, err
			}
			if text != "" {
				content = append(content, map[string]any{"type": "text", "text": text})
			}
			for _, call := range message.ToolCalls {
				arguments, err := decodeToolArguments(call.Function.Arguments)
				if err != nil {
					return "", nil, err
				}
				toolNames[call.ID] = call.Function.Name
				content = append(content, map[string]any{
					"type": "tool-call", "toolCallId": call.ID, "toolName": call.Function.Name, "input": arguments,
				})
			}
			if len(content) > 0 {
				messages = append(messages, map[string]any{"role": "assistant", "content": content})
			}
		case "tool":
			text, err := textFromContent(message.Content)
			if err != nil {
				return "", nil, err
			}
			result := map[string]any{
				"type": "tool-result", "toolCallId": message.ToolCallID,
				"toolName": firstNonEmpty(message.Name, toolNames[message.ToolCallID], "unknown"),
				"output":   map[string]any{"type": "text", "value": text},
			}
			if len(messages) > 0 && messages[len(messages)-1]["role"] == "tool" {
				existing := messages[len(messages)-1]["content"].([]map[string]any)
				messages[len(messages)-1]["content"] = append(existing, result)
			} else {
				messages = append(messages, map[string]any{"role": "tool", "content": []map[string]any{result}})
			}
		default:
			return "", nil, fmt.Errorf("unsupported message role %q", message.Role)
		}
	}
	return strings.Join(systemParts, "\n\n"), messages, nil
}

func decodeToolArguments(raw string) (map[string]any, error) {
	payload := strings.TrimSpace(raw)
	if payload == "" {
		payload = "{}"
	}
	dec := json.NewDecoder(strings.NewReader(payload))
	dec.UseNumber()
	var arguments map[string]any
	if err := dec.Decode(&arguments); err != nil {
		return nil, fmt.Errorf("%w", protocol.ErrInvalidToolArgs)
	}
	return arguments, nil
}

func textFromContent(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text, nil
	}
	var parts []map[string]any
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", fmt.Errorf("message content must be a string or content-part array")
	}
	var out strings.Builder
	for _, part := range parts {
		typ, _ := part["type"].(string)
		if typ == "text" || typ == "input_text" || typ == "output_text" {
			if value, _ := part["text"].(string); value != "" {
				out.WriteString(value)
			}
		}
	}
	return out.String(), nil
}

func userContent(raw json.RawMessage) ([]map[string]any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return []map[string]any{}, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return []map[string]any{{"type": "text", "text": text}}, nil
	}
	var parts []map[string]any
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, fmt.Errorf("user content must be a string or content-part array")
	}
	out := make([]map[string]any, 0, len(parts))
	for _, part := range parts {
		typ, _ := part["type"].(string)
		switch typ {
		case "text", "input_text":
			if value, _ := part["text"].(string); value != "" {
				out = append(out, map[string]any{"type": "text", "text": value})
			}
		case "image_url", "input_image":
			url := imageURL(part)
			if url == "" {
				continue
			}
			if !strings.HasPrefix(url, "data:") {
				return nil, fmt.Errorf("remote images are not allowed")
			}
			image := map[string]any{"type": "image", "image": url}
			if mediaType := mediaTypeFromDataURL(url); mediaType != "" {
				image["mediaType"] = mediaType
			}
			out = append(out, image)
		}
	}
	return out, nil
}

func imageURL(part map[string]any) string {
	switch value := part["image_url"].(type) {
	case string:
		return value
	case map[string]any:
		url, _ := value["url"].(string)
		return url
	}
	url, _ := part["image"].(string)
	return url
}

func mediaTypeFromDataURL(value string) string {
	if !strings.HasPrefix(value, "data:") {
		return ""
	}
	end := strings.IndexByte(value, ',')
	if end < 0 {
		return ""
	}
	header := strings.TrimPrefix(value[:end], "data:")
	if semi := strings.IndexByte(header, ';'); semi >= 0 {
		header = header[:semi]
	}
	mediaType, _, err := mime.ParseMediaType(header)
	if err != nil {
		return strings.TrimSpace(header)
	}
	return mediaType
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
