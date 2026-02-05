package memory

import "regexp"

// signalPattern matches lines of the form: AGENTIUM_MEMORY: TYPE content
var signalPattern = regexp.MustCompile(`(?m)^AGENTIUM_MEMORY:\s+(\w+)\s+(.+)$`)

// validTypes is the set of recognised signal types.
var validTypes = map[SignalType]bool{
	KeyFact:      true,
	Decision:     true,
	StepDone:     true,
	StepPending:  true,
	FileModified: true,
	Error:        true,
	EvalFeedback:    true,
	PhaseResult:     true,
	FeedbackResponse: true,
}

// ParseSignals extracts all memory signals from combined agent output.
func ParseSignals(output string) []Signal {
	matches := signalPattern.FindAllStringSubmatch(output, -1)
	signals := make([]Signal, 0, len(matches))
	for _, m := range matches {
		st := SignalType(m[1])
		if !validTypes[st] {
			continue
		}
		signals = append(signals, Signal{
			Type:    st,
			Content: m[2],
		})
	}
	return signals
}
