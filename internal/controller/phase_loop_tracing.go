package controller

import (
	"time"

	"github.com/andywolf/agentium/internal/handoff"
	"github.com/andywolf/agentium/internal/observability"
)

// initPhaseLoopTrace starts the Langfuse trace for the phase loop.
func (c *Controller) initPhaseLoopTrace(plc *phaseLoopContext) {
	plc.traceCtx = c.tracer.StartTrace(plc.taskID, observability.TraceOptions{
		Workflow:   "phase_loop",
		Repository: c.config.Repository,
		SessionID:  c.config.ID,
	})
	plc.traceStatus = "error" // default status if function exits unexpectedly
}

// completePhaseLoopTrace ends the phase loop trace, cleaning up any active span.
func (c *Controller) completePhaseLoopTrace(plc *phaseLoopContext) {
	c.endPhaseSpan(plc, "interrupted")
	c.tracer.CompleteTrace(plc.traceCtx, observability.CompleteOptions{
		Status:            plc.traceStatus,
		TotalInputTokens:  plc.totalInputTokens,
		TotalOutputTokens: plc.totalOutputTokens,
	})
}

// startPhaseSpan starts a Langfuse span for the current phase.
func (c *Controller) startPhaseSpan(plc *phaseLoopContext) {
	plc.activePhaseStart = time.Now()
	plc.activeSpanCtx = c.tracer.StartPhase(plc.traceCtx, string(plc.currentPhase), observability.SpanOptions{
		MaxIterations: plc.maxIter,
	})
	plc.hasActiveSpan = true
}

// endPhaseSpan ends the active Langfuse span if one exists.
func (c *Controller) endPhaseSpan(plc *phaseLoopContext, status string) {
	if plc.hasActiveSpan {
		opts := observability.EndPhaseOptions{
			Status:     status,
			DurationMs: time.Since(plc.activePhaseStart).Milliseconds(),
		}
		if c.handoffStore != nil {
			opts.Input = c.resolvePhaseInput(plc.taskID, plc.currentPhase)
			opts.Output = c.resolvePhaseOutput(plc.taskID, plc.currentPhase)
		}
		c.tracer.EndPhase(plc.activeSpanCtx, opts)
		plc.hasActiveSpan = false
	}
}

// resolvePhaseInput returns the structured input for a phase based on the
// handoff data flowing into it from previous phases.
func (c *Controller) resolvePhaseInput(taskID string, phase TaskPhase) interface{} {
	switch phase {
	case PhasePlan:
		return c.handoffStore.GetIssueContext(taskID)
	case PhaseImplement:
		return c.handoffStore.GetPlanOutput(taskID)
	case PhaseDocs:
		plan := c.handoffStore.GetPlanOutput(taskID)
		impl := c.handoffStore.GetImplementOutput(taskID)
		if plan == nil && impl == nil {
			return nil
		}
		m := map[string]interface{}{}
		if plan != nil {
			m["plan"] = plan
		}
		if impl != nil {
			m["implement"] = impl
		}
		return m
	case PhaseVerify:
		return c.handoffStore.GetImplementOutput(taskID)
	default:
		return nil
	}
}

// resolvePhaseOutput returns the structured output produced by the current phase,
// or nil if the phase was interrupted or hasn't produced output yet.
func (c *Controller) resolvePhaseOutput(taskID string, phase TaskPhase) interface{} {
	hd := c.handoffStore.GetPhaseOutput(taskID, handoff.Phase(phase))
	if hd == nil {
		return nil
	}
	return hd.GetOutput()
}

// recordGenerationTokens records a generation event and accumulates token counts.
func (c *Controller) recordGenerationTokens(plc *phaseLoopContext, gen observability.GenerationInput) {
	c.tracer.RecordGeneration(plc.activeSpanCtx, gen)
	plc.totalInputTokens += gen.InputTokens
	plc.totalOutputTokens += gen.OutputTokens
}
