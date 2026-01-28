package agentmd

const agentMDTemplate = `<!-- agentium:generated:start -->
# {{.Name}} Project Instructions

## Project Overview
{{if .Framework}}
**Framework:** {{.Framework}}
{{end}}
{{- if .Languages}}
**Languages:** {{range $i, $lang := .Languages}}{{if $i}}, {{end}}{{$lang.Name}} ({{printf "%.0f" $lang.Percentage}}%){{end}}
{{end}}
{{- if .BuildSystem}}
**Build System:** {{.BuildSystem}}
{{end}}

## Project Structure

{{- if .Structure.SourceDirs}}

### Source Directories
{{range .Structure.SourceDirs}}- ` + "`{{.}}/`" + `
{{end}}
{{- end}}

{{- if .Structure.TestDirs}}

### Test Directories
{{range .Structure.TestDirs}}- ` + "`{{.}}/`" + `
{{end}}
{{- end}}

{{- if .Structure.EntryPoints}}

### Entry Points
{{range .Structure.EntryPoints}}- ` + "`{{.}}`" + `
{{end}}
{{- end}}

{{- if .Structure.ConfigFiles}}

### Configuration Files
{{range .Structure.ConfigFiles}}- ` + "`{{.}}`" + `
{{end}}
{{- end}}

## Build & Test Commands

{{- if .BuildCommands}}

### Build
` + "```bash" + `
{{range .BuildCommands}}{{.}}
{{end}}` + "```" + `
{{- end}}

{{- if .TestCommands}}

### Test
` + "```bash" + `
{{range .TestCommands}}{{.}}
{{end}}` + "```" + `
{{- end}}

{{- if .LintCommands}}

### Lint
` + "```bash" + `
{{range .LintCommands}}{{.}}
{{end}}` + "```" + `
{{- end}}

{{- if .Structure.HasDocker}}

## Docker

This project uses Docker.
{{- if .Structure.ConfigFiles}}{{range .Structure.ConfigFiles}}{{if or (eq . "Dockerfile") (eq . "docker-compose.yml") (eq . "docker-compose.yaml")}}
- ` + "`{{.}}`" + `{{end}}{{end}}{{end}}
{{- end}}

{{- if .Structure.HasCI}}

## CI/CD

**System:** {{.Structure.CISystem}}
{{- end}}

{{- if .Dependencies}}

## Key Dependencies
{{range $i, $dep := .Dependencies}}{{if lt $i 10}}- {{$dep}}
{{end}}{{end}}
{{- end}}

<!-- agentium:generated:end -->
`
