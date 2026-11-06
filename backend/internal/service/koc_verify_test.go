package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/tiktoday/kocflow-api/internal/httpx"
)

type fakeVerifier struct {
	creator *TargetCreator
	err     error
	asked   []string
}

func (f *fakeVerifier) ResolveCreator(_ context.Context, username string) (*TargetCreator, error) {
	f.asked = append(f.asked, username)
	return f.creator, f.err
}

func statusOf(t *testing.T, err error) int {
	t.Helper()
	var ae *httpx.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("expected an AppError, got %v", err)
	}
	return ae.Status
}

func TestWithoutAVerifierNothingIsChecked(t *testing.T) {
	c, err := (&KocService{}).verifyCreator(context.Background(), "anyone")
	if c != nil || err != nil {
		t.Fatalf("got %v %v", c, err)
	}
}

func TestAKnownCreatorIsAccepted(t *testing.T) {
	v := &fakeVerifier{creator: &TargetCreator{OpenID: "o1", Username: "giadinhtraicay1202", Nickname: "Gia đình trái cây", Followers: 3100}}
	s := (&KocService{}).WithCreatorVerifier(v)
	c, err := s.verifyCreator(context.Background(), "giadinhtraicay1202")
	if err != nil || c == nil || c.OpenID != "o1" || len(v.asked) != 1 || v.asked[0] != "giadinhtraicay1202" {
		t.Fatalf("got %+v %v asked %v", c, err, v.asked)
	}
	p := marketplaceProfile(c, "giadinhtraicay1202")
	if p.DisplayName != "Gia đình trái cây" || p.Followers != 3100 || p.TiktokID != "giadinhtraicay1202" {
		t.Fatalf("profile %+v", p)
	}
}

func TestACreatorMissingFromTheMarketplaceIsRefused(t *testing.T) {
	s := (&KocService{}).WithCreatorVerifier(&fakeVerifier{err: ErrCreatorNotFound})
	_, err := s.verifyCreator(context.Background(), "nick_bua")
	if statusOf(t, err) != http.StatusUnprocessableEntity || !isNotListed(err) {
		t.Fatalf("a nick that TikTok Shop does not list must be refused with 422, got %v", err)
	}
}

func TestEveryFailureToVerifyRefusesTheKoc(t *testing.T) {
	cases := map[string]struct {
		err  error
		want int
	}{
		"shop not connected": {ErrTTShopNotConnected, http.StatusConflict},
		"our pause":          {&ErrRateLimited{}, http.StatusTooManyRequests},
		"tiktok rate limit":  {&TTSRejected{Code: ttsRateLimitCode}, http.StatusTooManyRequests},
		"tiktok refuses":     {&TTSRejected{Code: 1, Message: "x"}, http.StatusBadGateway},
		"network":            {errors.New("boom"), http.StatusBadGateway},
	}
	for name, c := range cases {
		s := (&KocService{}).WithCreatorVerifier(&fakeVerifier{err: c.err})
		got, err := s.verifyCreator(context.Background(), "x")
		if got != nil || err == nil {
			t.Fatalf("%s: could-not-check must never count as verified (%v %v)", name, got, err)
		}
		if st := statusOf(t, err); st != c.want {
			t.Fatalf("%s: status %d, want %d", name, st, c.want)
		}
		if name != "tiktok rate limit" && name != "our pause" && isNotListed(err) {
			t.Fatalf("%s: only a missing creator is a 422", name)
		}
	}
}
