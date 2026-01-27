package audit

import (
	"testing"
)

func TestIsSensitivePath(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		// Environment files
		{".env", true},
		{".env.local", true},
		{".env.production", true},
		{"config/.env", true},
		{"myenv", false}, // Should not match without dot

		// Certificates and keys
		{"server.pem", true},
		{"private.key", true},
		{"cert.crt", true},
		{"ssl.cer", true},
		{"keystore.p12", true},
		{"keypair.pfx", true},
		{"public.txt", false},

		// Credentials
		{"credentials.json", true},
		{"aws_credentials", true},
		{"my-credentials-file.yaml", true},
		{"credential_helper.go", true},
		{"credit.txt", false},

		// Secrets
		{"secrets.yaml", true},
		{"secret.json", true},
		{"mysecret.yml", true},
		{"secretive.txt", false}, // No dot after secret

		// CI/CD
		{".github/workflows/ci.yml", true},
		{".github/workflows/deploy.yaml", true},
		{".gitlab-ci.yml", true},
		{".gitlab-ci.yaml", true},
		{"github/workflows/ci.yml", false}, // Missing leading dot

		// Docker
		{"Dockerfile", true},
		{"Dockerfile.prod", true},
		{"dockerfile", true},
		{"docker-compose.yml", true},
		{"docker-compose.yaml", true},
		{"docker-compose.override.yml", true},

		// Build files
		{"Makefile", true},
		{"src/Makefile", true},
		{"Rakefile", true},
		{"Justfile", true},
		{"makefile.txt", false},

		// Script directories
		{"bin/deploy.sh", true},
		{"scripts/setup.sh", true},
		{"bootstrap/init.sh", true},
		{"mybin/deploy.sh", false}, // Not exact match

		// SSH
		{"id_rsa", true},
		{"id_rsa.pub", true},
		{".ssh/config", true},
		{".ssh/authorized_keys", true},
		{"authorized_keys", true},
		{"ssh_config", false},

		// Cloud config
		{".aws/credentials", true},
		{".aws/config", true},
		{".kube/config", true},
		{".gnupg/private-keys-v1.d/abc.key", true},

		// Normal files
		{"main.go", false},
		{"README.md", false},
		{"package.json", false},
		{"src/app.js", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := IsSensitivePath(tt.path)
			if result != tt.expected {
				t.Errorf("IsSensitivePath(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestIsPackageInstall(t *testing.T) {
	tests := []struct {
		command  string
		expected bool
	}{
		// npm
		{"npm install express", true},
		{"npm i lodash", true},
		{"npm add react", true},
		{"npm ci", true},
		{"npm run build", false},
		{"npm test", false},

		// yarn
		{"yarn add express", true},
		{"yarn install", true},
		{"yarn test", false},

		// pnpm
		{"pnpm add express", true},
		{"pnpm install", true},
		{"pnpm i", true},
		{"pnpm test", false},

		// pip
		{"pip install requests", true},
		{"pip3 install flask", true},
		{"pipx install black", true},
		{"pip freeze", false},
		{"pip list", false},

		// go
		{"go get github.com/pkg/errors", true},
		{"go install github.com/golangci/golangci-lint", true},
		{"go build ./...", false},
		{"go test ./...", false},

		// apt
		{"apt-get install curl", true},
		{"apt install vim", true},
		{"apt update", false},
		{"apt-get update", false},

		// apk
		{"apk add git", true},
		{"apk update", false},

		// cargo/gem/composer
		{"cargo install ripgrep", true},
		{"gem install rails", true},
		{"composer require laravel/framework", true},
		{"composer install", true},
		{"cargo build", false},
		{"gem list", false},

		// brew
		{"brew install go", true},
		{"brew update", false},

		// Normal commands
		{"ls -la", false},
		{"git clone repo", false},
		{"echo hello", false},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			result := IsPackageInstall(tt.command)
			if result != tt.expected {
				t.Errorf("IsPackageInstall(%q) = %v, want %v", tt.command, result, tt.expected)
			}
		})
	}
}

func TestIsOutboundTransfer(t *testing.T) {
	tests := []struct {
		command  string
		expected bool
	}{
		// curl with POST/PUT/PATCH
		{"curl -X POST https://api.example.com/data", true},
		{"curl -X PUT https://api.example.com/update", true},
		{"curl -X PATCH https://api.example.com/patch", true},
		{"curl --data '{\"key\": \"value\"}' https://api.example.com", true},
		{"curl -d 'data' https://api.example.com", true},
		{"curl --upload-file file.txt https://api.example.com", true},
		{"curl -T file.txt https://api.example.com", true},
		{"curl -F 'file=@data.txt' https://api.example.com", true},
		{"curl --form 'data=@file' https://api.example.com", true},
		{"curl https://api.example.com", false},      // GET request
		{"curl -X GET https://api.example.com", false}, // Explicit GET

		// wget with POST
		{"wget --post-data='key=value' https://api.example.com", true},
		{"wget --post-file=data.txt https://api.example.com", true},
		{"wget https://example.com/file.txt", false}, // Normal download

		// scp
		{"scp file.txt user@host:/path", true},
		{"scp user@host:/path/file.txt .", true},

		// rsync to remote
		{"rsync -avz /local/path user@host:/remote/path", true},
		{"rsync -avz /local/path /other/local/path", false}, // Local only

		// netcat piping
		{"cat file | nc host 1234", true},
		{"echo data | netcat host 1234", true},
		{"nc -l 1234", false}, // Listening, not sending

		// sftp
		{"sftp user@host", true},

		// Normal commands
		{"ls -la", false},
		{"git push origin main", false}, // Git push is allowed
		{"echo hello", false},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			result := IsOutboundTransfer(tt.command)
			if result != tt.expected {
				t.Errorf("IsOutboundTransfer(%q) = %v, want %v", tt.command, result, tt.expected)
			}
		})
	}
}

