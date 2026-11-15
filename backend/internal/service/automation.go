package service

import (
	"context"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// AutomationGate is the master switch for everything the system sends to creators on its own: the chatbot's answers,
// the reminders of the background sweep, the bulk invitations. It is OFF until an admin turns it on. What a booker
// writes and sends in the chat, or the messages the stage buttons prepare for the booker to send, never depend on it.
type AutomationGate interface {
	Enabled(ctx context.Context) (bool, error)
}

// ErrAutomationOff is returned by an action that would send to creators on its own while the switch is off.
var ErrAutomationOff = domain.Conflict("Công tắc gửi tin tự động đang tắt. Hãy bật công tắc trước khi dùng chức năng này.")

// WithAutomation gives the service the master switch.
func (s *OutreachService) WithAutomation(g AutomationGate) *OutreachService {
	s.gate = g
	return s
}

// automationOn reports whether the system may message creators on its own. Not wired, or not readable, means off.
func (s *OutreachService) automationOn(ctx context.Context) bool {
	if s.gate == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	on, err := s.gate.Enabled(ctx)
	return err == nil && on
}

// AutomationOn is automationOn for the handlers.
func (s *OutreachService) AutomationOn(ctx context.Context) bool { return s.automationOn(ctx) }

// ErrBulkOff is returned by a bulk send while its own switch is off.
var ErrBulkOff = domain.Conflict("Công tắc gửi hàng loạt đang tắt. Hãy bật công tắc trong cửa sổ gửi hàng loạt.")

// WithBulkSwitch gives the service the switch of the bulk message. It is not the master switch above: a person who
// presses send is not the system messaging creators on its own, so the two do not depend on each other.
func (s *OutreachService) WithBulkSwitch(g AutomationGate) *OutreachService {
	s.bulk = g
	return s
}

// bulkOn reports whether bulk sending is switched on. Not wired, or not readable, means off.
func (s *OutreachService) bulkOn(ctx context.Context) bool {
	if s.bulk == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	on, err := s.bulk.Enabled(ctx)
	return err == nil && on
}
