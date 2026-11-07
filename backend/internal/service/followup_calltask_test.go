package service

import (
	"context"
	"strings"
	"testing"

	"github.com/tiktoday/kocflow-api/internal/config"
	"github.com/tiktoday/kocflow-api/internal/domain"
)

func TestTheCallTaskForTheBookerNeverReachesTheKoc(t *testing.T) {
	line := &fakeZaloLine{connected: true} // if anything were pushed to Zalo it would be recorded here
	msgs := &fakeOutreachMsgs{}
	koc := domain.NewKoc()
	koc.TiktokID, koc.DisplayName, koc.Phone, koc.Stage = "linh.review", "Linh", "0327637213", domain.StageNoReply
	svc := &OutreachService{kocs: &fakeOutreachKocs{koc: koc}, convs: fakeKnownThread{}, msgs: msgs,
		zalo: NewZaloService(config.Config{}, line, nil)}

	updated, err := svc.AdvanceFollowUpStep(context.Background(), koc, 3)
	if err != nil {
		t.Fatal(err)
	}

	if len(line.sent) != 0 {
		t.Fatalf("an instruction for the booker was sent to the KOC: %v", line.sent)
	}
	if len(msgs.all) != 1 || msgs.all[0].Kind != "system" || !strings.Contains(msgs.all[0].Text, "Task gọi điện") {
		t.Fatalf("the task must be noted once in the thread as a system note: %+v", msgs.all)
	}
	if !updated.CallTaskPending || updated.ReminderStep != 3 || updated.Stage != domain.StageNoReply {
		t.Fatalf("the KOC still waits for the booker's call: %+v", updated)
	}
}

func TestTheOtherFollowUpStepsStillReachTheKoc(t *testing.T) {
	line := &fakeZaloLine{connected: true}
	koc := domain.NewKoc()
	koc.TiktokID, koc.DisplayName = "linh.review", "Linh"
	svc := &OutreachService{kocs: &fakeOutreachKocs{koc: koc}, convs: fakeKnownThread{}, msgs: &fakeOutreachMsgs{},
		zalo: NewZaloService(config.Config{}, line, nil)}

	if _, err := svc.AdvanceFollowUpStep(context.Background(), koc, 2); err != nil {
		t.Fatal(err)
	}
	if len(line.sent) != 1 {
		t.Fatalf("the reminder of step 2 is a real message to the KOC: %v", line.sent)
	}
}
