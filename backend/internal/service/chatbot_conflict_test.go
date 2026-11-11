package service

import (
	"context"
	"testing"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// conflictingKocs hands out a fresh copy of the KOC on every read, like the database does, and refuses the first
// `conflicts` saves as if somebody else had saved the KOC in between.
type conflictingKocs struct {
	OutreachKocs
	stored    domain.Koc
	conflicts int
	reads     int
	saves     int
	lastSaved *domain.Koc
}

func (f *conflictingKocs) FindActiveByContact(context.Context, string, ...string) (*domain.Koc, error) {
	f.reads++
	k := f.stored
	return &k, nil
}

func (f *conflictingKocs) Save(_ context.Context, k *domain.Koc) error {
	f.saves++
	if f.saves <= f.conflicts {
		return domain.Conflict("changed elsewhere")
	}
	f.lastSaved = k
	return nil
}

func inboundOn(kocs *conflictingKocs) (*OutreachService, *domain.Conversation) {
	svc := &OutreachService{kocs: kocs, convs: &fakeOutreachConvs{}, msgs: &fakeOutreachMsgs{}}
	return svc, &domain.Conversation{ID: domain.NewID(), ContactHandle: "@linh"}
}

func contactedKoc() domain.Koc {
	k := domain.NewKoc()
	k.TiktokID, k.DisplayName, k.Stage, k.Status = "linh", "Linh", domain.StageContacted, domain.KocActive
	return *k
}

// The chatbot is the one writer nobody can ask to try again: a KOC's message that lost the race for the save must not
// be dropped.
func TestAnInboundMessageIsAppliedAgainWhenTheKocWasSavedMeanwhile(t *testing.T) {
	kocs := &conflictingKocs{stored: contactedKoc(), conflicts: 1}
	svc, conv := inboundOn(kocs)

	res, err := svc.ProcessInboundWithChatbot(context.Background(), conv, "mình đồng ý hợp tác nha", true)
	if err != nil {
		t.Fatalf("the second attempt succeeds, so the message is processed: %v", err)
	}
	if kocs.reads != 2 || kocs.saves != 2 {
		t.Fatalf("the KOC is read again and the message applied again: reads %d saves %d", kocs.reads, kocs.saves)
	}
	if kocs.lastSaved == nil || kocs.lastSaved.Stage != domain.StageReplied {
		t.Fatalf("the agreeing message moves the KOC to REPLIED: %+v", kocs.lastSaved)
	}
	if res.BotMessage == nil {
		t.Fatal("the bot still answers, once")
	}
}

func TestAnInboundMessageGivesUpAfterOneRetryInsteadOfLoopingForever(t *testing.T) {
	kocs := &conflictingKocs{stored: contactedKoc(), conflicts: 100}
	svc, conv := inboundOn(kocs)

	if _, err := svc.ProcessInboundWithChatbot(context.Background(), conv, "mình đồng ý hợp tác nha", true); err == nil {
		t.Fatal("a KOC that keeps changing must end in an error the caller can see")
	}
	if kocs.reads != 2 {
		t.Fatalf("one retry only, got %d reads", kocs.reads)
	}
}

func TestANormalInboundMessageReadsAndSavesOnce(t *testing.T) {
	kocs := &conflictingKocs{stored: contactedKoc()}
	svc, conv := inboundOn(kocs)

	if _, err := svc.ProcessInboundWithChatbot(context.Background(), conv, "mình đồng ý hợp tác nha", true); err != nil {
		t.Fatal(err)
	}
	if kocs.reads != 1 || kocs.saves != 1 {
		t.Fatalf("no conflict, no retry: reads %d saves %d", kocs.reads, kocs.saves)
	}
}

// The bot's reply used to be written by saving the whole conversation, undoing the unread count a KOC's next message
// could raise in the same instant. Only the preview is written now.
func TestTheBotReplyWritesOnlyThePreviewOfTheConversation(t *testing.T) {
	kocs := &conflictingKocs{stored: contactedKoc()}
	convs := &fakeOutreachConvs{}
	svc := &OutreachService{kocs: kocs, convs: convs, msgs: &fakeOutreachMsgs{}}
	conv := &domain.Conversation{ID: domain.NewID(), ContactHandle: "@linh"}

	res, err := svc.ProcessInboundWithChatbot(context.Background(), conv, "mình đồng ý hợp tác nha", true)
	if err != nil || res.BotMessage == nil {
		t.Fatalf("%v %+v", err, res)
	}
	if convs.wholeSaves != 0 {
		t.Fatalf("the whole conversation must not be replaced, %d saves", convs.wholeSaves)
	}
	if len(convs.saved) != 1 || convs.saved[0] != res.BotMessage.Text {
		t.Fatalf("the preview shows the reply: %v", convs.saved)
	}
}
