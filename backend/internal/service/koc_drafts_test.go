package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/config"
	"github.com/tiktoday/kocflow-api/internal/domain"
)

// stageKocs is the KOC store of the stage tests: it hands back the KOC it was given and counts the saves.
type stageKocs struct {
	KocDirectory
	saves int
}

func (s *stageKocs) Save(context.Context, *domain.Koc) error { s.saves++; return nil }

// stageSetup builds the use case with a Zalo line that records everything pushed to it, and a thread store that
// would record a stored message: the stage buttons must reach neither.
func stageSetup(gate AutomationGate) (*KocService, *fakeZaloLine, *fakeOutreachMsgs) {
	line := &fakeZaloLine{connected: true}
	msgs := &fakeOutreachMsgs{}
	outreach := &OutreachService{kocs: &fakeOutreachKocs{}, convs: fakeKnownThread{}, msgs: msgs,
		zalo: NewZaloService(config.Config{}, line, nil), gate: gate, tracker: fakeTracker{}}
	return &KocService{kocs: &stageKocs{}, outreach: outreach}, line, msgs
}

type fakeTracker struct{}

func (fakeTracker) QueryTracking(_ context.Context, code, _ string) (*JTTracking, error) {
	return &JTTracking{BillCode: code, StatusText: "Đang vận chuyển"}, nil
}

func linh() *domain.Koc {
	k := domain.NewKoc()
	k.TiktokID, k.DisplayName, k.Phone, k.Stage, k.Status = "linh.review", "Linh", "0327637213", domain.StageAgreed, domain.KocActive
	k.ProductName = "Khăn lạnh Cooltech"
	return k
}

// Saving a waybill used to push "Gửi mẫu thành công ..." to the KOC's Zalo in the same click. Now the click saves the
// data and hands back a message for the booker to read, change and send.
func TestSavingTheWaybillPreparesTheMessageAndSendsNothing(t *testing.T) {
	for name, gate := range map[string]AutomationGate{"switch off": switchState(false), "switch on": switchState(true)} {
		svc, line, msgs := stageSetup(gate)
		code, addr, carrier := "841328496973", "12 ngõ 5 đường Láng, Hà Nội", "J&T Express"
		res, err := svc.ApplyStage(context.Background(), linh(), StageUpdate{Action: "ship_sample", TrackingCode: &code, ShippingAddress: &addr, ShippingCarrier: &carrier})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(line.sent) != 0 || len(msgs.all) != 0 {
			t.Fatalf("%s: nothing may leave the app or be stored as a message: zalo %v stored %+v", name, line.sent, msgs.all)
		}
		if res.Koc.Stage != domain.StageShipping || res.Koc.TrackingCode != code {
			t.Fatalf("%s: the data is still saved: %+v", name, res.Koc)
		}
		if !strings.Contains(res.Draft, code) || !strings.Contains(res.Draft, addr) || !strings.Contains(res.Draft, "Linh") {
			t.Fatalf("%s: the draft names the KOC, the address and the waybill: %q", name, res.Draft)
		}
	}
}

func TestEveryStageButtonThatHasAMessageOnlyPreparesIt(t *testing.T) {
	cases := []struct {
		name   string
		update StageUpdate
		koc    func() *domain.Koc
	}{
		{"mark_delivered", StageUpdate{Action: "mark_delivered"}, func() *domain.Koc { k := linh(); k.Stage = domain.StageShipping; return k }},
		{"send_invite", StageUpdate{Action: "send_invite"}, func() *domain.Koc { k := linh(); k.Stage = domain.StageQualified; return k }},
		{"remind_step", StageUpdate{Action: "remind_step"}, func() *domain.Koc { k := linh(); k.Stage = domain.StageContacted; return k }},
		{"remind_video", StageUpdate{Action: "remind_video"}, func() *domain.Koc { k := linh(); k.Stage = domain.StageDelivered; return k }},
	}
	for _, c := range cases {
		svc, line, msgs := stageSetup(switchState(true)) // even with the switch on: these are the booker's own clicks
		res, err := svc.ApplyStage(context.Background(), c.koc(), c.update)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if len(line.sent) != 0 || len(msgs.all) != 0 {
			t.Fatalf("%s sent or stored a message: zalo %v stored %+v", c.name, line.sent, msgs.all)
		}
		if strings.TrimSpace(res.Draft) == "" {
			t.Fatalf("%s must hand back a message to put into the chat", c.name)
		}
	}
}

func TestAStageButtonWithoutAMessageHasNoDraft(t *testing.T) {
	svc, _, _ := stageSetup(switchState(false))
	res, err := svc.ApplyStage(context.Background(), linh(), StageUpdate{Action: "toggle_dnc", Dnc: boolPtr(true)})
	if err != nil || res.Draft != "" {
		t.Fatalf("%v %q", err, res.Draft)
	}
}

func boolPtr(b bool) *bool { return &b }

