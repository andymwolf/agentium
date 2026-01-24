package memory

import (
	"testing"
)

func TestParseSignals_ValidTypes(t *testing.T) {
	output := `some output before
AGENTIUM_MEMORY: KEY_FACT The API uses JWT tokens
more output
AGENTIUM_MEMORY: DECISION Using PostgreSQL for persistence
AGENTIUM_MEMORY: STEP_DONE Implemented auth middleware
AGENTIUM_MEMORY: STEP_PENDING Write integration tests
AGENTIUM_MEMORY: FILE_MODIFIED internal/auth/handler.go
AGENTIUM_MEMORY: ERROR Failed to connect to test database
trailing output`

	signals := ParseSignals(output)
	if len(signals) != 6 {
		t.Fatalf("expected 6 signals, got %d", len(signals))
	}

	expected := []struct {
		typ     SignalType
		content string
	}{
		{KeyFact, "The API uses JWT tokens"},
		{Decision, "Using PostgreSQL for persistence"},
		{StepDone, "Implemented auth middleware"},
		{StepPending, "Write integration tests"},
		{FileModified, "internal/auth/handler.go"},
		{Error, "Failed to connect to test database"},
	}

	for i, exp := range expected {
		if signals[i].Type != exp.typ {
			t.Errorf("signal[%d]: expected type %q, got %q", i, exp.typ, signals[i].Type)
		}
		if signals[i].Content != exp.content {
			t.Errorf("signal[%d]: expected content %q, got %q", i, exp.content, signals[i].Content)
		}
	}
}

func TestParseSignals_NoMatches(t *testing.T) {
	output := "regular output with no memory signals\njust normal text"
	signals := ParseSignals(output)
	if len(signals) != 0 {
		t.Fatalf("expected 0 signals, got %d", len(signals))
	}
}

func TestParseSignals_InvalidType(t *testing.T) {
	output := "AGENTIUM_MEMORY: UNKNOWN_TYPE some content"
	signals := ParseSignals(output)
	if len(signals) != 0 {
		t.Fatalf("expected 0 signals for invalid type, got %d", len(signals))
	}
}

func TestParseSignals_MultipleOnSameLine(t *testing.T) {
	// Only start-of-line matches should be captured
	output := "prefix AGENTIUM_MEMORY: KEY_FACT should not match\nAGENTIUM_MEMORY: KEY_FACT should match"
	signals := ParseSignals(output)
	if len(signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(signals))
	}
	if signals[0].Content != "should match" {
		t.Errorf("expected content %q, got %q", "should match", signals[0].Content)
	}
}

func TestParseSignals_EmptyOutput(t *testing.T) {
	signals := ParseSignals("")
	if len(signals) != 0 {
		t.Fatalf("expected 0 signals for empty output, got %d", len(signals))
	}
}

func TestParseSignals_NewSignalTypes(t *testing.T) {
	output := `AGENTIUM_MEMORY: EVAL_FEEDBACK fix the nil pointer in TestLogin
AGENTIUM_MEMORY: PHASE_RESULT IMPLEMENT completed (iteration 2)`

	signals := ParseSignals(output)
	if len(signals) != 2 {
		t.Fatalf("expected 2 signals, got %d", len(signals))
	}

	if signals[0].Type != EvalFeedback {
		t.Errorf("signal[0].Type = %q, want %q", signals[0].Type, EvalFeedback)
	}
	if signals[0].Content != "fix the nil pointer in TestLogin" {
		t.Errorf("signal[0].Content = %q, want %q", signals[0].Content, "fix the nil pointer in TestLogin")
	}

	if signals[1].Type != PhaseResult {
		t.Errorf("signal[1].Type = %q, want %q", signals[1].Type, PhaseResult)
	}
	if signals[1].Content != "IMPLEMENT completed (iteration 2)" {
		t.Errorf("signal[1].Content = %q, want %q", signals[1].Content, "IMPLEMENT completed (iteration 2)")
	}
}
