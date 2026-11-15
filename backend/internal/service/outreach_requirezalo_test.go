package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/config"
	"github.com/tiktoday/kocflow-api/internal/domain"
)

// at10 is 10:00 on a Monday in Vietnam, inside the 08:30 to 20:30 window.
func vnClock(h, m int) func() time.Time {
	return func() time.Time { return time.Date(2026, 11, 16, h, m, 0, 0, vnZone) }
}

// fakeInternalThread answers every KOC with a thread that is not linked to Zalo and has no phone to look up.
type fakeInternalThread struct{ OutreachConvs }

func (fakeInternalThread) UpsertKocMessage(context.Context, domain.KocContact, time.Time) (*domain.Conversation, error) {
	return &domain.Conversation{ID: domain.NewID(), Channel: domain.ChannelInternal}, nil
}

func TestTheSendWindowFollowsTheServiceClock(t *testing.T) {
	svc := &OutreachService{msgs: &fakeOutreachMsgs{}}

	svc.clock = vnClock(22, 0)
	if check := svc.CanSendProactiveMessage(domain.NewKoc(), false); check.Allowed || check.Reason != "outside_0830_2030_window" {
		t.Fatalf("22:00 is outside the window: %+v", check)
	}
	svc.clock = vnClock(10, 0)
	if check := svc.CanSendProactiveMessage(domain.NewKoc(), false); !check.Allowed {
		t.Fatalf("10:00 is inside the window: %+v", check)
	}
}

func TestTheTwentyFourHourRuleFollowsTheServiceClock(t *testing.T) {
	svc := &OutreachService{msgs: &fakeOutreachMsgs{}, clock: vnClock(10, 0)}
	contacted := func(ago time.Duration) *domain.Koc {
		koc := domain.NewKoc()
		at := svc.clock().Add(-ago)
		koc.LastContactedAt = &at
		return koc
	}

	if check := svc.CanSendProactiveMessage(contacted(23*time.Hour), false); check.Allowed || check.Reason != "already_sent_within_24h" {
		t.Fatalf("23 hours ago is within 24 hours: %+v", check)
	}
	if check := svc.CanSendProactiveMessage(contacted(25*time.Hour), false); !check.Allowed {
		t.Fatalf("25 hours ago is outside 24 hours: %+v", check)
	}
}

func TestRequireZaloSendsNothingToAThreadWithoutAZaloAccount(t *testing.T) {
	msgs := &fakeOutreachMsgs{}
	svc := &OutreachService{convs: fakeInternalThread{}, msgs: msgs, zalo: NewZaloService(config.Config{}, &fakeZaloLine{connected: true}, nil)}

	d, err := svc.deliverToKoc(context.Background(), domain.NewKoc(), "Chào", true, true)
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != DeliveryNotOnZalo {
		t.Fatalf("a KOC without a Zalo account is reported as such, got %q", d.Status)
	}
	if len(msgs.all) != 0 || svc.dayCount != 0 {
		t.Fatalf("nothing is stored and no quota is spent: %d messages, count %d", len(msgs.all), svc.dayCount)
	}
}

func TestRequireZaloStillSendsWhenTheThreadIsOnZalo(t *testing.T) {
	line := &fakeZaloLine{connected: true}
	svc, msgs := deliveryService(line)

	d, err := svc.deliverToKoc(context.Background(), domain.NewKoc(), "Chào bạn", true, true)
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != domain.MessageSent || len(msgs.all) != 1 || len(line.sent) != 1 {
		t.Fatalf("a KOC on Zalo is sent normally: status %q, %d stored, %d sent", d.Status, len(msgs.all), len(line.sent))
	}
}

// fakePhoneThread answers every KOC with a thread that is not linked to Zalo but has a phone to look up.
type fakePhoneThread struct{ OutreachConvs }

func (fakePhoneThread) UpsertKocMessage(context.Context, domain.KocContact, time.Time) (*domain.Conversation, error) {
	return &domain.Conversation{ID: domain.NewID(), Channel: domain.ChannelInternal, ContactPhone: "0976451571"}, nil
}

// fakeFailingLookup is a Zalo line whose phone lookup always fails (the line is down, or the phone has no account).
type fakeFailingLookup struct{ fakeZaloLine }

func (f *fakeFailingLookup) ResolveByPhone(context.Context, string) (*ZaloUser, error) {
	return nil, errors.New("lookup failed")
}

func TestAFailedLookupOnADeadLineIsZaloNotReady(t *testing.T) {
	msgs := &fakeOutreachMsgs{}
	svc := &OutreachService{convs: fakePhoneThread{}, msgs: msgs, zalo: NewZaloService(config.Config{}, &fakeFailingLookup{fakeZaloLine: fakeZaloLine{connected: false}}, nil)}

	d, err := svc.deliverToKoc(context.Background(), domain.NewKoc(), "Chào", true, true)
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != DeliveryZaloNotReady || len(msgs.all) != 0 || svc.dayCount != 0 {
		t.Fatalf("a dead line says nothing about the phone: %q, %d messages, count %d", d.Status, len(msgs.all), svc.dayCount)
	}
}

func TestAFailedLookupOnALiveLineIsNoZaloAccount(t *testing.T) {
	svc := &OutreachService{convs: fakePhoneThread{}, msgs: &fakeOutreachMsgs{}, zalo: NewZaloService(config.Config{}, &fakeFailingLookup{fakeZaloLine: fakeZaloLine{connected: true}}, nil)}

	d, err := svc.deliverToKoc(context.Background(), domain.NewKoc(), "Chào", true, true)
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != DeliveryNotOnZalo {
		t.Fatalf("the line is up and the lookup found nothing: %q", d.Status)
	}
}

// failingMsgs is a message store whose Create fails.
type failingMsgs struct{ fakeOutreachMsgs }

func (f *failingMsgs) Create(context.Context, *domain.Message) error { return errors.New("db down") }

func TestAMessageThatWentOutIsStillReturnedWhenItCouldNotBeStored(t *testing.T) {
	line := &fakeZaloLine{connected: true}
	svc, _ := deliveryService(line)
	svc.msgs = &failingMsgs{}

	d, err := svc.deliverToKoc(context.Background(), domain.NewKoc(), "Chào bạn", true, false)
	if err == nil {
		t.Fatal("the store error is still reported")
	}
	if d == nil || d.Status != domain.MessageSent || len(line.sent) != 1 {
		t.Fatalf("the caller must learn that the message went out: %+v, %d sent", d, len(line.sent))
	}
}
