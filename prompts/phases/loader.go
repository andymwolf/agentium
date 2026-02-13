package phases

import (
	_ "embed"
	"strings"
)

//go:embed plan_worker.md
var planWorker string

//go:embed plan_reviewer.md
var planReviewer string

//go:embed plan_judge.md
var planJudge string

//go:embed implement_worker.md
var implementWorker string

//go:embed implement_reviewer.md
var implementReviewer string

//go:embed implement_judge.md
var implementJudge string

//go:embed docs_worker.md
var docsWorker string

//go:embed docs_reviewer.md
var docsReviewer string

//go:embed docs_judge.md
var docsJudge string

//go:embed verify_worker.md
var verifyWorker string

//go:embed verify_reviewer.md
var verifyReviewer string

//go:embed verify_judge.md
var verifyJudge string

// promptMap maps "PHASE:ROLE" keys to their embedded prompt content.
var promptMap = map[string]string{
	"PLAN:WORKER":        planWorker,
	"PLAN:REVIEWER":      planReviewer,
	"PLAN:JUDGE":         planJudge,
	"IMPLEMENT:WORKER":   implementWorker,
	"IMPLEMENT:REVIEWER": implementReviewer,
	"IMPLEMENT:JUDGE":    implementJudge,
	"DOCS:WORKER":        docsWorker,
	"DOCS:REVIEWER":      docsReviewer,
	"DOCS:JUDGE":         docsJudge,
	"VERIFY:WORKER":      verifyWorker,
	"VERIFY:REVIEWER":    verifyReviewer,
	"VERIFY:JUDGE":       verifyJudge,
}

// Get returns the static prompt for the given phase and role.
// Phase should be one of: PLAN, IMPLEMENT, DOCS, VERIFY.
// Role should be one of: WORKER, REVIEWER, JUDGE.
// Returns empty string for unknown combinations.
func Get(phase, role string) string {
	key := strings.ToUpper(phase) + ":" + strings.ToUpper(role)
	return promptMap[key]
}

// Phases returns the known phase names in execution order.
func Phases() []string {
	return []string{"PLAN", "IMPLEMENT", "DOCS", "VERIFY"}
}
