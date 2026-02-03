package claudecode

import (
	"bytes"
	"encoding/json"
	"strings"
)

// StreamEventType enumerates event types in Claude Code's stream-json output
type StreamEventType string

const (
	EventSystem    StreamEventType = "system"
	EventAssistant StreamEventType = "assistant"
	EventUser      StreamEventType = "user"
	EventResult    StreamEventType = "result"
)

// ContentBlockType enumerates content block types within messages
type ContentBlockType string

const (
	BlockText       ContentBlockType = "text"
	BlockThinking   ContentBlockType = "thinking"
	BlockToolUse    ContentBlockType = "tool_use"
	BlockToolResult ContentBlockType = "tool_result"
)

// TokenUsage holds token usage counts from a result event.
type TokenUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// StreamEvent is a single high-level event extracted from the NDJSON stream.
type StreamEvent struct {
	Type       StreamEventType  `json:"type"`
	Subtype    ContentBlockType `json:"subtype,omitempty"`
	Content    string           `json:"content,omitempty"`
	ToolName   string           `json:"tool_name,omitempty"`
	ToolInput  json.RawMessage  `json:"tool_input,omitempty"`
	Tokens     *TokenUsage      `json:"usage,omitempty"`
	StopReason string           `json:"stop_reason,omitempty"`
}

// ParseResult holds all parsed events and aggregated metadata.
type ParseResult struct {
	Events      []StreamEvent
	TextContent string
	TotalTokens *TokenUsage
	StopReason  string
}

// MaxThinkingBytes is the truncation limit for thinking content (Cloud Logging 64KB limit).
const MaxThinkingBytes = 50000

// rawContentBlock is an intermediate representation of a content block in a message.
type rawContentBlock struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	Thinking string          `json:"thinking,omitempty"`
	Name     string          `json:"name,omitempty"`
	Input    json.RawMessage `json:"input,omitempty"`
	Content  interface{}     `json:"content,omitempty"`
}

// rawEvent is the top-level NDJSON line structure.
type rawEvent struct {
	Type    string          `json:"type"`
	Subtype string          `json:"subtype,omitempty"`
	Message json.RawMessage `json:"message,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
}

// rawMessage holds the message body with content blocks.
type rawMessage struct {
	Content []rawContentBlock `json:"content"`
}

// rawResult holds the result body with content, usage, and stop reason.
type rawResult struct {
	Content    []rawContentBlock `json:"content"`
	Usage      *TokenUsage       `json:"usage,omitempty"`
	StopReason string            `json:"stop_reason,omitempty"`
}

// ParseStreamJSON parses NDJSON output from Claude Code's stream-json format.
// Malformed lines are silently skipped.
func ParseStreamJSON(data []byte) *ParseResult {
	result := &ParseResult{}
	var textParts [][]byte

	lines := bytes.Split(data, []byte("\n"))
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		var evt rawEvent
		if err := json.Unmarshal(line, &evt); err != nil {
			continue // skip malformed lines
		}

		evtType := StreamEventType(evt.Type)

		switch evtType {
		case EventAssistant, EventUser:
			var msg rawMessage
			if err := json.Unmarshal(evt.Message, &msg); err != nil {
				continue
			}
			extractBlocks(evtType, msg.Content, result, &textParts)

		case EventResult:
			var res rawResult
			if err := json.Unmarshal(evt.Result, &res); err != nil {
				continue
			}
			extractBlocks(evtType, res.Content, result, &textParts)
			if res.Usage != nil {
				result.TotalTokens = res.Usage
			}
			if res.StopReason != "" {
				result.StopReason = res.StopReason
			}

		case EventSystem:
			result.Events = append(result.Events, StreamEvent{
				Type:    EventSystem,
				Subtype: ContentBlockType(evt.Subtype),
			})
		}
	}

	result.TextContent = string(bytes.Join(textParts, []byte("\n")))
	return result
}

// extractBlocks processes content blocks from a message or result, appending
// StreamEvents and accumulating text content.
func extractBlocks(evtType StreamEventType, blocks []rawContentBlock, result *ParseResult, textParts *[][]byte) {
	for _, block := range blocks {
		blockType := ContentBlockType(block.Type)
		switch blockType {
		case BlockText:
			se := StreamEvent{
				Type:    evtType,
				Subtype: BlockText,
				Content: block.Text,
			}
			result.Events = append(result.Events, se)
			if block.Text != "" {
				*textParts = append(*textParts, []byte(block.Text))
			}

		case BlockThinking:
			content := block.Thinking
			if len(content) > MaxThinkingBytes {
				content = content[:MaxThinkingBytes]
			}
			result.Events = append(result.Events, StreamEvent{
				Type:    evtType,
				Subtype: BlockThinking,
				Content: content,
			})

		case BlockToolUse:
			result.Events = append(result.Events, StreamEvent{
				Type:      evtType,
				Subtype:   BlockToolUse,
				ToolName:  block.Name,
				ToolInput: block.Input,
			})

		case BlockToolResult:
			content := blockContentToString(block.Content)
			se := StreamEvent{
				Type:    evtType,
				Subtype: BlockToolResult,
				Content: content,
			}
			result.Events = append(result.Events, se)
			if content != "" {
				*textParts = append(*textParts, []byte(content))
			}
		}
	}
}

// ExtractAssistantText returns only text from assistant messages, excluding tool results.
// This is suitable for GitHub comments where tool output adds noise.
func (pr *ParseResult) ExtractAssistantText() string {
	var parts []string
	for _, evt := range pr.Events {
		if evt.Type == EventAssistant && evt.Subtype == BlockText && evt.Content != "" {
			parts = append(parts, evt.Content)
		}
	}
	return strings.Join(parts, "\n")
}

// blockContentToString converts a content field (which may be a string or array) to a string.
func blockContentToString(content interface{}) string {
	if content == nil {
		return ""
	}
	switch v := content.(type) {
	case string:
		return v
	case []interface{}:
		// Array of content blocks with "text" fields
		var parts []string
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				if text, ok := m["text"].(string); ok && text != "" {
					parts = append(parts, text)
				}
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
		// Fallback: marshal the array as JSON
		data, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(data)
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(data)
	}
}
