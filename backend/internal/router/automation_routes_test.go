package router_test

import (
	"context"
	"sync"
	"testing"

	"github.com/tiktoday/kocflow-api/internal/repository"
)

// memSwitch keeps the master switch in memory.
type memSwitch struct {
	mu sync.Mutex
	st repository.AutomationState
}

func (m *memSwitch) State(context.Context) (repository.AutomationState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.st, nil
}

func (m *memSwitch) SetEnabled(_ context.Context, enabled bool, by string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.st = repository.AutomationState{Enabled: enabled, UpdatedBy: by}
	return nil
}

// The system must not message creators by itself until an admin has deliberately turned it on.
func TestTheAutomationSwitchIsOffByDefaultAndOnlyAnAdminTurnsItOn(t *testing.T) {
	app, admin, member := newApp(t)

	if code, _ := do(t, app, "GET", "/api/v1/automation", "", ""); code != 401 {
		t.Fatalf("the switch is not public: %d", code)
	}
	code, body := do(t, app, "GET", "/api/v1/automation", member, "")
	data, _ := body["data"].(map[string]any)
	if code != 200 || data["enabled"] != false {
		t.Fatalf("a member sees it, and it starts off: %d %v", code, body)
	}

	if code, _ := do(t, app, "PUT", "/api/v1/automation", member, `{"enabled":true,"confirm":true}`); code != 403 {
		t.Fatalf("a member cannot turn it on: %d", code)
	}
	if code, _ := do(t, app, "PUT", "/api/v1/automation", admin, `{"enabled":true}`); code != 400 {
		t.Fatalf("turning it on needs a confirmation: %d", code)
	}
	if _, body := do(t, app, "GET", "/api/v1/automation", admin, ""); body["data"].(map[string]any)["enabled"] != false {
		t.Fatalf("a refused request changes nothing: %v", body)
	}

	if code, _ := do(t, app, "PUT", "/api/v1/automation", admin, `{"enabled":true,"confirm":true}`); code != 200 {
		t.Fatalf("an admin turns it on: %d", code)
	}
	if _, body := do(t, app, "GET", "/api/v1/automation", member, ""); body["data"].(map[string]any)["enabled"] != true || body["data"].(map[string]any)["updatedBy"] != "admin@t.t" {
		t.Fatalf("on, with who did it: %v", body)
	}
	// turning it off needs no confirmation
	if code, _ := do(t, app, "PUT", "/api/v1/automation", admin, `{"enabled":false}`); code != 200 {
		t.Fatalf("turning it off is immediate: %d", code)
	}
	if _, body := do(t, app, "GET", "/api/v1/automation", member, ""); body["data"].(map[string]any)["enabled"] != false {
		t.Fatalf("off again: %v", body)
	}
}
