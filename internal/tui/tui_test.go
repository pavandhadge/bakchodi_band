package tui

import (
	"reflect"
	"strconv"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pavandhadge/dopamine_blocker/internal/model"
	"github.com/pavandhadge/dopamine_blocker/internal/platform"
)

func TestRequestForMatchesCLIBackendContract(t *testing.T) {
	tests := []struct {
		name     string
		action   pendingAction
		endpoint string
		payload  map[string]any
	}{
		{
			name:     "block url",
			action:   pendingAction{command: commandBlock, target: targetURL, value: "youtube.com"},
			endpoint: "/lock-url",
			payload:  map[string]any{"target": "youtube.com"},
		},
		{
			name:     "block group",
			action:   pendingAction{command: commandBlock, target: targetGroup, value: "social"},
			endpoint: "/lock-group",
			payload:  map[string]any{"target": "social"},
		},
		{
			name:     "block all",
			action:   pendingAction{command: commandBlock, target: targetAll},
			endpoint: "/lock",
			payload:  map[string]any{},
		},
		{
			name:     "allow url",
			action:   pendingAction{command: commandAllow, target: targetURL, value: "youtube.com", reason: "break"},
			endpoint: "/unlock-url",
			payload:  map[string]any{"target": "youtube.com", "reason": "break"},
		},
		{
			name:     "allow group",
			action:   pendingAction{command: commandAllow, target: targetGroup, value: "social", reason: "break"},
			endpoint: "/unlock-group",
			payload:  map[string]any{"target": "social", "reason": "break"},
		},
		{
			name:     "allow all",
			action:   pendingAction{command: commandAllow, target: targetAll, reason: "break"},
			endpoint: "/unlock",
			payload:  map[string]any{"reason": "break"},
		},
		{
			name:     "panic url",
			action:   pendingAction{command: commandPanic, target: targetURL, value: "youtube.com", reason: "urgent"},
			endpoint: "/unlock-url",
			payload:  map[string]any{"target": "youtube.com", "reason": "urgent", "break_glass": true},
		},
		{
			name:     "plan",
			action:   pendingAction{command: commandPlan, hours: 8, reason: "exam"},
			endpoint: "/commit",
			payload:  map[string]any{"hours": 8, "reason": "exam"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint, payload := requestFor(tt.action)
			if endpoint != tt.endpoint {
				t.Fatalf("endpoint mismatch: got %q, want %q", endpoint, tt.endpoint)
			}
			if !reflect.DeepEqual(payload, tt.payload) {
				t.Fatalf("payload mismatch: got %#v, want %#v", payload, tt.payload)
			}
		})
	}
}

func TestAllowFlowMatchesCLIOrder(t *testing.T) {
	m := InitialModel(testConfig())
	m.command = commandAllow
	m.target = targetURL
	m.valueInput.SetValue("youtube.com")
	m.policy = model.FrictionPolicy{ExtraWait: 5, Challenges: 2}

	updated, cmd := m.submit()
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("allow submit should start the wait timer")
	}
	if m.mode != modeWait {
		t.Fatalf("allow should start with wait mode, got %v", m.mode)
	}
	if m.waitRemaining != 35 {
		t.Fatalf("allow URL wait mismatch: got %d, want 35", m.waitRemaining)
	}
	if m.required != 3 {
		t.Fatalf("allow must require at least 3 challenges, got %d", m.required)
	}
	if m.pending.reason != "" {
		t.Fatalf("reason should not be collected before friction, got %q", m.pending.reason)
	}

	m.waitRemaining = 1
	updated, _ = m.Update(waitTickMsg{})
	m = updated.(Model)
	if m.mode != modeChallenge {
		t.Fatalf("after wait, expected challenge mode, got %v", m.mode)
	}

	for m.mode == modeChallenge {
		m.answerInput.SetValue(strconv.Itoa(m.answer))
		updated, _ = m.handleChallengeKey(tea.KeyMsg{Type: tea.KeyEnter})
		m = updated.(Model)
	}
	if m.mode != modeReason {
		t.Fatalf("after solving challenges, expected reason mode, got %v", m.mode)
	}
	if m.working {
		t.Fatal("TUI should not execute unlock before reason is entered")
	}

	m.reasonInput.SetValue("planned break")
	updated, cmd = m.handleReasonKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("reason submit should execute the unlock request")
	}
	if !m.working {
		t.Fatal("model should be working while unlock request is executing")
	}
	if m.pending.reason != "planned break" {
		t.Fatalf("pending reason mismatch: got %q", m.pending.reason)
	}
}

func TestPanicFlowUsesBreakGlassFriction(t *testing.T) {
	m := InitialModel(testConfig())
	m.command = commandPanic
	m.target = targetAll
	m.policy = model.FrictionPolicy{ExtraWait: 7, Challenges: 3}

	updated, cmd := m.submit()
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("panic submit should start the wait timer")
	}
	if m.mode != modeWait {
		t.Fatalf("panic should start with wait mode, got %v", m.mode)
	}
	if m.waitRemaining != 367 {
		t.Fatalf("panic all wait mismatch: got %d, want 367", m.waitRemaining)
	}
	if m.required != 5 {
		t.Fatalf("panic should add 2 challenges, got %d", m.required)
	}
}

func TestBlockAndPlanExecuteWithoutUnlockFriction(t *testing.T) {
	block := InitialModel(testConfig())
	block.command = commandBlock
	block.target = targetGroup
	block.valueInput.SetValue("social")

	updated, cmd := block.submit()
	block = updated.(Model)
	if cmd == nil {
		t.Fatal("block should execute immediately")
	}
	if block.mode != modeForm || !block.working {
		t.Fatalf("block should execute from form without wait, mode=%v working=%v", block.mode, block.working)
	}

	plan := InitialModel(testConfig())
	plan.command = commandPlan
	plan.hours = 12
	plan.reasonInput.SetValue("deep work")

	updated, cmd = plan.submit()
	plan = updated.(Model)
	if cmd == nil {
		t.Fatal("plan should execute immediately")
	}
	if plan.mode != modeForm || !plan.working {
		t.Fatalf("plan should execute from form without wait, mode=%v working=%v", plan.mode, plan.working)
	}
	if plan.pending.reason != "deep work" {
		t.Fatalf("plan reason mismatch: got %q", plan.pending.reason)
	}
}

func testConfig() platform.Config {
	return platform.Config{}
}
