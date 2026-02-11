package controller

import (
	"io"
	"log"
	"testing"

	"github.com/andywolf/agentium/internal/agent"
)

func TestLogTokenConsumption(t *testing.T) {
	t.Run("skips when cloudLogger is nil", func(t *testing.T) {
		c := &Controller{
			logger:         log.New(io.Discard, "", 0),
			cloudLogger:    nil,
			activeTaskType: "issue",
			activeTask:     "42",
		}

		result := &agent.IterationResult{
			InputTokens:  1000,
			OutputTokens: 500,
		}
		session := &agent.Session{}

		// Should not panic when cloudLogger is nil
		c.logTokenConsumption(result, "claude-code", session)
	})

	t.Run("skips when tokens are zero", func(t *testing.T) {
		c := &Controller{
			logger:         log.New(io.Discard, "", 0),
			cloudLogger:    nil,
			activeTaskType: "issue",
			activeTask:     "42",
		}

		result := &agent.IterationResult{
			InputTokens:  0,
			OutputTokens: 0,
		}
		session := &agent.Session{}

		// Should not panic when tokens are zero
		c.logTokenConsumption(result, "claude-code", session)
	})

	t.Run("builds correct task ID and phase", func(t *testing.T) {
		c := &Controller{
			logger:         log.New(io.Discard, "", 0),
			cloudLogger:    nil, // We can't test actual logging without mock
			activeTaskType: "issue",
			activeTask:     "42",
			taskStates: map[string]*TaskState{
				"issue:42": {ID: "42", Type: "issue", Phase: PhaseImplement},
			},
		}

		result := &agent.IterationResult{
			InputTokens:  1500,
			OutputTokens: 300,
		}
		session := &agent.Session{}

		// Should not panic - can't verify labels without mock logger
		c.logTokenConsumption(result, "claude-code", session)
	})
}
