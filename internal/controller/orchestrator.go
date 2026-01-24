package controller

import "context"

// SubTaskOrchestrator maps task phases to specialized sub-agent configurations
// and runs delegated iterations when a config exists for the current phase.
type SubTaskOrchestrator struct {
	config     DelegationConfig
	controller *Controller
}

// NewSubTaskOrchestrator creates an orchestrator with the given delegation config.
func NewSubTaskOrchestrator(config DelegationConfig, controller *Controller) *SubTaskOrchestrator {
	return &SubTaskOrchestrator{
		config:     config,
		controller: controller,
	}
}

// ConfigForPhase returns the SubTaskConfig for the given phase, or nil if
// no delegation is configured for that phase.
func (o *SubTaskOrchestrator) ConfigForPhase(phase TaskPhase) *SubTaskConfig {
	subType, ok := phaseToSubTask[phase]
	if !ok {
		return nil
	}
	cfg, ok := o.config.SubAgents[subType]
	if !ok {
		return nil
	}
	return &cfg
}

// RunSubTask executes a delegated sub-task using runDelegatedIteration.
func (o *SubTaskOrchestrator) RunSubTask(ctx context.Context, st *SubTask) error {
	st.Status = "running"
	result, err := o.controller.runDelegatedIteration(ctx, TaskPhase(st.Type), &st.Config)
	if err != nil {
		st.Status = "failed"
		st.Result = &SubTaskResult{
			Success: false,
			Summary: err.Error(),
		}
		return err
	}
	st.Status = "completed"
	st.Result = &SubTaskResult{
		Success:     result.Success,
		Summary:     result.Summary,
		AgentStatus: result.AgentStatus,
	}
	return nil
}
