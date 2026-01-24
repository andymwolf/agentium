package controller

import "github.com/andywolf/agentium/internal/routing"

// SubTaskType identifies the kind of delegated sub-task.
type SubTaskType string

const (
	SubTaskPlan      SubTaskType = "plan"
	SubTaskImplement SubTaskType = "implement"
	SubTaskTest      SubTaskType = "test"
	SubTaskReview    SubTaskType = "review"
)

// phaseToSubTask maps controller task phases to sub-task types.
var phaseToSubTask = map[TaskPhase]SubTaskType{
	PhaseImplement:  SubTaskImplement,
	PhaseTest:       SubTaskTest,
	PhasePRCreation: SubTaskReview,
	PhaseAnalyze:    SubTaskPlan,
	PhasePush:       SubTaskReview,
}

// SubTaskConfig specifies the agent, model, and skills for a delegated sub-task.
type SubTaskConfig struct {
	Agent  string              `json:"agent,omitempty"`
	Model  *routing.ModelConfig `json:"model,omitempty"`
	Skills []string            `json:"skills,omitempty"`
}

// DelegationConfig controls sub-agent delegation behavior.
type DelegationConfig struct {
	Enabled   bool                        `json:"enabled"`
	Strategy  string                      `json:"strategy"`
	SubAgents map[SubTaskType]SubTaskConfig `json:"sub_agents,omitempty"`
}

// SubTask represents a delegated unit of work.
type SubTask struct {
	ID          string
	ParentTask  string
	Type        SubTaskType
	Description string
	Config      SubTaskConfig
	Status      string
	Result      *SubTaskResult
}

// SubTaskResult holds the outcome of a delegated sub-task execution.
type SubTaskResult struct {
	Success       bool
	Summary       string
	FilesModified []string
	NextSteps     []string
	AgentStatus   string
}