// The messages are read by creators on Zalo: no [tags], no quotation marks, no explanations in brackets.
func TestTheMessagesReadLikeAPersonWroteThem(t *testing.T) {
	k := linh()
	k.TrackingCode, k.ShippingCarrier, k.ShippingAddress, k.Campaign = "841328496973", "J&T Express", "12 Láng, Hà Nội", "Hè 2026"
	k.CastFee = 300000
	texts := map[string]string{
		"invite":       BuildInviteMessage(k, ""),
		"invite+brief": BuildInviteMessage(k, "Quay 1 video review ngắn"),
		"shipped":      ShipmentDraft(k),
		"delivered":    DeliveredDraft(k),
		"selfship":     SelfShipQuestion(k),
		"return":       ReturnReminderDraft(k),
	}
	for step := 1; step <= 2; step++ {
		texts["followup"+string(rune('0'+step))] = FollowUpDraft(k, step)
	}
	for step := 1; step <= 3; step++ {
		texts["video"+string(rune('0'+step))] = VideoReminderDraft(k, step)
	}
	k.ShippingCarrier = "Tự vận chuyển"
	texts["shipped self"] = ShipmentDraft(k)
	for name, text := range texts {
		if strings.TrimSpace(text) == "" {
			t.Errorf("%s: empty", name)
		}
		if i := strings.IndexAny(text, "\"“”'‘’()[]{}«»"); i >= 0 {
			t.Errorf("%s has a mark that explains or quotes (%q): %s", name, text[i:i+1], text)
		}
	}
}

func TestTheSelfShippedParcelIsAskedAboutWithoutAWaybill(t *testing.T) {
	k := linh()
	k.ShippingCarrier = "Tự vận chuyển"
	k.ShippingAddress = "12 Láng, Hà Nội"
	text := ShipmentDraft(k)
	if strings.Contains(text, "mã vận đơn") || !strings.Contains(text, "12 Láng, Hà Nội") {
		t.Fatalf("no waybill to give, but the address is confirmed: %s", text)
	}
}

// ---- the switch ----

type switchState bool

func (s switchState) Enabled(context.Context) (bool, error) { return bool(s), nil }

type brokenSwitch struct{}

func (brokenSwitch) Enabled(context.Context) (bool, error) { return true, context.DeadlineExceeded }

func TestTheSwitchIsOffUnlessItWasTurnedOn(t *testing.T) {
	ctx := context.Background()
	for name, svc := range map[string]*OutreachService{
		"no switch wired":   {},
		"switch off":        {gate: switchState(false)},
		"switch unreadable": {gate: brokenSwitch{}},
	} {
		if svc.automationOn(ctx) {
			t.Errorf("%s: automation must stay off", name)
		}
	}
	if !(&OutreachService{gate: switchState(true)}).automationOn(ctx) {
		t.Error("switched on")
	}
}

// With the switch off the sweep still keeps the data fresh (a waybill's status) but writes to nobody.
func TestTheSweepSendsNothingWhileTheSwitchIsOff(t *testing.T) {
	for _, on := range []bool{false, true} {
		line := &fakeZaloLine{connected: true}
		koc := sweepKoc("linh")
		kocs := &fakeSweepKocs{chase: []*domain.Koc{koc}, notFit: []*domain.Koc{sweepKoc("nf")}}
		msgs := &fakeOutreachMsgs{}
		svc := &OutreachService{kocs: kocs, convs: fakeKnownThread{}, msgs: msgs, zalo: NewZaloService(config.Config{}, line, nil), gate: switchState(on)}

		sum, err := svc.RunStage2Sweep(context.Background(), true)
		if err != nil {
			t.Fatal(err)
		}
		sent := len(line.sent) + len(msgs.all)
		if !on && (sent != 0 || sum.EmailReminded != 0 || sum.ReturnReminded != 0 || koc.ReminderStep != 0) {
			t.Fatalf("switch off: no message and no reminder step burnt: sent %d, %+v, step %d", sent, sum, koc.ReminderStep)
		}
		if on && (sum.EmailReminded != 1 || sum.ReturnReminded != 1 || sent == 0) {
			t.Fatalf("switch on: the sweep works as before: %+v sent %d", sum, sent)
		}
	}
}

func TestTheChatbotStaysSilentWhileTheSwitchIsOffButStillLearns(t *testing.T) {
	for _, on := range []bool{false, true} {
		koc := contactedKoc()
		kocs := &conflictingKocs{stored: koc}
		svc := &OutreachService{kocs: kocs, convs: &fakeOutreachConvs{}, msgs: &fakeOutreachMsgs{}, gate: switchState(on)}
		conv := &domain.Conversation{ID: domain.NewID(), ContactHandle: "@linh"}

		res, err := svc.ProcessInboundWithChatbot(context.Background(), conv, "mình đồng ý hợp tác nha", true)
		if err != nil {
			t.Fatal(err)
		}
		if kocs.lastSaved == nil || kocs.lastSaved.Stage != domain.StageReplied {
			t.Fatalf("on=%v: the KOC's progress is recorded either way: %+v", on, kocs.lastSaved)
		}
		if (res.BotMessage != nil) != on {
			t.Fatalf("on=%v: the bot answers only with the switch on, got %+v", on, res.BotMessage)
		}
	}
}

func TestTheBulkInviteIsRefusedWhileTheSwitchIsOff(t *testing.T) {
	svc, line, _ := stageSetup(switchState(false))
	svc.kocs = &bulkKocs{stageKocs: &stageKocs{}, list: []*domain.Koc{linh()}}
	if _, err := svc.InviteAllQualified(context.Background(), ""); err == nil {
		t.Fatal("the bulk invitation sends to many creators on its own: it needs the switch")
	}
	if len(line.sent) != 0 {
		t.Fatalf("%v", line.sent)
	}
}

type bulkKocs struct {
	*stageKocs
	list []*domain.Koc
}

func (b *bulkKocs) FindQualifiedNotCollaborated(context.Context) ([]*domain.Koc, error) {
	return b.list, nil
}

var _ = time.Now
