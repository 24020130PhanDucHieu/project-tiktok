package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// emailKocs is the KOC store as the email sender uses it: find an active KOC, claim it for contact, give the claim back.
type emailKocs struct {
	kocs      map[domain.ID]*domain.Koc
	claimedAt map[domain.ID]time.Time
	claimErr  error
	released  int
}

func (f *emailKocs) FindActiveByID(_ context.Context, id string) (*domain.Koc, error) {
	oid, err := domain.ParseID(id)
	if err != nil {
		return nil, nil
	}
	return f.kocs[oid], nil
}

func (f *emailKocs) ClaimContact(_ context.Context, id domain.ID, notAfter, at time.Time) (bool, *time.Time, error) {
	if f.claimErr != nil {
		return false, nil, f.claimErr
	}
	if last, ok := f.claimedAt[id]; ok && last.After(notAfter) {
		return false, nil, nil
	}
	f.claimedAt[id] = at
	return true, nil, nil
}

func (f *emailKocs) ReleaseContact(_ context.Context, id domain.ID, _ time.Time, _ *time.Time) error {
	f.released++
	delete(f.claimedAt, id)
	return nil
}

type subjectMailer struct {
	sent []struct{ to, subject, text string }
	err  error
}

func (m *subjectMailer) Send(to, subject, text string) error {
	if m.err != nil {
		return m.err
	}
	m.sent = append(m.sent, struct{ to, subject, text string }{to, subject, text})
	return nil
}

func emailFixture(over func(*domain.Koc)) (*EmailSender, *emailKocs, *subjectMailer, *domain.Koc, *time.Time) {
	koc := &domain.Koc{ID: domain.NewID(), TiktokID: "lan.beauty", DisplayName: "Nguyễn Lan", Email: "lan@x.vn", Status: domain.KocActive}
	if over != nil {
		over(koc)
	}
	store := &emailKocs{kocs: map[domain.ID]*domain.Koc{koc.ID: koc}, claimedAt: map[domain.ID]time.Time{}}
	mail := &subjectMailer{}
	now := time.Date(2026, 11, 20, 10, 0, 0, 0, vnZone)
	return NewEmailSender(store, mail, func() time.Time { return now }), store, mail, koc, &now
}

func TestSendEmailToSendsAnEmailWithTheNameFilledIn(t *testing.T) {
	s, _, mail, koc, _ := emailFixture(nil)
	st, err := s.SendEmailTo(context.Background(), koc.ID.Hex(), "Hợp tác với {ten}", "Chào {ten}, kênh {kenh} rất hay")
	if err != nil || st != EmailSent {
		t.Fatalf("%v %v", st, err)
	}
	if len(mail.sent) != 1 || mail.sent[0].to != "lan@x.vn" || mail.sent[0].subject != "Hợp tác với Lan" || !strings.HasPrefix(mail.sent[0].text, "Chào Lan, kênh ") {
		t.Fatalf("the email is written for this KOC: %+v", mail.sent)
	}
}

func TestSendEmailToUsesADefaultSubjectWhenNoneIsGiven(t *testing.T) {
	s, _, mail, koc, _ := emailFixture(nil)
	if st, err := s.SendEmailTo(context.Background(), koc.ID.Hex(), "", "Chào"); err != nil || st != EmailSent {
		t.Fatalf("%v %v", st, err)
	}
	if mail.sent[0].subject != "Cơ hội hợp tác TikTok dành cho Nguyễn Lan" {
		t.Fatalf("the default subject: %q", mail.sent[0].subject)
	}
}

func TestSendEmailToSendsNothingToAKocThatMustNotOrCannotBeEmailed(t *testing.T) {
	cases := map[string]struct {
		over func(*domain.Koc)
		want EmailStatus
	}{
		"no address":     {func(k *domain.Koc) { k.Email = "" }, EmailNoAddress},
		"do not contact": {func(k *domain.Koc) { k.Dnc = true }, EmailBlocked},
		"already worked": {func(k *domain.Koc) { k.DaHopTac = true }, EmailBlocked},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			s, store, mail, koc, _ := emailFixture(tc.over)
			st, err := s.SendEmailTo(context.Background(), koc.ID.Hex(), "", "Chào")
			if err != nil || st != tc.want || len(mail.sent) != 0 {
				t.Fatalf("%v %v sent=%d", st, err, len(mail.sent))
			}
			if len(store.claimedAt) != 0 {
				t.Fatal("a KOC that was not emailed is not marked as contacted")
			}
		})
	}
	s, _, mail, _, _ := emailFixture(nil)
	if st, err := s.SendEmailTo(context.Background(), domain.NewID().Hex(), "", "Chào"); err != nil || st != EmailBlocked || len(mail.sent) != 0 {
		t.Fatalf("a KOC that is gone or archived: %v %v", st, err)
	}
}

func TestSendEmailToDoesNotContactTheSameKocTwiceInADay(t *testing.T) {
	s, _, mail, koc, now := emailFixture(nil)
	if st, _ := s.SendEmailTo(context.Background(), koc.ID.Hex(), "", "Chào"); st != EmailSent {
		t.Fatal("the first email goes out")
	}
	*now = now.Add(23 * time.Hour)
	if st, err := s.SendEmailTo(context.Background(), koc.ID.Hex(), "", "Chào"); err != nil || st != EmailRecent || len(mail.sent) != 1 {
		t.Fatalf("a second email within 24 hours is not sent (this is also what keeps a restart from sending twice): %v %v sent=%d", st, err, len(mail.sent))
	}
	*now = now.Add(2 * time.Hour)
	if st, _ := s.SendEmailTo(context.Background(), koc.ID.Hex(), "", "Chào"); st != EmailSent {
		t.Fatal("after a day the KOC may be emailed again")
	}
}

func TestSendEmailToGivesTheClaimBackWhenNothingCouldBeSent(t *testing.T) {
	s, store, mail, koc, _ := emailFixture(nil)
	mail.err = ErrEmailNotConfigured
	st, err := s.SendEmailTo(context.Background(), koc.ID.Hex(), "", "Chào")
	if !errors.Is(err, ErrEmailNotConfigured) || st != EmailFailed {
		t.Fatalf("%v %v", st, err)
	}
	if store.released != 1 || len(store.claimedAt) != 0 {
		t.Fatal("nothing was sent, so the KOC is not marked as contacted")
	}
}

func TestSendEmailToKeepsTheClaimWhenTheOutcomeIsUnknown(t *testing.T) {
	s, store, mail, koc, _ := emailFixture(nil)
	mail.err = errors.New("smtp: connection reset after DATA") // the server may have taken the mail
	st, err := s.SendEmailTo(context.Background(), koc.ID.Hex(), "", "Chào")
	if err == nil || st != EmailFailed {
		t.Fatalf("%v %v", st, err)
	}
	if store.released != 0 || len(store.claimedAt) != 1 {
		t.Fatal("it may have gone out: the KOC stays marked, so it is never emailed again by a retry")
	}
}

func TestSendEmailToReportsAFailingStore(t *testing.T) {
	s, store, _, koc, _ := emailFixture(nil)
	store.claimErr = errors.New("db down")
	if st, err := s.SendEmailTo(context.Background(), koc.ID.Hex(), "", "Chào"); err == nil || st != EmailFailed {
		t.Fatalf("%v %v", st, err)
	}
}
