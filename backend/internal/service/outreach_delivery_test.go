package service

import (
	"context"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/config"
	"github.com/tiktoday/kocflow-api/internal/domain"
)

// fakeZaloLine is the personal Zalo account: it is either logged in or not.
type fakeZaloLine struct {
	PersonalZalo
	connected bool
	sent      []string
}

func (f *fakeZaloLine) IsConnected(context.Context) bool { return f.connected }
func (f *fakeZaloLine) Send(_ context.Context, id, text string) error {
	f.sent = append(f.sent, id+": "+text)
	return nil
}

// fakeKnownThread answers every KOC with a thread that already knows its Zalo id.
type fakeKnownThread struct{ OutreachConvs }

func (fakeKnownThread) UpsertKocMessage(context.Context, domain.KocContact, time.Time) (*domain.Conversation, error) {
	return &domain.Conversation{ID: domain.NewID(), Channel: domain.ChannelZalo, ExternalContactID: "42"}, nil
}

func deliveryService(line *fakeZaloLine) (*OutreachService, *fakeOutreachMsgs) {
	msgs := &fakeOutreachMsgs{}
	return &OutreachService{convs: fakeKnownThread{}, msgs: msgs, zalo: NewZaloService(config.Config{}, line, nil)}, msgs
}

func TestAnOutreachMessageStaysPendingWhenNoZaloLineCouldCarryIt(t *testing.T) {
	line := &fakeZaloLine{connected: false} // bridge down or logged out, and no official-account token
	svc, msgs := deliveryService(line)
	koc := domain.NewKoc()

	d, err := svc.DeliverMessageToKoc(context.Background(), koc, "Chào bạn", false)
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != domain.MessagePending || len(msgs.all) != 1 || msgs.all[0].Status != domain.MessagePending {
		t.Fatalf("a message nobody delivered must not be stored as sent: delivery %q stored %+v", d.Status, msgs.all)
	}
	if len(line.sent) != 0 {
		t.Fatalf("no line was up, yet %v was sent", line.sent)
	}
}

func TestAnOutreachMessageIsSentWhenThePersonalLineIsUp(t *testing.T) {
	line := &fakeZaloLine{connected: true}
	svc, msgs := deliveryService(line)

	d, err := svc.DeliverMessageToKoc(context.Background(), domain.NewKoc(), "Chào bạn", false)
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != domain.MessageSent || msgs.all[0].Status != domain.MessageSent {
		t.Fatalf("a delivered message is sent: %q", d.Status)
	}
	if len(line.sent) != 1 || line.sent[0] != "42: Chào bạn" {
		t.Fatalf("the text must reach the KOC's Zalo id: %v", line.sent)
	}
}
