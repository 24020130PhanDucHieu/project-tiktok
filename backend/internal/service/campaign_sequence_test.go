package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/config"
	"github.com/tiktoday/kocflow-api/internal/domain"
)

// seqLine is a Zalo line that records every call in order and can fail at one of them.
type seqLine struct {
	PersonalZalo
	connected bool
	calls     []string
	failAt    int // 1-based index of the call that fails; 0 = none
	failErr   error
	onCall    func(call string)
}

func (l *seqLine) IsConnected(context.Context) bool { return l.connected }
func (l *seqLine) record(call string) error {
	l.calls = append(l.calls, call)
	if l.onCall != nil {
		l.onCall(call)
	}
	if l.failAt != 0 && len(l.calls) == l.failAt {
		return l.failErr
	}
	return nil
}
func (l *seqLine) Send(_ context.Context, _ string, text string) error {
	return l.record("text:" + text)
}
func (l *seqLine) SendImage(_ context.Context, _ string, img ZaloImage) error {
	return l.record("image:" + img.Filename + ":" + img.Caption)
}

// seqConvs is a thread on Zalo that remembers the last message line.
type seqConvs struct {
	fakeKnownThread
	lastText string
}

func (c *seqConvs) SetLastMessageAt(_ context.Context, _ domain.ID, text string, _ time.Time) error {
	c.lastText = text
	return nil
}

type fakeImages struct{ missing bool }

func (f fakeImages) Image(_ context.Context, id string) (*domain.CampaignImage, error) {
	if f.missing {
		return nil, domain.NotFound("no picture")
	}
	return &domain.CampaignImage{Filename: "p-" + id[:4] + ".png", Width: 3, Height: 2, Data: []byte("png")}, nil
}

type seqEnv struct {
	svc   *OutreachService
	line  *seqLine
	kocs  *claimKocs
	msgs  *fakeOutreachMsgs
	convs *seqConvs
}

func newSeqEnv() *seqEnv {
	line := &seqLine{connected: true}
	kocs, msgs, convs := &claimKocs{marks: map[domain.ID]*time.Time{}}, &fakeOutreachMsgs{}, &seqConvs{}
	return &seqEnv{
		line: line, kocs: kocs, msgs: msgs, convs: convs,
		svc: &OutreachService{kocs: kocs, convs: convs, msgs: msgs, zalo: NewZaloService(config.Config{}, line, nil), clock: vnClock(10, 0)},
	}
}

func textStep(s string) domain.CampaignStep {
	return domain.CampaignStep{Kind: domain.StepText, Text: s}
}
func imageStep() domain.CampaignStep {
	return domain.CampaignStep{Kind: domain.StepImage, ImageID: domain.NewID(), Caption: "Ảnh của {kenh}"}
}

func targetFor(k *domain.Koc) domain.RunTarget {
	return domain.RunTarget{KocID: k.ID.Hex(), KocName: k.DisplayName, Handle: "@" + k.TiktokID, Status: domain.TargetQueued}
}

func (e *seqEnv) send(koc *domain.Koc, steps []domain.CampaignStep, h SequenceHooks) (domain.RunTarget, bool) {
	return e.svc.SendSequence(context.Background(), koc, steps, fakeImages{}, targetFor(koc), h)
}

func TestSequenceSendsEveryStepInOrderAndClaimsOnce(t *testing.T) {
	e := newSeqEnv()
	koc := kocWith("Nguyễn Lan", "0976451571")
	koc.TiktokID = "lan99"
	out, fault := e.send(koc, []domain.CampaignStep{textStep("Chào {ten}"), imageStep(), textStep("Kênh {kenh}")}, SequenceHooks{})
	if fault || out.Status != domain.TargetSent || out.StepsSent != 3 || out.StartedAt == nil {
		t.Fatalf("all three went out: %+v fault=%v", out, fault)
	}
	if len(e.line.calls) != 3 || e.line.calls[0] != "text:Chào Lan" || !strings.HasPrefix(e.line.calls[1], "image:") || !strings.HasSuffix(e.line.calls[1], ":Ảnh của @lan99") || e.line.calls[2] != "text:Kênh @lan99" {
		t.Fatalf("in order, with {ten} and {kenh} filled: %v", e.line.calls)
	}
	if e.kocs.claims != 1 {
		t.Fatalf("one claim for the whole chain, got %d", e.kocs.claims)
	}
	if len(e.msgs.all) != 3 || e.msgs.all[1].Text != "[Ảnh] Ảnh của @lan99" {
		t.Fatalf("every step is stored: %+v", e.msgs.all)
	}
	if koc.LastContactedAt == nil {
		t.Fatal("the KOC is marked as contacted")
	}
}

func TestSequenceStopsWhenTheKocReplies(t *testing.T) {
	e := newSeqEnv()
	e.msgs.inbound = func(domain.ID, time.Time) bool { return len(e.line.calls) >= 1 } // answers after the first step
	out, _ := e.send(kocWith("Lan", "0976451571"), []domain.CampaignStep{textStep("a"), textStep("b"), textStep("c")}, SequenceHooks{})
	if out.Status != domain.TargetReplied || out.StepsSent != 1 || len(e.line.calls) != 1 {
		t.Fatalf("one step went out, then the reply stopped it: %+v calls=%v", out, e.line.calls)
	}
}

func TestProgressSaysSendingBeforeZaloIsCalled(t *testing.T) {
	e := newSeqEnv()
	var last domain.RunTarget
	var atCall []string
	e.line.onCall = func(string) { atCall = append(atCall, last.Status) }
	e.send(kocWith("Lan", "0976451571"), []domain.CampaignStep{textStep("a"), textStep("b")}, SequenceHooks{Progress: func(t domain.RunTarget) { last = t }})
	if len(atCall) != 2 || atCall[0] != domain.TargetSending || atCall[1] != domain.TargetSending {
		t.Fatalf("the stored state said \"sending\" when Zalo was called: %v", atCall)
	}
}

