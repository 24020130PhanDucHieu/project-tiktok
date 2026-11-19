package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// fakeEmailer stands for the email sender: it records who was emailed and when, and refuses a KOC it already emailed (the
// claim of the real one).
type fakeEmailer struct {
	mu            sync.Mutex
	clock         func() time.Time
	status        map[string]EmailStatus // the answer for a KOC (sent when absent)
	errs          map[string]error
	notConfigured bool
	sent          []string
	times         []time.Time
	subjects      []string
	texts         []string
	emailed       map[string]bool
}

func newFakeEmailer(clock func() time.Time) *fakeEmailer {
	return &fakeEmailer{clock: clock, status: map[string]EmailStatus{}, errs: map[string]error{}, emailed: map[string]bool{}}
}

func (f *fakeEmailer) SendEmailTo(_ context.Context, kocID, subject, text string) (EmailStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.notConfigured {
		return EmailFailed, ErrEmailNotConfigured
	}
	if err := f.errs[kocID]; err != nil {
		return EmailFailed, err
	}
	if st, ok := f.status[kocID]; ok && st != EmailSent {
		return st, nil
	}
	if f.emailed[kocID] {
		return EmailRecent, nil
	}
	f.emailed[kocID] = true
	f.sent = append(f.sent, kocID)
	f.times = append(f.times, f.clock())
	f.subjects = append(f.subjects, subject)
	f.texts = append(f.texts, text)
	return EmailSent, nil
}

func emailEnv() (*wfEnv, *fakeEmailer) {
	e := newWfEnv()
	f := newFakeEmailer(func() time.Time { return e.clock })
	e.r.WithEmailer(f)
	e.r.emailPause = func() time.Duration { return 5 * time.Second }
	return e, f
}

func emailGraph() ([]domain.WorkflowNode, []domain.WorkflowEdge) {
	return []domain.WorkflowNode{wnode("i", domain.NodeImport), {ID: "m", Type: domain.NodeEmail, Subject: "Hợp tác", Text: "Chào {ten}"}, wwait("w", 5)},
		[]domain.WorkflowEdge{wedge("i", domain.PortOut, "m"), wedge("m", domain.PortOut, "w")}
}

func TestEmailBlockEmailsOneKocAtATimeAndPassesOnTheOnesThatGotTheEmail(t *testing.T) {
	e, f := emailEnv()
	f.status["k1"] = EmailNoAddress
	f.status["k2"] = EmailRecent
	nodes, edges := emailGraph()
	var carried []domain.TokenKoc
	var seen bool
	e.watchTokenAt("w", &carried, &seen)
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "m", Kocs: tokenKocs("k0", "k1", "k2", "k3"), ReadyAt: e.clock})

	if strings.Join(f.sent, ",") != "k0,k3" {
		t.Fatalf("only the KOCs that can be emailed, in order: %v", f.sent)
	}
	if f.subjects[0] != "Hợp tác" || f.texts[0] != "Chào {ten}" {
		t.Fatalf("the block hands over its subject and text (the sender fills the name in): %q %q", f.subjects[0], f.texts[0])
	}
	if gap := f.times[1].Sub(f.times[0]); gap < 5*time.Second {
		t.Fatalf("two emails are never closer than the pause: %v", gap)
	}
	if ids(carried) != "k0,k3" || carried[0].SentAt == nil {
		t.Fatalf("the KOCs that got the email go on, each with the moment it was sent: %+v", carried)
	}
	s, _ := e.stateOf("m")
	if s.Status != domain.NodeDone || s.Count != 2 || !strings.Contains(s.Reason, "2 bỏ qua") {
		t.Fatalf("the block says what it did: %+v", s)
	}
	if got := e.store.only(); got.Status != domain.WorkflowDone {
		t.Fatalf("%s %q", got.Status, got.StopReason)
	}
}

func TestEmailBlockWaitsForTheSendWindow(t *testing.T) {
	e, f := emailEnv()
	e.clock = time.Date(2026, 11, 20, 22, 0, 0, 0, vnZone)
	e.t0 = e.clock
	nodes, edges := emailGraph()
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "m", Kocs: tokenKocs("k0"), ReadyAt: e.clock})
	open := time.Date(2026, 11, 21, 8, 30, 0, 0, vnZone)
	if len(f.times) != 1 || f.times[0].Before(open) || f.times[0].After(open.Add(2*time.Minute)) {
		t.Fatalf("no email at 22:00: it goes out when the window opens (%v), not at %v", open, f.times)
	}
}

func TestEmailBlockSkipsAKocWhoAnsweredSinceItWasLastSentTo(t *testing.T) {
	e, f := emailEnv()
	e.replies.answer("k1", true, true)
	nodes, edges := emailGraph()
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "m", Kocs: sentKocs(e.clock.Add(-48*time.Hour), "k0", "k1"), ReadyAt: e.clock})
	if strings.Join(f.sent, ",") != "k0" {
		t.Fatalf("the KOC who answered is not emailed: %v", f.sent)
	}
	if s, _ := e.stateOf("m"); !strings.Contains(s.Reason, "1 bỏ qua") {
		t.Fatalf("%+v", s)
	}
}

