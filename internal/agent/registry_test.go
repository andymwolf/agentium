package agent

import (
	"testing"
)

// mockAgent implements Agent for testing
type mockAgent struct {
	name string
}

func (m *mockAgent) Name() string                                                    { return m.name }
func (m *mockAgent) ContainerImage() string                                          { return "mock:latest" }
func (m *mockAgent) ContainerEntrypoint() []string                                   { return []string{"mock"} }
func (m *mockAgent) BuildEnv(s *Session, i int) map[string]string                    { return nil }
func (m *mockAgent) BuildCommand(s *Session, i int) []string                         { return nil }
func (m *mockAgent) BuildPrompt(s *Session, i int) string                            { return "" }
func (m *mockAgent) ParseOutput(code int, out, err string) (*IterationResult, error) { return nil, nil }
func (m *mockAgent) Validate() error                                                 { return nil }

func TestRegister(t *testing.T) {
	// Clean up registry after test
	originalRegistry := make(map[string]func() Agent)
	for k, v := range registry {
		originalRegistry[k] = v
	}
	defer func() {
		registry = originalRegistry
	}()

	// Clear registry for this test
	registry = make(map[string]func() Agent)

	Register("test-agent", func() Agent {
		return &mockAgent{name: "test-agent"}
	})

	if !Exists("test-agent") {
		t.Error("Register() failed to register agent")
	}

	agent, err := Get("test-agent")
	if err != nil {
		t.Errorf("Get() returned error: %v", err)
	}
	if agent.Name() != "test-agent" {
		t.Errorf("Get() returned agent with name %q, want %q", agent.Name(), "test-agent")
	}
}

func TestGet_NotFound(t *testing.T) {
	_, err := Get("nonexistent-agent")
	if err == nil {
		t.Error("Get() expected error for nonexistent agent, got nil")
	}
}

func TestExists(t *testing.T) {
	// Clean up registry after test
	originalRegistry := make(map[string]func() Agent)
	for k, v := range registry {
		originalRegistry[k] = v
	}
	defer func() {
		registry = originalRegistry
	}()

	// Clear registry for this test
	registry = make(map[string]func() Agent)

	if Exists("not-registered") {
		t.Error("Exists() returned true for unregistered agent")
	}

	Register("registered-agent", func() Agent {
		return &mockAgent{name: "registered-agent"}
	})

	if !Exists("registered-agent") {
		t.Error("Exists() returned false for registered agent")
	}
}

func TestList(t *testing.T) {
	// Clean up registry after test
	originalRegistry := make(map[string]func() Agent)
	for k, v := range registry {
		originalRegistry[k] = v
	}
	defer func() {
		registry = originalRegistry
	}()

	// Clear registry for this test
	registry = make(map[string]func() Agent)

	agents := List()
	if len(agents) != 0 {
		t.Errorf("List() returned %d agents, want 0", len(agents))
	}

	Register("agent1", func() Agent { return &mockAgent{name: "agent1"} })
	Register("agent2", func() Agent { return &mockAgent{name: "agent2"} })

	agents = List()
	if len(agents) != 2 {
		t.Errorf("List() returned %d agents, want 2", len(agents))
	}

	// Check both agents are in the list
	found := make(map[string]bool)
	for _, name := range agents {
		found[name] = true
	}
	if !found["agent1"] || !found["agent2"] {
		t.Errorf("List() = %v, want [agent1, agent2]", agents)
	}
}

func TestRegister_Overwrite(t *testing.T) {
	// Clean up registry after test
	originalRegistry := make(map[string]func() Agent)
	for k, v := range registry {
		originalRegistry[k] = v
	}
	defer func() {
		registry = originalRegistry
	}()

	// Clear registry for this test
	registry = make(map[string]func() Agent)

	Register("overwrite-test", func() Agent {
		return &mockAgent{name: "original"}
	})

	agent1, _ := Get("overwrite-test")
	if agent1.Name() != "original" {
		t.Errorf("First registration returned %q, want %q", agent1.Name(), "original")
	}

	// Register with same name should overwrite
	Register("overwrite-test", func() Agent {
		return &mockAgent{name: "overwritten"}
	})

	agent2, _ := Get("overwrite-test")
	if agent2.Name() != "overwritten" {
		t.Errorf("After overwrite, got %q, want %q", agent2.Name(), "overwritten")
	}
}
