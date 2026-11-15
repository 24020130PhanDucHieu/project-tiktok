package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/config"
	"github.com/tiktoday/kocflow-api/internal/domain"
)

func TestRenderTemplateUsesTheLastWordOfTheName(t *testing.T) {
	koc := &domain.Koc{DisplayName: "Nguyễn Thị Lan"}
	if got := RenderTemplate("Chào {ten}, {ten} ơi", koc); got != "Chào Lan, Lan ơi" {
		t.Fatalf("every {ten} becomes the last word of the name, got %q", got)
	}
}

func TestRenderTemplateFallsBackToBanWhenTheNameIsEmpty(t *testing.T) {
	koc := &domain.Koc{DisplayName: "  "}
	if got := RenderTemplate("Chào {ten}!", koc); got != "Chào bạn!" {
		t.Fatalf("a KOC without a name is greeted as \"bạn\", not \"Chào !\", got %q", got)
	}
}

func TestRenderTemplateLeavesATemplateWithoutTheTagUntouched(t *testing.T) {
	if got := RenderTemplate("Xin chào", &domain.Koc{DisplayName: "Lan"}); got != "Xin chào" {
		t.Fatalf("a template without {ten} is sent as it is, got %q", got)
	}
}

// ---- SendTemplateBatch ------------------------------------------------------------------------------------------

type fakeGate struct{ on bool }

func (g fakeGate) Enabled(context.Context) (bool, error) { return g.on, nil }

// claimKocs is a KOC store with the atomic contact mark: marks is the lastContactedAt of every KOC "in the database".
type claimKocs struct {
	fakeOutreachKocs
	marks    map[domain.ID]*time.Time
	claims   int
	releases int
	claimErr error
}

func (c *claimKocs) ClaimContact(_ context.Context, id domain.ID, notAfter, at time.Time) (bool, *time.Time, error) {
	if c.claimErr != nil {
		return false, nil, c.claimErr
	}
	prev := c.marks[id]
	if prev != nil && prev.After(notAfter) {
		return false, prev, nil
	}
	c.claims++
	mark := at
	c.marks[id] = &mark
	return true, prev, nil
}

func (c *claimKocs) ReleaseContact(_ context.Context, id domain.ID, at time.Time, previous *time.Time) error {
	c.releases++
	if m := c.marks[id]; m != nil && m.Equal(at) {
		c.marks[id] = previous
	}
	return nil
}

// badHandleThreads fails for the KOC whose TikTok id is "bad" and answers every other KOC with a thread on Zalo.
type badHandleThreads struct{ fakeKnownThread }

func (f badHandleThreads) UpsertKocMessage(ctx context.Context, c domain.KocContact, at time.Time) (*domain.Conversation, error) {
	if c.Handle == "@bad" {
		return nil, errors.New("thread store down")
	}
	return f.fakeKnownThread.UpsertKocMessage(ctx, c, at)
}

// batchService is a service whose clock reads 10:00 in Vietnam, with every KOC on Zalo and the switch as asked.
func batchService(line *fakeZaloLine, gateOn bool) (*OutreachService, *claimKocs, *fakeOutreachMsgs) {
	kocs, msgs := &claimKocs{marks: map[domain.ID]*time.Time{}}, &fakeOutreachMsgs{}
	return &OutreachService{
		kocs: kocs, convs: fakeKnownThread{}, msgs: msgs, zalo: NewZaloService(config.Config{}, line, nil),
		gate: fakeGate{on: gateOn}, clock: vnClock(10, 0),
	}, kocs, msgs
}

func kocWith(name, phone string) *domain.Koc {
	koc := domain.NewKoc()
	koc.ID = domain.NewID()
	koc.DisplayName, koc.Phone = name, phone
	return koc
}

// summary lists "id:status" or "id:status:reason" for every result, to compare a whole report at once.
func summary(r *BatchReport) []string {
	var out []string
	for _, x := range r.Results {
		s := x.KocID + ":" + x.Status
		if x.Reason != "" {
			s += ":" + x.Reason
		}
		out = append(out, s)
	}
	return out
}

