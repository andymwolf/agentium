package memory

import (
	"strings"
	"testing"
	"time"
)

func TestBuildContext_Empty(t *testing.T) {
	s := NewStore(t.TempDir(), Config{})
	ctx := s.BuildContext()
	if ctx != "" {
		t.Errorf("expected empty context for empty store, got %q", ctx)
	}
}

func TestBuildContext_GroupsByType(t *testing.T) {
	s := NewStore(t.TempDir(), Config{ContextBudget: 5000})
	s.data.Entries = []Entry{
		{Type: KeyFact, Content: "fact one", Iteration: 1, Timestamp: time.Now()},
		{Type: KeyFact, Content: "fact two", Iteration: 1, Timestamp: time.Now()},
		{Type: StepDone, Content: "implemented auth", Iteration: 1, Timestamp: time.Now()},
		{Type: StepPending, Content: "write tests", Iteration: 1, Timestamp: time.Now()},
		{Type: Decision, Content: "use JWT", Iteration: 1, Timestamp: time.Now()},
	}

	ctx := s.BuildContext()

	// Check header
	if !strings.Contains(ctx, "## Memory from Previous Iterations") {
		t.Error("missing header")
	}

	// Check sections exist
	if !strings.Contains(ctx, "### Pending Steps") {
		t.Error("missing Pending Steps section")
	}
	if !strings.Contains(ctx, "### Key Facts") {
		t.Error("missing Key Facts section")
	}
	if !strings.Contains(ctx, "### Decisions") {
		t.Error("missing Decisions section")
	}
	if !strings.Contains(ctx, "### Completed Steps") {
		t.Error("missing Completed Steps section")
	}

	// Check priority order: StepPending before KeyFact before Decision before StepDone
	pendingIdx := strings.Index(ctx, "### Pending Steps")
	factsIdx := strings.Index(ctx, "### Key Facts")
	decisionsIdx := strings.Index(ctx, "### Decisions")
	doneIdx := strings.Index(ctx, "### Completed Steps")

	if pendingIdx > factsIdx {
		t.Error("Pending Steps should come before Key Facts")
	}
	if factsIdx > decisionsIdx {
		t.Error("Key Facts should come before Decisions")
	}
	if decisionsIdx > doneIdx {
		t.Error("Decisions should come before Completed Steps")
	}
}

func TestBuildContext_RespectsBudget(t *testing.T) {
	s := NewStore(t.TempDir(), Config{ContextBudget: 150})
	s.data.Entries = []Entry{
		{Type: StepPending, Content: "short task", Iteration: 1, Timestamp: time.Now()},
		{Type: KeyFact, Content: strings.Repeat("x", 200), Iteration: 1, Timestamp: time.Now()},
		{Type: Decision, Content: "should not appear", Iteration: 1, Timestamp: time.Now()},
	}

	ctx := s.BuildContext()

	// The first section (Pending Steps) should fit, but the second (Key Facts with 200 chars) should not
	if !strings.Contains(ctx, "### Pending Steps") {
		t.Error("Pending Steps should fit within budget")
	}
	if strings.Contains(ctx, "### Decisions") {
		t.Error("Decisions should be cut by budget")
	}
	if len(ctx) > 200 {
		t.Errorf("context should be within budget range, got %d chars", len(ctx))
	}
}

func TestBuildEvalContext_Empty(t *testing.T) {
	s := NewStore(t.TempDir(), Config{})
	ctx := s.BuildEvalContext()
	if ctx != "" {
		t.Errorf("expected empty eval context for empty store, got %q", ctx)
	}
}

