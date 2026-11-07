package service

import (
	"context"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

func sentToday(n int, proactive bool) *fakeOutreachMsgs {
	msgs := &fakeOutreachMsgs{}
	for i := 0; i < n; i++ {
		msgs.all = append(msgs.all, domain.Message{Proactive: proactive, SentAt: time.Now()})
	}
	return msgs
}

// The count used to live only in memory: a restart gave the whole quota back, so a server redeployed during the day
// could send far more than the limit that protects the Zalo account from being flagged as spam.
func TestTheDailyQuotaSurvivesARestart(t *testing.T) {
	svc := &OutreachService{msgs: sentToday(maxProactivePerDay, true)} // a freshly started service: nothing in memory
	check := svc.CanSendProactiveMessage(domain.NewKoc(), true)
	if check.Allowed || check.Reason != "daily_quota_reached" {
		t.Fatalf("%d messages were already sent today, the next one must be refused: %+v", maxProactivePerDay, check)
	}
}

func TestTheQuotaIsNotUsedUpUntilTheLimitIsReached(t *testing.T) {
	svc := &OutreachService{msgs: sentToday(maxProactivePerDay-1, true)}
	if check := svc.CanSendProactiveMessage(domain.NewKoc(), true); !check.Allowed {
		t.Fatalf("one message is still allowed: %+v", check)
	}
}

func TestRepliesDoNotCountAgainstTheQuota(t *testing.T) {
	svc := &OutreachService{msgs: sentToday(maxProactivePerDay+20, false)} // answers to KOCs, not started by the system
	if check := svc.CanSendProactiveMessage(domain.NewKoc(), true); !check.Allowed {
		t.Fatalf("only proactive messages count: %+v", check)
	}
}

// The quota day used to follow UTC, so it reset at 07:00 in Vietnam, in the middle of the morning working hours.
func TestTheQuotaDayStartsAtVietnamMidnight(t *testing.T) {
	beforeMidnight := time.Date(2026, 11, 7, 16, 59, 0, 0, time.UTC) // 23:59 in Vietnam
	afterMidnight := time.Date(2026, 11, 7, 17, 0, 0, 0, time.UTC)   // 00:00 on the 8th in Vietnam
	if vnDayKey(beforeMidnight) == vnDayKey(afterMidnight) {
		t.Fatal("00:00 in Vietnam must start a new quota day")
	}
	morning := time.Date(2026, 11, 8, 0, 30, 0, 0, time.UTC) // 07:30 in Vietnam, the same Vietnam day as afterMidnight
	if vnDayKey(afterMidnight) != vnDayKey(morning) {
		t.Fatal("07:00 UTC is the middle of a Vietnam morning, not a day boundary")
	}
	if got, want := startOfVNDay(morning), afterMidnight; !got.Equal(want) {
		t.Fatalf("the day of %v starts at %v, got %v", morning, want, got)
	}
}

func TestOnlyAProactiveSendIsMarkedForTheQuota(t *testing.T) {
	svc, msgs := deliveryService(&fakeZaloLine{connected: true})
	ctx := context.Background()

	if _, err := svc.DeliverMessageToKoc(ctx, domain.NewKoc(), "Lời mời", true); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DeliverMessageToKoc(ctx, domain.NewKoc(), "Trả lời", false); err != nil {
		t.Fatal(err)
	}
	if len(msgs.all) != 2 || !msgs.all[0].Proactive || msgs.all[1].Proactive {
		t.Fatalf("the invitation is proactive and the answer is not: %+v", msgs.all)
	}
}
