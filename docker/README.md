# Agentium Docker Images

This directory contains Dockerfiles for the various agent runtime environments supported by Agentium.

## Agent Images

- **claudecode/**: Claude Code agent runtime
- **aider/**: Aider agent runtime
- **codex/**: OpenAI Codex CLI runtime
- **controller/**: Session controller (runs on VM, not an agent)

## Language Runtime Auto-Detection

Agent containers automatically detect the project language and install the appropriate runtime before starting the agent. This ensures agents can build and test code in any supported language.

### Supported Languages

| Language | Detection Files | Runtime Installed |
|----------|----------------|-------------------|
| Go | `go.mod` | Go 1.22.0 |
| Rust | `Cargo.toml` | Latest stable via rustup |
| Java | `pom.xml`, `build.gradle`, `build.gradle.kts` | OpenJDK 17 |
| Ruby | `Gemfile` | Ruby (system package) |
| .NET/C# | `*.csproj`, `*.sln` | .NET SDK 8.0 |
| Node.js | `package.json` | Pre-installed (base image) |
| Python | `requirements.txt`, `pyproject.toml`, `setup.py` | Pre-installed (base image) |

### How It Works

1. When an agent container starts, the `agent-wrapper.sh` script runs first
2. It executes `install-runtime.sh` which detects the project type in `/workspace`
3. The appropriate language runtime is installed if needed
4. The agent command is then executed with the runtime available

### Adding New Languages

To add support for a new language:

1. Add detection logic to `docker/scripts/install-runtime.sh`
2. Add an installation function for the runtime
3. Ensure the runtime installer works with the `agentium` user (sudo is available)
4. Test the detection and installation

### Manual Testing

To test language detection logic:

```bash
bash docker/scripts/test-detection.sh
```

To build and test a specific agent image:

```bash
docker build -f docker/claudecode/Dockerfile -t agentium-claudecode .
docker run --rm -v $(pwd):/workspace agentium-claudecode --help
```