func sameList(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestABatchIsRefusedWhenTheAutomationSwitchIsOff(t *testing.T) {
	svc, _, msgs := batchService(&fakeZaloLine{connected: true}, false)

	_, err := svc.SendTemplateBatch(context.Background(), "Chào {ten}", []BatchTarget{{ID: "a", Koc: kocWith("An", "0976451571")}})
	if !errors.Is(err, ErrAutomationOff) {
		t.Fatalf("a bulk send needs the master switch: %v", err)
	}
	if len(msgs.all) != 0 {
		t.Fatalf("nothing is sent with the switch off, got %d messages", len(msgs.all))
	}
}

func TestAllEligibleKocsAreSentAndMarkedContacted(t *testing.T) {
	line := &fakeZaloLine{connected: true}
	svc, kocs, _ := batchService(line, true)
	targets := []BatchTarget{
		{ID: "a", Koc: kocWith("Nguyễn Văn An", "0976451571")},
		{ID: "b", Koc: kocWith("Trần Bình", "0976451572")},
		{ID: "c", Koc: kocWith("Lê Chi", "0976451573")},
	}

	rep, err := svc.SendTemplateBatch(context.Background(), "Chào {ten}", targets)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a:sent", "b:sent", "c:sent"}; !sameList(summary(rep), want) || rep.Stop {
		t.Fatalf("all three are sent and the batch does not stop: %v stop=%v", summary(rep), rep.Stop)
	}
	for _, tg := range targets {
		if tg.Koc.LastContactedAt == nil || !tg.Koc.LastContactedAt.Equal(svc.now()) {
			t.Fatalf("%s must be marked contacted at the service clock: %v", tg.ID, tg.Koc.LastContactedAt)
		}
	}
	if kocs.claims != 3 || kocs.releases != 0 {
		t.Fatalf("each KOC is claimed once and none is given back, got %d claims, %d releases", kocs.claims, kocs.releases)
	}
	for _, tg := range targets {
		if kocs.marks[tg.Koc.ID] == nil {
			t.Fatalf("%s keeps its contact mark in the store", tg.ID)
		}
	}
	if len(line.sent) != 3 || line.sent[0] != "42: Chào An" || line.sent[1] != "42: Chào Bình" {
		t.Fatalf("every KOC gets the text with its own name: %v", line.sent)
	}
}

func TestEachSkipReasonIsReportedAndTheRestStillSend(t *testing.T) {
	line := &fakeZaloLine{connected: true}
	svc, _, msgs := batchService(line, true)
	dnc := kocWith("Dnc", "0976451571")
	dnc.Dnc = true
	recent := kocWith("Recent", "0976451572")
	at := svc.now().Add(-2 * time.Hour)
	recent.LastContactedAt = &at
	targets := []BatchTarget{
		{ID: "gone", Koc: nil},
		{ID: "nophone", Koc: kocWith("NoPhone", "")},
		{ID: "dnc", Koc: dnc},
		{ID: "recent", Koc: recent},
		{ID: "ok", Koc: kocWith("Ok", "0976451573")},
	}

	rep, err := svc.SendTemplateBatch(context.Background(), "Chào {ten}", targets)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"gone:skipped:not_found", "nophone:skipped:no_phone", "dnc:skipped:dnc_or_blocked", "recent:skipped:already_sent_within_24h", "ok:sent"}
	if !sameList(summary(rep), want) || rep.Stop {
		t.Fatalf("each skip has its reason and the eligible KOC is still sent: %v stop=%v", summary(rep), rep.Stop)
	}
	if len(msgs.all) != 1 {
		t.Fatalf("only the eligible KOC gets a message, got %d", len(msgs.all))
	}
}

func TestTheQuotaRunningOutStopsTheBatch(t *testing.T) {
	svc, _, msgs := batchService(&fakeZaloLine{connected: true}, true)
	msgs.all = sentToday(maxProactivePerDay-1, true).all // one message of today's quota is left
	targets := []BatchTarget{
		{ID: "a", Koc: kocWith("An", "0976451571")},
		{ID: "b", Koc: kocWith("Bình", "0976451572")},
		{ID: "c", Koc: kocWith("Chi", "0976451573")},
	}

	rep, err := svc.SendTemplateBatch(context.Background(), "Chào {ten}", targets)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a:sent", "b:not_sent:daily_quota_reached", "c:not_sent:daily_quota_reached"}
	if !sameList(summary(rep), want) || !rep.Stop || rep.StopReason != "daily_quota_reached" {
		t.Fatalf("the last quota message goes, the rest are not sent and the batch stops: %v stop=%v reason=%q", summary(rep), rep.Stop, rep.StopReason)
	}
}

