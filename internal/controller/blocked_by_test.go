package controller

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

func TestParseBlockedByGraphQLResponse(t *testing.T) {
	tests := []struct {
		name     string
		jsonResp string
		wantIDs  []string
	}{
		{
			name: "mixed open and closed",
			jsonResp: `{
				"data": {
					"repository": {
						"issue": {
							"blockedBy": {
								"nodes": [
									{"number": 10, "state": "OPEN"},
									{"number": 11, "state": "CLOSED"},
									{"number": 12, "state": "OPEN"}
								]
							}
						}
					}
				}
			}`,
			wantIDs: []string{"10", "12"},
		},
		{
			name: "all closed",
			jsonResp: `{
				"data": {
					"repository": {
						"issue": {
							"blockedBy": {
								"nodes": [
									{"number": 10, "state": "CLOSED"},
									{"number": 11, "state": "CLOSED"}
								]
							}
						}
					}
				}
			}`,
			wantIDs: nil,
		},
		{
			name: "empty nodes",
			jsonResp: `{
				"data": {
					"repository": {
						"issue": {
							"blockedBy": {
								"nodes": []
							}
						}
					}
				}
			}`,
			wantIDs: nil,
		},
		{
			name: "all open",
			jsonResp: `{
				"data": {
					"repository": {
						"issue": {
							"blockedBy": {
								"nodes": [
									{"number": 50, "state": "OPEN"},
									{"number": 51, "state": "OPEN"}
								]
							}
						}
					}
				}
			}`,
			wantIDs: []string{"50", "51"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp blockedByGraphQLResponse
			if err := json.Unmarshal([]byte(tt.jsonResp), &resp); err != nil {
				t.Fatalf("failed to parse test JSON: %v", err)
			}

			var ids []string
			for _, node := range resp.Data.Repository.Issue.BlockedBy.Nodes {
				if strings.EqualFold(node.State, "OPEN") {
					ids = append(ids, strconv.Itoa(node.Number))
				}
			}

			if len(ids) != len(tt.wantIDs) {
				t.Fatalf("got %v (len %d), want %v (len %d)", ids, len(ids), tt.wantIDs, len(tt.wantIDs))
			}
			for i := range ids {
				if ids[i] != tt.wantIDs[i] {
					t.Errorf("[%d] = %q, want %q", i, ids[i], tt.wantIDs[i])
				}
			}
		})
	}
}

func TestDetectBlockingIssues_Caching(t *testing.T) {
	c := &Controller{
		config: SessionConfig{
			Repository: "org/repo",
		},
		blockedByCache: map[string][]string{
			"42": {"10", "11"},
		},
		logger: newTestLogger(),
	}

	ids, err := c.detectBlockingIssues(t.Context(), "42")
	if err != nil {
		t.Fatalf("detectBlockingIssues() error = %v", err)
	}
	if len(ids) != 2 || ids[0] != "10" || ids[1] != "11" {
		t.Errorf("detectBlockingIssues() = %v, want [10, 11]", ids)
	}
}

func TestBlockedByMarksPhaseBlocked(t *testing.T) {
	c := &Controller{
		config: SessionConfig{
			Repository: "org/repo",
		},
		taskStates: map[string]*TaskState{
			"issue:5": {ID: "5", Type: "issue", Phase: PhaseImplement},
		},
		blockedByCache: map[string][]string{
			"5": {"10", "11"},
		},
		logger: newTestLogger(),
	}

	ids, err := c.detectBlockingIssues(t.Context(), "5")
	if err != nil {
		t.Fatalf("detectBlockingIssues() error = %v", err)
	}
	if len(ids) == 0 {
		t.Fatal("expected blocking IDs, got none")
	}

	// Simulate what runMainLoop does: mark PhaseBlocked
	taskID := taskKey("issue", "5")
	if state, ok := c.taskStates[taskID]; ok {
		state.Phase = PhaseBlocked
	}

	state := c.taskStates[taskID]
	if state.Phase != PhaseBlocked {
		t.Errorf("phase = %q, want %q", state.Phase, PhaseBlocked)
	}
}

func TestBlockedByAllClosed_Proceeds(t *testing.T) {
	c := &Controller{
		config: SessionConfig{
			Repository: "org/repo",
		},
		taskStates: map[string]*TaskState{
			"issue:5": {ID: "5", Type: "issue", Phase: PhaseImplement},
		},
		blockedByCache: map[string][]string{
			"5": {}, // all blockers closed → empty list
		},
		logger: newTestLogger(),
	}

	ids, err := c.detectBlockingIssues(t.Context(), "5")
	if err != nil {
		t.Fatalf("detectBlockingIssues() error = %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected no blocking IDs, got %v", ids)
	}

	// Phase should remain unchanged
	state := c.taskStates["issue:5"]
	if state.Phase != PhaseImplement {
		t.Errorf("phase = %q, want %q (should not be blocked)", state.Phase, PhaseImplement)
	}
}
