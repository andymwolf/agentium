// Package audit provides security audit logging for agent tool actions.
// It classifies tool invocations into security-relevant categories and
// emits structured log entries to Cloud Logging for forensic visibility.
package audit

// Category represents a security-relevant action category.
type Category string

const (
	// BashCommand is any bash/command execution that is NOT a `gh` command.
	BashCommand Category = "BASH_COMMAND"
	// URLBrowsed is any URL fetched (WebFetch) or web search (WebSearch).
	URLBrowsed Category = "URL_BROWSED"
	// SensitiveFileWrite is a file write/edit to a sensitive path.
	SensitiveFileWrite Category = "SENSITIVE_FILE_WRITE"
	// PackageInstall is a package installation command.
	PackageInstall Category = "PACKAGE_INSTALL"
	// OutboundDataTransfer is a command that could exfiltrate data.
	OutboundDataTransfer Category = "OUTBOUND_DATA_TRANSFER"
)

// Event represents a single security audit event.
type Event struct {
	// Category is the security classification of this event.
	Category Category
	// ToolName is the name of the tool that triggered this event (e.g., "Bash", "Write").
	ToolName string
	// Agent is the name of the agent adapter (e.g., "claudecode", "codex").
	Agent string
	// TaskID is the active task identifier (e.g., "issue:42").
	TaskID string
	// Message is the full tool input (command text, URL, file path).
	Message string
}