func TestOutsideTheSendWindowNothingIsSentAndTheBatchStops(t *testing.T) {
	svc, _, msgs := batchService(&fakeZaloLine{connected: true}, true)
	svc.clock = vnClock(22, 0)
	targets := []BatchTarget{{ID: "a", Koc: kocWith("An", "0976451571")}, {ID: "b", Koc: kocWith("Bình", "0976451572")}}

	rep, err := svc.SendTemplateBatch(context.Background(), "Chào {ten}", targets)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a:not_sent:outside_0830_2030_window", "b:not_sent:outside_0830_2030_window"}
	if !sameList(summary(rep), want) || !rep.Stop || len(msgs.all) != 0 {
		t.Fatalf("at 22:00 nothing goes out: %v stop=%v messages=%d", summary(rep), rep.Stop, len(msgs.all))
	}
}

func TestAKocWithoutAZaloAccountIsSkippedAndCostsNoQuota(t *testing.T) {
	svc, _, msgs := batchService(&fakeZaloLine{connected: true}, true)
	svc.convs = fakeInternalThread{} // the phone finds no Zalo account

	rep, err := svc.SendTemplateBatch(context.Background(), "Chào {ten}", []BatchTarget{{ID: "a", Koc: kocWith("An", "0976451571")}})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a:skipped:not_on_zalo"}; !sameList(summary(rep), want) {
		t.Fatalf("a KOC without Zalo is never reported as sent: %v", summary(rep))
	}
	if len(msgs.all) != 0 || svc.dayCount != 0 {
		t.Fatalf("no message stored and no quota spent: %d messages, count %d", len(msgs.all), svc.dayCount)
	}
}

