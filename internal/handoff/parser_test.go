package handoff

import (
	"testing"
)

func TestParseHandoffSignal(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "simple plan output",
			input: `Some text before AGENTIUM_HANDOFF: {"summary": "test", "files_to_modify": []} more text`,
			want:  `{"summary": "test", "files_to_modify": []}`,
		},
		{
			name: "multiline JSON",
			input: `AGENTIUM_HANDOFF: {
  "summary": "test",
  "files_to_modify": ["file1.go"]
}
some text after`,
			want: `{
  "summary": "test",
  "files_to_modify": ["file1.go"]
}`,
		},
		{
			name:  "nested objects",
			input: `AGENTIUM_HANDOFF: {"steps": [{"number": 1, "data": {"key": "value"}}]}`,
			want:  `{"steps": [{"number": 1, "data": {"key": "value"}}]}`,
		},
		{
			name:    "no signal",
			input:   "Some text without handoff signal",
			wantErr: true,
		},
		{
			name:    "signal without JSON",
			input:   "AGENTIUM_HANDOFF: not json",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseHandoffSignal(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseHandoffSignal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseHandoffSignal() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParsePlanOutput(t *testing.T) {
	jsonStr := `{
		"summary": "Add user authentication",
		"files_to_modify": ["auth.go", "handler.go"],
		"files_to_create": ["middleware.go"],
		"implementation_steps": [
			{"number": 1, "description": "Add middleware", "file": "middleware.go"},
			{"number": 2, "description": "Update handler"}
		],
		"testing_approach": "Unit tests"
	}`

	output, err := ParsePlanOutput(jsonStr)
	if err != nil {
		t.Fatalf("ParsePlanOutput() error = %v", err)
	}

	if output.Summary != "Add user authentication" {
		t.Errorf("Summary = %q, want %q", output.Summary, "Add user authentication")
	}
	if len(output.FilesToModify) != 2 {
		t.Errorf("FilesToModify len = %d, want 2", len(output.FilesToModify))
	}
	if len(output.FilesToCreate) != 1 {
		t.Errorf("FilesToCreate len = %d, want 1", len(output.FilesToCreate))
	}
	if len(output.ImplementationSteps) != 2 {
		t.Errorf("ImplementationSteps len = %d, want 2", len(output.ImplementationSteps))
	}
	if output.ImplementationSteps[0].File != "middleware.go" {
		t.Errorf("Step 1 file = %q, want %q", output.ImplementationSteps[0].File, "middleware.go")
	}
}

func TestParseImplementOutput(t *testing.T) {
	jsonStr := `{
		"branch_name": "agentium/issue-42-add-auth",
		"commits": [
			{"sha": "abc1234", "message": "Add auth middleware"},
			{"sha": "def5678", "message": "Fix tests"}
		],
		"files_changed": ["auth.go", "auth_test.go"],
		"tests_passed": true,
		"test_output": "ok  ./..."
	}`

	output, err := ParseImplementOutput(jsonStr)
	if err != nil {
		t.Fatalf("ParseImplementOutput() error = %v", err)
	}

	if output.BranchName != "agentium/issue-42-add-auth" {
		t.Errorf("BranchName = %q, want %q", output.BranchName, "agentium/issue-42-add-auth")
	}
	if len(output.Commits) != 2 {
		t.Errorf("Commits len = %d, want 2", len(output.Commits))
	}
	if !output.TestsPassed {
		t.Error("TestsPassed = false, want true")
	}
}

func TestParseReviewOutput(t *testing.T) {
	jsonStr := `{
		"issues_found": [
			{"file": "auth.go", "line": 42, "description": "Missing error check", "severity": "error", "fixed": true}
		],
		"fixes_applied": ["Added error handling in auth.go:42"],
		"regression_needed": false,
		"regression_reason": ""
	}`

	output, err := ParseReviewOutput(jsonStr)
	if err != nil {
		t.Fatalf("ParseReviewOutput() error = %v", err)
	}

	if len(output.IssuesFound) != 1 {
		t.Errorf("IssuesFound len = %d, want 1", len(output.IssuesFound))
	}
	if output.IssuesFound[0].File != "auth.go" {
		t.Errorf("Issue file = %q, want %q", output.IssuesFound[0].File, "auth.go")
	}
	if !output.IssuesFound[0].Fixed {
		t.Error("Issue fixed = false, want true")
	}
	if output.RegressionNeeded {
		t.Error("RegressionNeeded = true, want false")
	}
}

func TestHasHandoffSignal(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"AGENTIUM_HANDOFF: {}", true},
		{"Some AGENTIUM_HANDOFF: {} text", true},
		{"No signal here", false},
		{"AGENTIUM_STATUS: COMPLETE", false},
	}

	for _, tt := range tests {
		got := HasHandoffSignal(tt.input)
		if got != tt.want {
			t.Errorf("HasHandoffSignal(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestExtractAllHandoffSignals(t *testing.T) {
	input := `First signal: AGENTIUM_HANDOFF: {"a": 1}
Some text
Second signal: AGENTIUM_HANDOFF: {"b": 2}
End`

	signals := ExtractAllHandoffSignals(input)
	if len(signals) != 2 {
		t.Fatalf("ExtractAllHandoffSignals() = %d signals, want 2", len(signals))
	}
	if signals[0] != `{"a": 1}` {
		t.Errorf("Signal 0 = %q, want %q", signals[0], `{"a": 1}`)
	}
	if signals[1] != `{"b": 2}` {
		t.Errorf("Signal 1 = %q, want %q", signals[1], `{"b": 2}`)
	}
}

func TestParseAndStorePhaseOutput(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	taskID := "issue:42"

	// Test PLAN phase
	planOutput := `Some agent output
AGENTIUM_HANDOFF: {"summary": "Test plan", "files_to_modify": [], "files_to_create": [], "implementation_steps": [], "testing_approach": "unit tests"}
AGENTIUM_STATUS: COMPLETE`

	err := ParseAndStorePhaseOutput(store, taskID, "PLAN", planOutput)
	if err != nil {
		t.Fatalf("ParseAndStorePhaseOutput(PLAN) error = %v", err)
	}

	plan := store.GetPlanOutput(taskID)
	if plan == nil {
		t.Fatal("expected plan output to be stored")
	}
	if plan.Summary != "Test plan" {
		t.Errorf("Plan summary = %q, want %q", plan.Summary, "Test plan")
	}
}

func TestExtractJSONObject(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "simple object",
			input: `{"key": "value"}`,
			want:  `{"key": "value"}`,
		},
		{
			name:  "nested object",
			input: `{"outer": {"inner": "value"}}`,
			want:  `{"outer": {"inner": "value"}}`,
		},
		{
			name:  "with array",
			input: `{"arr": [1, 2, {"nested": true}]}`,
			want:  `{"arr": [1, 2, {"nested": true}]}`,
		},
		{
			name:  "with escaped quotes",
			input: `{"str": "value with \"quotes\""}`,
			want:  `{"str": "value with \"quotes\""}`,
		},
		{
			name:  "with trailing text",
			input: `{"key": "value"} more text`,
			want:  `{"key": "value"}`,
		},
		{
			name:    "not starting with brace",
			input:   "not json",
			wantErr: true,
		},
		{
			name:    "incomplete object",
			input:   `{"key": "value"`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractJSONObject(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractJSONObject() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("extractJSONObject() = %q, want %q", got, tt.want)
			}
		})
	}
}
