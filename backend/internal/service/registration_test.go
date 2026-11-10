package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/tiktoday/kocflow-api/internal/config"
	"github.com/tiktoday/kocflow-api/internal/domain"
)

// fakeCaptcha answers like Cloudflare Turnstile: valid for the token "human", or an outage error.
type fakeCaptcha struct {
	calls int
	err   error
}

func (f *fakeCaptcha) Verify(_ context.Context, token, _ string) (bool, error) {
	f.calls++
	return token == "human", f.err
}

var openRegistration = config.Config{JWTSecret: "test-secret", AllowRegistration: true}

func signUp(name, email, password string) RegistrationRequest {
	return RegistrationRequest{Name: name, Email: email, Password: password, CaptchaToken: "human", IP: "1.2.3.4"}
}

func TestASignUpWaitsForAnAdminAndGivesNoSession(t *testing.T) {
	users := newFakeUsers()
	auth := NewAuthService(users, nil, nil, nil, openRegistration).WithCaptcha(&fakeCaptcha{})

	if err := auth.Register(context.Background(), signUp("Lan Anh", " Lan@X.vn ", "Matkhau123")); err != nil {
		t.Fatal(err)
	}
	if len(users.created) != 1 {
		t.Fatalf("one account must be created, got %d", len(users.created))
	}
	u := users.created[0]
	if u.Role != domain.RoleMember || u.Status != domain.UserPending || u.Email != "lan@x.vn" {
		t.Fatalf("a sign-up is a pending member with a normalised email: %+v", u)
	}
	// a pending account cannot sign in, even with the right password, and is told why
	_, err := auth.Login(context.Background(), "lan@x.vn", "Matkhau123", "")
	var f *domain.Fault
	if !errors.As(err, &f) || f.Kind != domain.FaultForbidden || f.Message != pendingLogin {
		t.Fatalf("pending sign-in must be refused with the waiting message, got %v", err)
	}
}

func TestASignUpWithAKnownEmailLooksTheSameAndChangesNothing(t *testing.T) {
	existing := userWithPassword(t, "known@x.vn", domain.RoleAdmin, "Matkhau123")
	users := newFakeUsers(existing)
	auth := NewAuthService(users, nil, nil, nil, openRegistration).WithCaptcha(&fakeCaptcha{})

	// the same answer as a new address, so nobody can probe which emails have an account
	if err := auth.Register(context.Background(), signUp("X", "known@x.vn", "Matkhau123")); err != nil {
		t.Fatalf("a known email must get the same answer as a new one, got %v", err)
	}
	if len(users.created) != 0 || len(users.patches) != 0 || existing.Role != domain.RoleAdmin {
		t.Fatal("the existing account must be left untouched")
	}
}

func TestASignUpNeedsAHumanAndTheHoneypotEmpty(t *testing.T) {
	ctx := context.Background()

	users := newFakeUsers()
	captcha := &fakeCaptcha{}
	auth := NewAuthService(users, nil, nil, nil, openRegistration).WithCaptcha(captcha)
	bad := signUp("Bot", "bot@x.vn", "Matkhau123")
	bad.CaptchaToken = "forged"
	if err := auth.Register(ctx, bad); statusOf(t, err) != http.StatusBadRequest {
		t.Fatalf("a failed captcha is refused: %v", err)
	}

	trap := signUp("Bot", "bot2@x.vn", "Matkhau123")
	trap.Website = "http://spam.example" // a field people never see, that bots fill in
	if err := auth.Register(ctx, trap); err != nil {
		t.Fatalf("a bot gets the normal answer so it learns nothing: %v", err)
	}
	if len(users.created) != 0 {
		t.Fatalf("no account may be created for a bot, got %d", len(users.created))
	}

	down := NewAuthService(newFakeUsers(), nil, nil, nil, openRegistration).WithCaptcha(&fakeCaptcha{err: errors.New("timeout")})
	if err := down.Register(ctx, signUp("A", "a@x.vn", "Matkhau123")); statusOf(t, err) != http.StatusBadGateway {
		t.Fatalf("a captcha outage is reported as unavailable, not as a bad token: %v", err)
	}
}

func TestASignUpIsRefusedWhileRegistrationIsClosedAndChecksThePassword(t *testing.T) {
	closed := NewAuthService(newFakeUsers(), nil, nil, nil, testAuthCfg).WithCaptcha(&fakeCaptcha{})
	if err := closed.Register(context.Background(), signUp("A", "a@x.vn", "Matkhau123")); statusOf(t, err) != http.StatusForbidden {
		t.Fatalf("closed registration: %v", err)
	}
	auth := NewAuthService(newFakeUsers(), nil, nil, nil, openRegistration).WithCaptcha(&fakeCaptcha{})
	if err := auth.Register(context.Background(), signUp("A", "a@x.vn", "chiluchu")); statusOf(t, err) != http.StatusBadRequest {
		t.Fatalf("a weak password is refused: %v", err)
	}
}

func TestAnAdminApprovesAPendingSignUp(t *testing.T) {
	boss := userWithPassword(t, "boss@x.vn", domain.RoleAdmin, "Matkhau123")
	waiting := userWithPassword(t, "new@x.vn", domain.RoleMember, "Matkhau123")
	waiting.Status = domain.UserPending
	store := newFakeUsers(boss, waiting)
	active := domain.UserActive

	if _, err := adminService(store).UpdateUser(context.Background(), Actor{ID: boss.ID.Hex()}, waiting.ID.Hex(), UserPatch{Status: &active}); err != nil {
		t.Fatal(err)
	}
	if len(store.patches) != 1 || store.patches[0].Status == nil || *store.patches[0].Status != domain.UserActive {
		t.Fatalf("approving writes the active status: %+v", store.patches)
	}
	pending := domain.UserPending
	if _, err := adminService(store).UpdateUser(context.Background(), Actor{ID: boss.ID.Hex()}, boss.ID.Hex(), UserPatch{Status: &pending}); err == nil {
		t.Fatal("an admin cannot put an account back to pending: it is only for new sign-ups")
	}
}

func TestRegistrationOptionsTellTheLoginPageWhatToShow(t *testing.T) {
	cfg := openRegistration
	cfg.TurnstileSiteKey = "0x4AAAA-site"
	got := NewAuthService(newFakeUsers(), nil, nil, nil, cfg).RegistrationOptions()
	if !got.AllowRegistration || got.CaptchaSiteKey != "0x4AAAA-site" {
		t.Fatalf("%+v", got)
	}
}