func TestAPendingMessageStopsTheRestOfTheBatch(t *testing.T) {
	svc, kocs, _ := batchService(&fakeZaloLine{connected: false}, true) // the Zalo line is down
	first, second := kocWith("An", "0976451571"), kocWith("Bình", "0976451572")

	rep, err := svc.SendTemplateBatch(context.Background(), "Chào {ten}", []BatchTarget{{ID: "a", Koc: first}, {ID: "b", Koc: second}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a:pending", "b:not_sent:zalo_not_ready"}
	if !sameList(summary(rep), want) || !rep.Stop || rep.StopReason != "zalo_not_ready" {
		t.Fatalf("one pending message stops the batch before more quota is spent: %v stop=%v reason=%q", summary(rep), rep.Stop, rep.StopReason)
	}
	if first.LastContactedAt != nil || kocs.marks[first.ID] != nil || kocs.releases != 1 {
		t.Fatalf("a message that did not reach the KOC is not a contact, the mark is given back: %v, %d releases", kocs.marks[first.ID], kocs.releases)
	}
}

// A message that Zalo accepted but the store could not record has gone out: it must not be reported as failed (the user
// would send it again), and the contact mark stays.
func TestAMessageThatWentOutButWasNotRecordedIsStillSentAndStopsTheBatch(t *testing.T) {
	line := &fakeZaloLine{connected: true}
	svc, kocs, _ := batchService(line, true)
	svc.msgs = &failingMsgs{}
	first := kocWith("An", "0976451571")

	rep, err := svc.SendTemplateBatch(context.Background(), "Chào {ten}", []BatchTarget{{ID: "a", Koc: first}, {ID: "b", Koc: kocWith("Bình", "0976451572")}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a:sent:message_not_recorded", "b:not_sent:message_not_recorded"}
	if !sameList(summary(rep), want) || !rep.Stop || rep.StopReason != "message_not_recorded" {
		t.Fatalf("the message did go out: %v stop=%v reason=%q", summary(rep), rep.Stop, rep.StopReason)
	}
	if len(line.sent) != 1 || kocs.marks[first.ID] == nil || kocs.releases != 0 {
		t.Fatalf("one message went out and the contact mark stays: %d sent, mark %v, %d releases", len(line.sent), kocs.marks[first.ID], kocs.releases)
	}
}

// Two people (or two tabs) sending to the same KOC at the same moment both loaded the KOC before either marked it.
func TestAKocClaimedByAnotherSenderMeanwhileIsSkippedAndNothingIsSent(t *testing.T) {
	line := &fakeZaloLine{connected: true}
	svc, kocs, msgs := batchService(line, true)
	koc := kocWith("An", "0976451571") // our copy says "never contacted"
	justNow := svc.now().Add(-time.Minute)
	kocs.marks[koc.ID] = &justNow // ... but the store says somebody just marked it

	rep, err := svc.SendTemplateBatch(context.Background(), "Chào {ten}", []BatchTarget{{ID: "a", Koc: koc}})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a:skipped:already_sent_within_24h"}; !sameList(summary(rep), want) {
		t.Fatalf("the store decides, not the copy loaded earlier: %v", summary(rep))
	}
	if len(msgs.all) != 0 || len(line.sent) != 0 {
		t.Fatalf("nothing is sent: %d stored, %d sent", len(msgs.all), len(line.sent))
	}
}

func TestAClaimErrorIsFailedAndNothingIsSent(t *testing.T) {
	line := &fakeZaloLine{connected: true}
	svc, kocs, _ := batchService(line, true)
	kocs.claimErr = errors.New("db down")

	rep, err := svc.SendTemplateBatch(context.Background(), "Chào {ten}", []BatchTarget{{ID: "a", Koc: kocWith("An", "0976451571")}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Results) != 1 || rep.Results[0].Status != BatchFailed || len(line.sent) != 0 {
		t.Fatalf("without the claim nothing is sent: %+v, %d sent", rep.Results, len(line.sent))
	}
}

func TestTheClaimIsGivenBackWhenNothingWasSent(t *testing.T) {
	svc, kocs, _ := batchService(&fakeZaloLine{connected: true}, true)
	svc.convs = badHandleThreads{}
	bad := kocWith("Bad", "0976451571")
	bad.TiktokID = "bad"

	if _, err := svc.SendTemplateBatch(context.Background(), "Chào {ten}", []BatchTarget{{ID: "bad", Koc: bad}}); err != nil {
		t.Fatal(err)
	}
	if kocs.marks[bad.ID] != nil || kocs.releases != 1 {
		t.Fatalf("an error before any send gives the mark back: %v, %d releases", kocs.marks[bad.ID], kocs.releases)
	}
}

func TestAKocWithoutAZaloAccountGetsItsClaimBack(t *testing.T) {
	svc, kocs, _ := batchService(&fakeZaloLine{connected: true}, true)
	svc.convs = fakeInternalThread{}
	koc := kocWith("An", "0976451571")

	if _, err := svc.SendTemplateBatch(context.Background(), "Chào {ten}", []BatchTarget{{ID: "a", Koc: koc}}); err != nil {
		t.Fatal(err)
	}
	if kocs.marks[koc.ID] != nil || kocs.releases != 1 {
		t.Fatalf("a KOC that was not contacted is not marked: %v, %d releases", kocs.marks[koc.ID], kocs.releases)
	}
}

// With the Zalo line down the phone cannot be looked up: that is "Zalo not ready", not "no Zalo account".
func TestAZaloLineThatIsDownStopsTheBatchInsteadOfReportingNoAccount(t *testing.T) {
	svc, kocs, msgs := batchService(&fakeZaloLine{connected: false}, true)
	svc.convs = fakePhoneThread{}
	svc.zalo = NewZaloService(config.Config{}, &fakeFailingLookup{fakeZaloLine: fakeZaloLine{connected: false}}, nil)
	first := kocWith("An", "0976451571")

	rep, err := svc.SendTemplateBatch(context.Background(), "Chào {ten}", []BatchTarget{{ID: "a", Koc: first}, {ID: "b", Koc: kocWith("Bình", "0976451572")}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a:not_sent:zalo_not_ready", "b:not_sent:zalo_not_ready"}
	if !sameList(summary(rep), want) || !rep.Stop || rep.StopReason != "zalo_not_ready" {
		t.Fatalf("a dead line must not turn every KOC into \"no Zalo account\": %v stop=%v", summary(rep), rep.Stop)
	}
	if len(msgs.all) != 0 || kocs.marks[first.ID] != nil {
		t.Fatalf("nothing is stored and the claim is given back: %d messages, mark %v", len(msgs.all), kocs.marks[first.ID])
	}
}

func TestAStoreErrorForOneKocIsFailedAndTheNextOneIsStillSent(t *testing.T) {
	svc, _, _ := batchService(&fakeZaloLine{connected: true}, true)
	svc.convs = badHandleThreads{}
	bad := kocWith("Bad", "0976451571")
	bad.TiktokID = "bad"

	rep, err := svc.SendTemplateBatch(context.Background(), "Chào {ten}", []BatchTarget{{ID: "bad", Koc: bad}, {ID: "ok", Koc: kocWith("Ok", "0976451572")}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Results) != 2 || rep.Results[0].Status != BatchFailed || rep.Results[0].Reason == "" || rep.Results[1].Status != BatchSent || rep.Stop {
		t.Fatalf("one KOC fails with a reason, the next is still sent: %+v stop=%v", rep.Results, rep.Stop)
	}
}
