package audit

import (
	"regexp"
	"strings"
)

// sensitivePathPatterns matches file paths that are considered sensitive for writes.
var sensitivePathPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\.env($|\.)`),                        // .env, .env.local, etc.
	regexp.MustCompile(`(?i)\.(pem|key|crt|cer|p12|pfx)$`),       // Certificates and keys
	regexp.MustCompile(`(?i)credentials?`),                       // credential*, credentials*, *credential*
	regexp.MustCompile(`(?i)secrets?\.`),                         // secret.yaml, secrets.json, etc.
	regexp.MustCompile(`(?i)\.github/workflows/`),                // GitHub Actions workflows
	regexp.MustCompile(`(?i)\.gitlab-ci\.ya?ml$`),                // GitLab CI config
	regexp.MustCompile(`(?i)Dockerfile`),                         // Dockerfile, Dockerfile.prod, etc.
	regexp.MustCompile(`(?i)docker-compose`),                     // docker-compose.yml, etc.
	regexp.MustCompile(`(?i)(^|/)Makefile$`),                     // Makefile
	regexp.MustCompile(`(?i)(^|/)Rakefile$`),                     // Rakefile
	regexp.MustCompile(`(?i)(^|/)Justfile$`),                     // Justfile
	regexp.MustCompile(`(?i)(^|/)(bin|scripts|bootstrap)/[^/]+`), // bin/*, scripts/*, bootstrap/*
	regexp.MustCompile(`(?i)id_rsa`),                             // SSH private keys
	regexp.MustCompile(`(?i)authorized_keys$`),                   // SSH authorized keys
	regexp.MustCompile(`(?i)(^|/)\.ssh/`),                        // .ssh directory
	regexp.MustCompile(`(?i)(^|/)\.gnupg/`),                      // GPG directory
	regexp.MustCompile(`(?i)(^|/)\.aws/`),                        // AWS config directory
	regexp.MustCompile(`(?i)(^|/)\.kube/`),                       // Kubernetes config
}

// packageInstallPatterns matches commands that install packages.
var packageInstallPatterns = []*regexp.Regexp{
	// npm/yarn/pnpm
	regexp.MustCompile(`(?i)\bnpm\s+(install|i|add|ci)\b`),
	regexp.MustCompile(`(?i)\byarn\s+(add|install)\b`),
	regexp.MustCompile(`(?i)\bpnpm\s+(add|install|i)\b`),
	// pip/pipx
	regexp.MustCompile(`(?i)\bpip3?\s+install\b`),
	regexp.MustCompile(`(?i)\bpipx?\s+install\b`),
	// go
	regexp.MustCompile(`(?i)\bgo\s+(get|install)\b`),
	// apt/apk
	regexp.MustCompile(`(?i)\bapt(-get)?\s+install\b`),
	regexp.MustCompile(`(?i)\bapk\s+add\b`),
	// cargo/gem/composer
	regexp.MustCompile(`(?i)\bcargo\s+install\b`),
	regexp.MustCompile(`(?i)\bgem\s+install\b`),
	regexp.MustCompile(`(?i)\bcomposer\s+(require|install)\b`),
	// brew
	regexp.MustCompile(`(?i)\bbrew\s+install\b`),
}

// outboundTransferPatterns matches commands that could exfiltrate data.
var outboundTransferPatterns = []*regexp.Regexp{
	// curl with POST/PUT/PATCH or data flags
	regexp.MustCompile(`(?i)\bcurl\b[^|]*(-X\s*(POST|PUT|PATCH)|--data|-d\s|--upload-file|-T\s|-F\s|--form)`),
	// wget with POST data
	regexp.MustCompile(`(?i)\bwget\b[^|]*(--post-data|--post-file)`),
	// scp (any direction)
	regexp.MustCompile(`(?i)\bscp\b`),
	// rsync to remote (contains user@host: pattern)
	regexp.MustCompile(`(?i)\brsync\b[^|]*\w+@[\w.-]+:`),
	// piping to netcat
	regexp.MustCompile(`\|\s*(nc|netcat)\b`),
	// sftp
	regexp.MustCompile(`(?i)\bsftp\b`),
	// ftp put commands
	regexp.MustCompile(`(?i)\bftp\b[^|]*\bput\b`),
}

// IsSensitivePath returns true if the given file path matches a sensitive pattern.
func IsSensitivePath(path string) bool {
	for _, pattern := range sensitivePathPatterns {
		if pattern.MatchString(path) {
			return true
		}
	}
	return false
}

// IsPackageInstall returns true if the command appears to install packages.
func IsPackageInstall(command string) bool {
	for _, pattern := range packageInstallPatterns {
		if pattern.MatchString(command) {
			return true
		}
	}
	return false
}

// IsOutboundTransfer returns true if the command could exfiltrate data.
func IsOutboundTransfer(command string) bool {
	for _, pattern := range outboundTransferPatterns {
		if pattern.MatchString(command) {
			return true
		}
	}
	return false
}

// IsGHCommand returns true if the command is a `gh` CLI command.
// These are excluded from BASH_COMMAND audit events since gh operations
// are expected and already logged elsewhere.
func IsGHCommand(command string) bool {
	trimmed := strings.TrimSpace(command)
	return strings.HasPrefix(trimmed, "gh ") || trimmed == "gh"
}

// ClassifyBashCommand returns all categories that apply to the given bash command.
// A single command can match multiple categories (e.g., npm install is both
// BASH_COMMAND and PACKAGE_INSTALL).
func ClassifyBashCommand(command string) []Category {
	var categories []Category

	// Skip gh commands for BASH_COMMAND category
	if !IsGHCommand(command) {
		categories = append(categories, BashCommand)
	}

	if IsPackageInstall(command) {
		categories = append(categories, PackageInstall)
	}

	if IsOutboundTransfer(command) {
		categories = append(categories, OutboundDataTransfer)
	}

	return categories
}
