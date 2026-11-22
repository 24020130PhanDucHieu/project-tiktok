package service

import (
	"context"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/config"
	"github.com/tiktoday/kocflow-api/internal/domain"
)

// accountLine is a personal Zalo line that notes which account each call was made for.
type accountLine struct {
	PersonalZalo
	sentVia     []string // the account of every Send
	resolvedVia []string // the account of every phone look-up
	connectedOf []string
}

func (l *accountLine) IsConnected(ctx context.Context) bool {
	l.connectedOf = append(l.connectedOf, ZaloAccountFrom(ctx))
	return true
}
func (l *accountLine) Send(ctx context.Context, _, _ string) error {
	l.sentVia = append(l.sentVia, ZaloAccountFrom(ctx))
	return nil
}
func (l *accountLine) ResolveByPhone(ctx context.Context, _ string) (*ZaloUser, error) {
	l.resolvedVia = append(l.resolvedVia, ZaloAccountFrom(ctx))
	return &ZaloUser{UserID: "88"}, nil
}

// defaultIs answers with a fixed default account.
type defaultIs string

func (d defaultIs) Default(context.Context) (*domain.ZaloAccount, error) {
	var id domain.ID
	copy(id[:], []byte(d))
	return &domain.ZaloAccount{ID: id}, nil
}

// threadOf answers every KOC with one thread and notes the account each binding was made under.
type threadOf struct {
	OutreachConvs
	thread  *domain.Conversation
	boundAs []string
}

func (t *threadOf) UpsertKocMessage(context.Context, domain.KocContact, time.Time) (*domain.Conversation, error) {
	c := *t.thread
	return &c, nil
}
func (t *threadOf) SetZaloContact(ctx context.Context, _ domain.ID, _ domain.ZaloContact) error {
	t.boundAs = append(t.boundAs, ZaloAccountFrom(ctx))
	return nil
}

func TestAThreadIsAnsweredThroughItsOwnAccount(t *testing.T) {
	line := &accountLine{}
	svc := &OutreachService{
		convs: &threadOf{thread: &domain.Conversation{ID: domain.NewID(), Channel: domain.ChannelZalo, ExternalContactID: "42", ZaloAccountID: "acc-B"}},
		msgs:  &fakeOutreachMsgs{},
		zalo:  NewZaloService(config.Config{}, line, nil),
	}
	if _, err := svc.DeliverMessageToKoc(context.Background(), domain.NewKoc(), "Chào", false); err != nil {
		t.Fatal(err)
	}
	if len(line.sentVia) != 1 || line.sentVia[0] != "acc-B" {
		t.Fatalf("sent via %v, want the thread's own account acc-B", line.sentVia)
	}
}

func TestAThreadWithoutAnAccountUsesTheDefaultOneAndIsStamped(t *testing.T) {
	line := &accountLine{}
	threads := &threadOf{thread: &domain.Conversation{ID: domain.NewID(), Channel: domain.ChannelInternal, ContactPhone: "0327637213"}}
	zalo := NewZaloService(config.Config{}, line, nil)
	zalo.SetAccounts(defaultIs("default-account"))
	svc := &OutreachService{convs: threads, msgs: &fakeOutreachMsgs{}, zalo: zalo}

	if _, err := svc.DeliverMessageToKoc(context.Background(), domain.NewKoc(), "Chào", true); err != nil {
		t.Fatal(err)
	}
	wantID := domain.ID{}
	copy(wantID[:], []byte("default-account"))
	want := wantID.Hex()
	if len(line.resolvedVia) != 1 || line.resolvedVia[0] != want {
		t.Fatalf("the phone was looked up through %v, want the default account %s", line.resolvedVia, want)
	}
	if len(threads.boundAs) != 1 || threads.boundAs[0] != want {
		t.Fatalf("the thread was bound under %v, want it stamped with the default account %s", threads.boundAs, want)
	}
	if len(line.sentVia) != 1 || line.sentVia[0] != want {
		t.Fatalf("sent via %v, want the default account %s", line.sentVia, want)
	}
}

func TestWithoutStoredAccountsNothingNamesAnAccount(t *testing.T) {
	line := &accountLine{}
	svc := &OutreachService{
		convs: &threadOf{thread: &domain.Conversation{ID: domain.NewID(), Channel: domain.ChannelZalo, ExternalContactID: "42"}},
		msgs:  &fakeOutreachMsgs{},
		zalo:  NewZaloService(config.Config{}, line, nil),
	}
	if _, err := svc.DeliverMessageToKoc(context.Background(), domain.NewKoc(), "Chào", false); err != nil {
		t.Fatal(err)
	}
	if len(line.sentVia) != 1 || line.sentVia[0] != "" {
		t.Fatalf("sent via %v, want no account named (the old single account)", line.sentVia)
	}
}

func TestForConvPrefersTheThreadsAccountOverTheDefault(t *testing.T) {
	zalo := NewZaloService(config.Config{}, &accountLine{}, nil)
	zalo.SetAccounts(defaultIs("default-account"))
	got := ZaloAccountFrom(zalo.ForConv(context.Background(), &domain.Conversation{ZaloAccountID: "acc-B"}))
	if got != "acc-B" {
		t.Fatalf("account = %q, want acc-B", got)
	}
}

func TestForConvOnAServiceThatHasNoZaloLeavesTheContextAlone(t *testing.T) {
	var z *ZaloService // outreach built without Zalo (nothing to send through)
	ctx := context.Background()
	if got := z.ForConv(ctx, &domain.Conversation{}); got != ctx {
		t.Fatal("the context changed")
	}
	if got := ZaloAccountFrom(z.ForConv(ctx, &domain.Conversation{ZaloAccountID: "acc-B"})); got != "acc-B" {
		t.Fatalf("account = %q: a thread's own account still counts without a service", got)
	}
}
