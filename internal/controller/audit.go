package controller

import (
	"encoding/json"
	"fmt"

	"github.com/andywolf/agentium/internal/agent/claudecode"
	"github.com/andywolf/agentium/internal/agent/codex"
	"github.com/andywolf/agentium/internal/audit"
	"github.com/andywolf/agentium/internal/cloud/gcp"
)

// emitAuditEvents extracts security-relevant events from agent output and logs
// them to Cloud Logging at INFO severity. Each event includes labels for
// audit_category, tool_name, task_id, and agent.
func (c *Controller) emitAuditEvents(events []interface{}, agentName string) {
	if c.cloudLogger == nil {
		return
	}

	taskID := fmt.Sprintf("%s:%s", c.activeTaskType, c.activeTask)

	var auditEvents []audit.Event

	switch agentName {
	case "claudecode":
		auditEvents = extractFromClaudeCodeEvents(events, agentName, taskID)
	case "codex":
		auditEvents = extractFromCodexEvents(events, agentName, taskID)
	default:
		// Unknown agent, try Claude Code format as default
		auditEvents = extractFromClaudeCodeEvents(events, agentName, taskID)
	}

	// Emit each audit event to Cloud Logging
	for _, evt := range auditEvents {
		labels := map[string]string{
			"audit_category": string(evt.Category),
			"tool_name":      evt.ToolName,
			"task_id":        evt.TaskID,
			"agent":          evt.Agent,
		}

		msg := evt.Message
		if len(msg) > 2000 {
			msg = msg[:2000] + "...(truncated)"
		}

		c.cloudLogger.LogWithLabels(gcp.SeverityInfo, msg, labels)
	}
}

// extractFromClaudeCodeEvents processes Claude Code StreamEvent objects
func extractFromClaudeCodeEvents(events []interface{}, agentName, taskID string) []audit.Event {
	var auditEvents []audit.Event

	for _, evt := range events {
		se, ok := evt.(claudecode.StreamEvent)
		if !ok {
			continue
		}

		if se.Subtype != claudecode.BlockToolUse {
			continue
		}

		auditEvents = append(auditEvents, extractFromTool(se.ToolName, se.ToolInput, agentName, taskID)...)
	}

	return auditEvents
}

// extractFromCodexEvents processes Codex event objects
func extractFromCodexEvents(events []interface{}, agentName, taskID string) []audit.Event {
	var codexEvents []audit.CodexEventData

	for _, evt := range events {
		ce, ok := evt.(codex.CodexEvent)
		if !ok {
			continue
		}

		if ce.Type != "item.completed" || ce.Item == nil {
			continue
		}

		switch ce.Item.Type {
		case "command_execution":
			codexEvents = append(codexEvents, audit.CodexEventData{
				Type:    "command_execution",
				Command: ce.Item.Command,
			})
		case "file_change":
			codexEvents = append(codexEvents, audit.CodexEventData{
				Type:     "file_change",
				FilePath: ce.Item.FilePath,
				Action:   ce.Item.Action,
			})
		}
	}

	return audit.ExtractFromCodexEvents(codexEvents, agentName, taskID)
}

// extractFromTool extracts audit events from a single Claude Code tool invocation
func extractFromTool(toolName string, toolInput json.RawMessage, agentName, taskID string) []audit.Event {
	var auditEvents []audit.Event

	switch toolName {
	case "Bash":
		var input audit.BashInput
		if err := json.Unmarshal(toolInput, &input); err == nil && input.Command != "" {
			categories := audit.ClassifyBashCommand(input.Command)
			for _, cat := range categories {
				auditEvents = append(auditEvents, audit.Event{
					Category: cat,
					ToolName: toolName,
					Agent:    agentName,
					TaskID:   taskID,
					Message:  input.Command,
				})
			}
		}

	case "Write":
		var input audit.WriteInput
		if err := json.Unmarshal(toolInput, &input); err == nil && input.FilePath != "" {
			if audit.IsSensitivePath(input.FilePath) {
				auditEvents = append(auditEvents, audit.Event{
					Category: audit.SensitiveFileWrite,
					ToolName: toolName,
					Agent:    agentName,
					TaskID:   taskID,
					Message:  input.FilePath,
				})
			}
		}

	case "Edit":
		var input audit.EditInput
		if err := json.Unmarshal(toolInput, &input); err == nil && input.FilePath != "" {
			if audit.IsSensitivePath(input.FilePath) {
				auditEvents = append(auditEvents, audit.Event{
					Category: audit.SensitiveFileWrite,
					ToolName: toolName,
					Agent:    agentName,
					TaskID:   taskID,
					Message:  input.FilePath,
				})
			}
		}

	case "WebFetch":
		var input audit.WebFetchInput
		if err := json.Unmarshal(toolInput, &input); err == nil && input.URL != "" {
			auditEvents = append(auditEvents, audit.Event{
				Category: audit.URLBrowsed,
				ToolName: toolName,
				Agent:    agentName,
				TaskID:   taskID,
				Message:  input.URL,
			})
		}

	case "WebSearch":
		var input audit.WebSearchInput
		if err := json.Unmarshal(toolInput, &input); err == nil && input.Query != "" {
			auditEvents = append(auditEvents, audit.Event{
				Category: audit.URLBrowsed,
				ToolName: toolName,
				Agent:    agentName,
				TaskID:   taskID,
				Message:  input.Query,
			})
		}
	}

	return auditEvents
}