func TestAnUnreachableBridgeLeavesTheStepUnknownAndIsNotRetried(t *testing.T) {
	e := newSeqEnv()
	e.line.failAt, e.line.failErr = 2, fmt.Errorf("no answer: %w", ErrZaloUnreachable)
	out, fault := e.send(kocWith("Lan", "0976451571"), []domain.CampaignStep{textStep("a"), textStep("b"), textStep("c")}, SequenceHooks{})
	if out.Status != domain.TargetUnknown || out.Reason != "step_outcome_unknown" || !fault || len(e.line.calls) != 2 || out.StepsSent != 1 {
		t.Fatalf("step 2 is unknown, nothing more is tried: %+v fault=%v calls=%v", out, fault, e.line.calls)
	}
	if e.kocs.releases != 0 {
		t.Fatal("an unknown step may have gone out: the claim stays")
	}
}

func TestARefusedFirstStepReleasesTheClaim(t *testing.T) {
	e := newSeqEnv()
	e.line.failAt, e.line.failErr = 1, errors.New("zalo said no")
	out, fault := e.send(kocWith("Lan", "0976451571"), []domain.CampaignStep{textStep("a"), textStep("b")}, SequenceHooks{})
	if out.Status != domain.TargetFailed || !fault || out.StepsSent != 0 || e.kocs.releases != 1 || len(e.line.calls) != 1 {
		t.Fatalf("failed, claim given back: %+v fault=%v releases=%d", out, fault, e.kocs.releases)
	}
}

func TestAStopDuringThePauseSendsNothingMore(t *testing.T) {
	e := newSeqEnv()
	n := 0
	pause := func(context.Context) error {
		n++
		if n == 2 {
			return errors.New("switch off")
		}
		return nil
	}
	out, fault := e.send(kocWith("Lan", "0976451571"), []domain.CampaignStep{textStep("a"), textStep("b"), textStep("c")}, SequenceHooks{Pause: pause})
	if out.Status != domain.TargetPartial || out.Reason != "stopped" || out.StepsSent != 1 || fault || len(e.line.calls) != 1 {
		t.Fatalf("one step went out before the stop: %+v fault=%v", out, fault)
	}
	e2 := newSeqEnv()
	out2, _ := e2.send(kocWith("Lan", "0976451571"), []domain.CampaignStep{textStep("a")}, SequenceHooks{Pause: func(context.Context) error { return errors.New("off") }})
	if out2.Status != domain.TargetNotSent || out2.Reason != "stopped" || e2.kocs.releases != 1 || len(e2.line.calls) != 0 {
		t.Fatalf("stopped before anything: not sent, claim given back: %+v", out2)
	}
}

func TestCampaignMessagesDoNotSpendTheProactiveQuota(t *testing.T) {
	e := newSeqEnv()
	e.send(kocWith("Lan", "0976451571"), []domain.CampaignStep{textStep("a"), textStep("b"), textStep("c")}, SequenceHooks{})
	if e.svc.dayCount != 0 {
		t.Fatalf("the 80-a-day count is for the other proactive messages, got %d", e.svc.dayCount)
	}
	for _, m := range e.msgs.all {
		if m.Proactive {
			t.Fatalf("a campaign message is not stored as proactive: %+v", m)
		}
	}
}

func TestSequenceSkipsAKocItMayNotContact(t *testing.T) {
	e := newSeqEnv()
	steps := []domain.CampaignStep{textStep("a")}
	dnc := kocWith("B", "0976451571")
	dnc.Dnc = true
	cases := map[string]*domain.Koc{"no_phone": kocWith("A", ""), "dnc_or_blocked": dnc}
	for reason, koc := range cases {
		if out, _ := e.send(koc, steps, SequenceHooks{}); out.Status != domain.TargetSkipped || out.Reason != reason {
			t.Errorf("%s: %+v", reason, out)
		}
	}
	if out, _ := e.svc.SendSequence(context.Background(), nil, steps, fakeImages{}, domain.RunTarget{KocID: "x"}, SequenceHooks{}); out.Status != domain.TargetSkipped || out.Reason != "not_found" {
		t.Errorf("not_found: %+v", out)
	}
	recent := kocWith("C", "0976451571")
	at := e.svc.now().Add(-time.Hour)
	e.kocs.marks[recent.ID] = &at
	if out, _ := e.send(recent, steps, SequenceHooks{}); out.Reason != "already_sent_within_24h" || len(e.line.calls) != 0 {
		t.Errorf("24h rule: %+v", out)
	}
}

func TestAMissingPictureStopsTheChainBeforeIt(t *testing.T) {
	e := newSeqEnv()
	koc := kocWith("Lan", "0976451571")
	out, fault := e.svc.SendSequence(context.Background(), koc, []domain.CampaignStep{textStep("a"), imageStep()}, fakeImages{missing: true}, targetFor(koc), SequenceHooks{})
	if out.Status != domain.TargetFailed || out.Reason != "image_missing" || out.StepsSent != 1 || fault {
		t.Fatalf("the text went, the missing picture stopped it, and that is not Zalo's fault: %+v fault=%v", out, fault)
	}
}

func TestRenderTemplateFillsTheChannel(t *testing.T) {
	if got := RenderTemplate("Kênh {kenh}", &domain.Koc{TiktokID: "abc"}); got != "Kênh @abc" {
		t.Fatalf("got %q", got)
	}
	if got := RenderTemplate("Kênh {kenh}.", &domain.Koc{}); got != "Kênh ." {
		t.Fatalf("no channel: %q", got)
	}
}