func TestEmailBlockDoesNotEmailAKocItCannotCheckForAReply(t *testing.T) {
	e, f := emailEnv()
	e.replies.answers["k0"] = replyAnswer{errTimes: 1000}
	nodes, edges := emailGraph()
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "m", Kocs: sentKocs(e.clock.Add(-time.Hour), "k0", "k1"), ReadyAt: e.clock})
	if strings.Join(f.sent, ",") != "k1" {
		t.Fatalf("a KOC whose reply cannot be checked is left out after the retries: %v", f.sent)
	}
}

func TestEmailBlockStopsAtOneHundredEmails(t *testing.T) {
	e, f := emailEnv()
	nodes, edges := emailGraph()
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "m", Kocs: kocTokens(130), ReadyAt: e.clock})
	if len(f.sent) != MaxEmailsPerBlock {
		t.Fatalf("at most %d emails: %d", MaxEmailsPerBlock, len(f.sent))
	}
	s, _ := e.stateOf("m")
	if s.Status != domain.NodeDone || s.Count != MaxEmailsPerBlock || !strings.Contains(s.Reason, "đủ 100 email") {
		t.Fatalf("the block says it reached the limit: %+v", s)
	}
}

func TestEmailBlockStopsTheWorkflowAfterTwoFailuresInARow(t *testing.T) {
	e, f := emailEnv()
	f.errs["k0"] = errors.New("smtp refused")
	f.errs["k1"] = errors.New("smtp refused")
	nodes, edges := emailGraph()
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "m", Kocs: tokenKocs("k0", "k1", "k2"), ReadyAt: e.clock})
	got := e.store.only()
	if got.Status != domain.WorkflowStopped || got.StopReason != "email_errors" || len(f.sent) != 0 {
		t.Fatalf("%s %q sent=%v", got.Status, got.StopReason, f.sent)
	}
}

func TestEmailBlockGoesOnAfterASingleFailure(t *testing.T) {
	e, f := emailEnv()
	f.errs["k0"] = errors.New("smtp refused")
	nodes, edges := emailGraph()
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "m", Kocs: tokenKocs("k0", "k1", "k2"), ReadyAt: e.clock})
	if got := e.store.only(); got.Status != domain.WorkflowDone || strings.Join(f.sent, ",") != "k1,k2" {
		t.Fatalf("%s %q sent=%v", got.Status, got.StopReason, f.sent)
	}
}

func TestEmailBlockStopsTheWorkflowWhenSMTPIsNotConfigured(t *testing.T) {
	e, f := emailEnv()
	f.notConfigured = true
	nodes, edges := emailGraph()
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "m", Kocs: tokenKocs("k0"), ReadyAt: e.clock})
	if got := e.store.only(); got.Status != domain.WorkflowStopped || got.StopReason != "email_not_configured" {
		t.Fatalf("%s %q", got.Status, got.StopReason)
	}
}

func TestEmailBlockNeverEmailsTheSameKocTwiceWhenItRunsAgainAfterARestart(t *testing.T) {
	e, f := emailEnv()
	nodes, edges := emailGraph()
	tok := domain.WorkflowToken{NodeID: "m", Kocs: tokenKocs("k0", "k1"), ReadyAt: e.clock}
	wf := e.begin(nodes, edges, tok)
	f.sent, f.times = nil, nil
	delete(f.emailed, "k1") // only k0 was emailed before the server died; the token was not saved after it

	e.r.advanceEmail(context.Background(), wf, nodes[1], tok) // after the restart the token still has k0 at its head
	if len(f.sent) != 0 {
		t.Fatalf("k0 already got its email (the sender remembers for a day): it is skipped, not emailed again: %v", f.sent)
	}
}

func TestStoppingTheWorkflowStopsTheEmailsAtOnce(t *testing.T) {
	e, f := emailEnv()
	nodes, edges := emailGraph()
	e.onNap = func(n int) {
		if n == 3 {
			e.store.askToStop()
		}
	}
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "m", Kocs: kocTokens(50), ReadyAt: e.clock})
	got := e.store.only()
	if got.Status != domain.WorkflowStopped || got.StopReason != "stopped_by_user" {
		t.Fatalf("%s %q", got.Status, got.StopReason)
	}
	if len(f.sent) == 0 || len(f.sent) > 4 {
		t.Fatalf("a few emails went out before the stop, and none after it: %d", len(f.sent))
	}
}

func TestAnEmailBlockWithoutASenderStopsTheWorkflow(t *testing.T) {
	e := newWfEnv()
	nodes, edges := emailGraph()
	e.begin(nodes, edges, domain.WorkflowToken{NodeID: "m", Kocs: tokenKocs("k0"), ReadyAt: e.clock})
	if got := e.store.only(); got.Status != domain.WorkflowStopped || got.StopReason != "email_unavailable" {
		t.Fatalf("%s %q", got.Status, got.StopReason)
	}
}
