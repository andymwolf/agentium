package controller

import "github.com/andywolf/agentium/internal/routing"

// SubTaskType identifies the kind of delegated sub-task.
type SubTaskType string

const (
	SubTaskPlan       SubTaskType = "plan"
	SubTaskImplement  SubTaskType = "implement"
	SubTaskTest       SubTaskType = "test"
	SubTaskReview     SubTaskType = "review"
	SubTaskPRCreation SubTaskType = "pr_creation"
	SubTaskDocs       SubTaskType = "docs"
	SubTaskPush       SubTaskType = "push"
	SubTaskEvaluate   SubTaskType = "evaluate"
)

// phaseToSubTask maps controller task phases to sub-task types.
var phaseToSubTask = map[TaskPhase]SubTaskType{
	PhasePlan:       SubTaskPlan,
	PhaseImplement:  SubTaskImplement,
	PhaseReview:     SubTaskReview,
	PhaseDocs:       SubTaskDocs,
	PhasePRCreation: SubTaskPRCreation,
	PhaseAnalyze:    SubTaskPlan,
	PhasePush:       SubTaskPush,
}

// SubTaskConfig specifies the agent, model, and skills for a delegated sub-task.
type SubTaskConfig struct {
	Agent  string               `json:"agent,omitempty"`
	Model  *routing.ModelConfig `json:"model,omitempty"`
	Skills []string             `json:"skills,omitempty"`
}

// DelegationConfig controls sub-agent delegation behavior.
type DelegationConfig struct {
	Enabled   bool                          `json:"enabled"`
	Strategy  string                        `json:"strategy"`
	SubAgents map[SubTaskType]SubTaskConfig `json:"sub_agents,omitempty"`
}
