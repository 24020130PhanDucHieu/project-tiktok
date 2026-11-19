package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

type repliedKocs struct {
	kocs map[string]*domain.Koc
	err  error
}

func (f *repliedKocs) FindActiveByID(_ context.Context, id string) (*domain.Koc, error) {
	return f.kocs[id], f.err
}

type repliedThreads struct {
	byHandle map[string]*domain.Conversation
	asked    []string
	err      error
}

func (f *repliedThreads) FindByHandle(_ context.Context, handle string) (*domain.Conversation, error) {
	f.asked = append(f.asked, handle)
	return f.byHandle[handle], f.err
}

// repliedMsgs holds the moments the contact wrote, per conversation.
type repliedMsgs struct {
	inbound map[domain.ID][]time.Time
	err     error
}

func (f *repliedMsgs) InboundSince(_ context.Context, id domain.ID, since time.Time) (bool, error) {
	for _, at := range f.inbound[id] {
		if at.After(since) {
			return true, f.err
		}
	}
	return false, f.err
}

func repliedFixture() (*ReplyLookup, *repliedKocs, *repliedThreads, *repliedMsgs, domain.ID) {
	koc := &domain.Koc{ID: domain.NewID(), TiktokID: "linh"}
	conv := &domain.Conversation{ID: domain.NewID()}
	kocs := &repliedKocs{kocs: map[string]*domain.Koc{koc.ID.Hex(): koc}}
	threads := &repliedThreads{byHandle: map[string]*domain.Conversation{"@linh": conv}}
	msgs := &repliedMsgs{inbound: map[domain.ID][]time.Time{}}
	return NewReplyLookup(kocs, threads, msgs), kocs, threads, msgs, koc.ID
}

func TestRepliedSinceSeesOnlyMessagesAfterTheGivenTime(t *testing.T) {
	l, _, threads, msgs, kocID := repliedFixture()
	wrote := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	msgs.inbound[threads.byHandle["@linh"].ID] = []time.Time{wrote}

	if replied, known, err := l.RepliedSince(context.Background(), kocID.Hex(), wrote.Add(-time.Hour)); err != nil || !known || !replied {
		t.Fatalf("a message after the send time is a reply: %v %v %v", replied, known, err)
	}
	if replied, known, err := l.RepliedSince(context.Background(), kocID.Hex(), wrote.Add(time.Hour)); err != nil || !known || replied {
		t.Fatalf("a message from before the send time is not: %v %v %v", replied, known, err)
	}
	if len(threads.asked) != 2 || threads.asked[0] != "@linh" {
		t.Fatalf("the thread of the KOC is found by its handle: %v", threads.asked)
	}
}

func TestRepliedSinceIsUnknownWhenTheKocHasNoConversation(t *testing.T) {
	l, _, threads, _, kocID := repliedFixture()
	delete(threads.byHandle, "@linh")
	replied, known, err := l.RepliedSince(context.Background(), kocID.Hex(), time.Now())
	if err != nil || known || replied {
		t.Fatalf("no thread: nobody can tell, and it is not an error: %v %v %v", replied, known, err)
	}
}

func TestRepliedSinceIsUnknownForAKocThatIsGone(t *testing.T) {
	l, _, _, _, _ := repliedFixture()
	replied, known, err := l.RepliedSince(context.Background(), domain.NewID().Hex(), time.Now())
	if err != nil || known || replied {
		t.Fatalf("an unknown or archived KOC: %v %v %v", replied, known, err)
	}
}

func TestRepliedSinceReportsAFailureOfTheStores(t *testing.T) {
	for name, fail := range map[string]func(*repliedKocs, *repliedThreads, *repliedMsgs){
		"koc store":     func(k *repliedKocs, _ *repliedThreads, _ *repliedMsgs) { k.err = errors.New("db") },
		"thread store":  func(_ *repliedKocs, th *repliedThreads, _ *repliedMsgs) { th.err = errors.New("db") },
		"message store": func(_ *repliedKocs, _ *repliedThreads, m *repliedMsgs) { m.err = errors.New("db") },
	} {
		t.Run(name, func(t *testing.T) {
			l, kocs, threads, msgs, kocID := repliedFixture()
			fail(kocs, threads, msgs)
			if _, known, err := l.RepliedSince(context.Background(), kocID.Hex(), time.Now()); err == nil || known {
				t.Fatalf("a failing store is an error, never a guess: known=%v err=%v", known, err)
			}
		})
	}
}