func TestBuildEvalContext_FiltersToEvalRelevant(t *testing.T) {
	s := NewStore(t.TempDir(), Config{ContextBudget: 5000})
	s.data.Entries = []Entry{
		{Type: EvalFeedback, Content: "fix tests", Iteration: 1, Timestamp: time.Now()},
		{Type: PhaseResult, Content: "PLAN completed (iteration 1)", Iteration: 1, Timestamp: time.Now()},
		{Type: StepPending, Content: "write tests", Iteration: 1, Timestamp: time.Now()},
		{Type: KeyFact, Content: "important fact", Iteration: 2, Timestamp: time.Now()},
		{Type: FileModified, Content: "auth.go", Iteration: 2, Timestamp: time.Now()},
		{Type: EvalFeedback, Content: "add error handling", Iteration: 2, Timestamp: time.Now()},
	}

	ctx := s.BuildEvalContext()

	// Should contain eval-relevant entries
	if !strings.Contains(ctx, "## Iteration History") {
		t.Error("missing header")
	}
	if !strings.Contains(ctx, "### Evaluator Feedback") {
		t.Error("missing Evaluator Feedback section")
	}
	if !strings.Contains(ctx, "### Phase Results") {
		t.Error("missing Phase Results section")
	}
	if !strings.Contains(ctx, "fix tests") {
		t.Error("missing eval feedback content")
	}
	if !strings.Contains(ctx, "PLAN completed") {
		t.Error("missing phase result content")
	}

	// Should NOT contain agent-internal signals
	if strings.Contains(ctx, "write tests") {
		t.Error("should not contain StepPending entries")
	}
	if strings.Contains(ctx, "important fact") {
		t.Error("should not contain KeyFact entries")
	}
	if strings.Contains(ctx, "auth.go") {
		t.Error("should not contain FileModified entries")
	}
}

func TestBuildEvalContext_IncludesIterationNumbers(t *testing.T) {
	s := NewStore(t.TempDir(), Config{ContextBudget: 5000})
	s.data.Entries = []Entry{
		{Type: EvalFeedback, Content: "first feedback", Iteration: 3, Timestamp: time.Now()},
		{Type: PhaseResult, Content: "phase done", Iteration: 5, Timestamp: time.Now()},
	}

	ctx := s.BuildEvalContext()

	if !strings.Contains(ctx, "[iter 3]") {
		t.Error("should include iteration number for feedback entry")
	}
	if !strings.Contains(ctx, "[iter 5]") {
		t.Error("should include iteration number for phase result entry")
	}
}

func TestBuildEvalContext_NoEvalEntries(t *testing.T) {
	s := NewStore(t.TempDir(), Config{ContextBudget: 5000})
	s.data.Entries = []Entry{
		{Type: StepPending, Content: "write tests", Iteration: 1, Timestamp: time.Now()},
		{Type: KeyFact, Content: "important", Iteration: 1, Timestamp: time.Now()},
		{Type: FileModified, Content: "main.go", Iteration: 2, Timestamp: time.Now()},
	}

	ctx := s.BuildEvalContext()
	if ctx != "" {
		t.Errorf("expected empty eval context when no eval-relevant entries, got %q", ctx)
	}
}

func TestBuildEvalContext_RespectsBudget(t *testing.T) {
	s := NewStore(t.TempDir(), Config{ContextBudget: 100})
	s.data.Entries = []Entry{
		{Type: EvalFeedback, Content: "short feedback", Iteration: 1, Timestamp: time.Now()},
		{Type: PhaseResult, Content: strings.Repeat("x", 200), Iteration: 2, Timestamp: time.Now()},
	}

	ctx := s.BuildEvalContext()

	if !strings.Contains(ctx, "### Evaluator Feedback") {
		t.Error("first section should fit within budget")
	}
	if strings.Contains(ctx, "### Phase Results") {
		t.Error("second section should be cut by budget")
	}
}

func TestBuildContext_AllEntriesExceedBudget(t *testing.T) {
	s := NewStore(t.TempDir(), Config{ContextBudget: 50})
	s.data.Entries = []Entry{
		{Type: StepPending, Content: strings.Repeat("x", 200), Iteration: 1, Timestamp: time.Now()},
	}

	ctx := s.BuildContext()
	if ctx != "" {
		t.Errorf("expected empty context when no section fits budget, got %q", ctx)
	}
}
