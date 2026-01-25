# Runtime Detection and Installation

Agentium automatically detects the programming language of your project and installs the appropriate runtime environment in the agent container. This ensures agents can build and test code in any supported language.

## How It Works

When an agent container starts:

1. The `agent-wrapper.sh` script runs the `install-runtime.sh` script
2. The script checks for language-specific marker files in `/workspace`
3. If a supported language is detected, its runtime is installed
4. The agent then starts with the necessary tools available

## Supported Languages

| Language | Detection File | Runtime Installed |
|----------|---------------|-------------------|
| Go | `go.mod` | Go 1.22.0 |
| Rust | `Cargo.toml` | Rust (latest stable via rustup) |
| Java | `pom.xml`, `build.gradle`, `build.gradle.kts` | OpenJDK 17 |
| Ruby | `Gemfile` | Ruby (system package) |
| .NET | `*.csproj`, `*.sln` | .NET SDK 8.0 |
| Node.js | `package.json` | Pre-installed (Node 20) |
| Python | `requirements.txt`, `pyproject.toml`, `setup.py` | Pre-installed (Python 3) |

## Implementation Details

### Scripts

- **`/docker/scripts/install-runtime.sh`**: Main detection and installation logic
- **`/docker/scripts/agent-wrapper.sh`**: Wrapper that runs before the agent starts

### Docker Integration

All agent Dockerfiles (`claudecode`, `aider`, `codex`) include:

```dockerfile
# Copy runtime installation scripts
COPY docker/scripts/install-runtime.sh /runtime-scripts/install-runtime.sh
COPY docker/scripts/agent-wrapper.sh /runtime-scripts/agent-wrapper.sh
RUN chmod +x /runtime-scripts/*.sh

# Use wrapper script as entrypoint
ENTRYPOINT ["/runtime-scripts/agent-wrapper.sh", "<agent-name>"]
```

### Performance Considerations

- Installation happens once per container start
- Downloads are cached by the container runtime
- Installation typically takes 10-30 seconds for uncached runtimes
- No installation occurs for Node.js and Python projects (pre-installed)

## Testing

The runtime detection logic is tested in `internal/controller/runtime_test.go`.

To test the installation scripts manually:

```bash
# Build a test image
docker build -t test-agent -f docker/claudecode/Dockerfile .

# Run with a Go project
docker run -v /path/to/go/project:/workspace test-agent

# Verify Go is available
docker exec <container-id> go version
```

## Adding New Languages

To add support for a new language:

1. Add detection logic to `install-runtime.sh`:
   ```bash
   elif file_exists "<marker-file>"; then
       install_<language>
   ```

2. Add installation function:
   ```bash
   install_<language>() {
       log "Detected <Language> project"
       log "Installing <Language> runtime..."
       # Installation commands
       log "<Language> installed successfully"
   }
   ```

3. Update this documentation
4. Add test cases to `runtime_test.go`

## Troubleshooting

If a runtime fails to install:

1. Check container logs: `docker logs <container-id>`
2. Verify the marker file exists in `/workspace`
3. Check internet connectivity (for downloading runtimes)
4. Ensure sufficient disk space for installation

## Future Improvements

- [ ] Cache downloaded runtimes in a volume
- [ ] Support multiple language detection (polyglot projects)
- [ ] Add version detection and selection
- [ ] Support for language-specific package managers (npm, pip, cargo, etc.)