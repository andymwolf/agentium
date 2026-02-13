package controller

import (
	"time"

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
		c.tracer.EndPhase(plc.activeSpanCtx, status, time.Since(plc.activePhaseStart).Milliseconds())
		plc.hasActiveSpan = false
	}
}

// recordGenerationTokens records a generation event and accumulates token counts.
func (c *Controller) recordGenerationTokens(plc *phaseLoopContext, gen observability.GenerationInput) {
	c.tracer.RecordGeneration(plc.activeSpanCtx, gen)
	plc.totalInputTokens += gen.InputTokens
	plc.totalOutputTokens += gen.OutputTokens
}