func TestIsGHCommand(t *testing.T) {
	tests := []struct {
		command  string
		expected bool
	}{
		{"gh pr create", true},
		{"gh issue list", true},
		{"gh auth login", true},
		{"gh", true},
		{"  gh pr view 123", true}, // Leading whitespace
		{"echo gh", false},
		{"github-cli", false},
		{"git hub", false},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			result := IsGHCommand(tt.command)
			if result != tt.expected {
				t.Errorf("IsGHCommand(%q) = %v, want %v", tt.command, result, tt.expected)
			}
		})
	}
}

func TestClassifyBashCommand(t *testing.T) {
	tests := []struct {
		command    string
		expected   []Category
		minCount   int // Minimum expected categories
	}{
		// gh commands should be excluded from BASH_COMMAND
		{"gh pr create", nil, 0},
		{"gh issue list", nil, 0},

		// Normal bash command
		{"ls -la", []Category{BashCommand}, 1},
		{"git status", []Category{BashCommand}, 1},

		// Package install (also BASH_COMMAND)
		{"npm install express", []Category{BashCommand, PackageInstall}, 2},
		{"pip install requests", []Category{BashCommand, PackageInstall}, 2},

		// Outbound transfer (also BASH_COMMAND)
		{"curl -X POST https://api.example.com", []Category{BashCommand, OutboundDataTransfer}, 2},
		{"scp file.txt user@host:/path", []Category{BashCommand, OutboundDataTransfer}, 2},

		// Multiple categories
		{"npm install && curl -X POST https://api.example.com", []Category{BashCommand, PackageInstall, OutboundDataTransfer}, 3},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			result := ClassifyBashCommand(tt.command)
			if len(result) < tt.minCount {
				t.Errorf("ClassifyBashCommand(%q) returned %d categories, want at least %d", tt.command, len(result), tt.minCount)
			}

			// Check that expected categories are present
			for _, expected := range tt.expected {
				found := false
				for _, got := range result {
					if got == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("ClassifyBashCommand(%q) missing category %s", tt.command, expected)
				}
			}
		})
	}
}
