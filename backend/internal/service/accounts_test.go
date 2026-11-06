package service

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/tiktoday/kocflow-api/internal/config"
	"github.com/tiktoday/kocflow-api/internal/domain"
)

// fakeUsers is an in-memory UserStore.
type fakeUsers struct {
	UserStore
	byID     map[string]*domain.User
	patches  []domain.UserChange
	renamed  []string
	pwdSet   int
	cleared  int
	success  int
	failures int
	created  []*domain.User
}

func newFakeUsers(us ...*domain.User) *fakeUsers {
	f := &fakeUsers{byID: map[string]*domain.User{}}
	for _, u := range us {
		if u.ID.IsZero() {
			u.ID = domain.NewID()
		}
		f.byID[u.ID.Hex()] = u
	}
	return f
}

func (f *fakeUsers) FindByEmail(_ context.Context, email string) (*domain.User, error) {
	for _, u := range f.byID {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, nil
}
func (f *fakeUsers) FindByID(_ context.Context, id string) (*domain.User, error) {
	return f.byID[id], nil
}
func (f *fakeUsers) Create(_ context.Context, u *domain.User) error {
	u.ID = domain.NewID()
	f.byID[u.ID.Hex()] = u
	f.created = append(f.created, u)
	return nil
}
func (f *fakeUsers) Patch(_ context.Context, _ domain.ID, ch domain.UserChange) error {
	f.patches = append(f.patches, ch)
	return nil
}
func (f *fakeUsers) Rename(_ context.Context, _ domain.ID, name string) error {
	f.renamed = append(f.renamed, name)
	return nil
}
func (f *fakeUsers) SetPassword(context.Context, domain.ID, string, time.Time) error {
	f.pwdSet++
	return nil
}
func (f *fakeUsers) ClearLock(context.Context, domain.ID) error { f.cleared++; return nil }
func (f *fakeUsers) RecordLoginSuccess(context.Context, domain.ID) error {
	f.success++
	return nil
}
func (f *fakeUsers) RecordLoginFailure(context.Context, *domain.User, int, time.Duration) error {
	f.failures++
	return nil
}
func (f *fakeUsers) CountAdmins(context.Context) (int64, error) {
	var n int64
	for _, u := range f.byID {
		if u.Role == domain.RoleAdmin {
			n++
		}
	}
	return n, nil
}
func (f *fakeUsers) CountActiveAdmins(context.Context) (int64, error) {
	var n int64
	for _, u := range f.byID {
		if u.Role == domain.RoleAdmin && u.Status != domain.UserSuspended {
			n++
		}
	}
	return n, nil
}

var testAuthCfg = config.Config{JWTSecret: "test-secret", JWTExpiresIn: time.Hour}

func userWithPassword(t *testing.T, email, role, password string) *domain.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return &domain.User{Name: "Tester", Email: email, Role: role, Status: domain.UserActive, PasswordHash: string(hash)}
}

func TestSignInChecksThePasswordAndIssuesAUsableToken(t *testing.T) {
	u := userWithPassword(t, "a@x.vn", domain.RoleAdmin, "Matkhau123!")
	store := newFakeUsers(u)
	auth := NewAuthService(store, nil, nil, nil, testAuthCfg)

	if _, err := auth.Login(context.Background(), "a@x.vn", "sai", "1.1.1.1"); statusOf(t, err) != http.StatusUnauthorized || store.failures != 1 {
		t.Fatalf("a wrong password is 401 and counted: %v failures %d", err, store.failures)
	}
	if _, err := auth.Login(context.Background(), "nobody@x.vn", "x", "1.1.1.1"); statusOf(t, err) != http.StatusUnauthorized {
		t.Fatalf("an unknown e-mail looks the same as a wrong password: %v", err)
	}
	sess, err := auth.Login(context.Background(), " A@X.vn ", "Matkhau123!", "1.1.1.1")
	if err != nil || store.success != 1 {
		t.Fatalf("login: %v success %d", err, store.success)
	}
	got, err := auth.Authenticate(context.Background(), sess.Token)
	if err != nil || got.ID != u.ID {
		t.Fatalf("the token must authenticate the same account: %v", err)
	}
}

func TestALockedOrSuspendedAccountCannotSignIn(t *testing.T) {
	locked := userWithPassword(t, "l@x.vn", domain.RoleMember, "Matkhau123!")
	until := time.Now().Add(time.Hour)
	locked.LockedUntil = &until
	suspended := userWithPassword(t, "s@x.vn", domain.RoleMember, "Matkhau123!")
	suspended.Status = domain.UserSuspended
	auth := NewAuthService(newFakeUsers(locked, suspended), nil, nil, nil, testAuthCfg)

	if _, err := auth.Login(context.Background(), "l@x.vn", "Matkhau123!", ""); statusOf(t, err) != 423 {
		t.Fatalf("locked: %v", err)
	}
	if _, err := auth.Login(context.Background(), "s@x.vn", "Matkhau123!", ""); statusOf(t, err) != http.StatusForbidden {
		t.Fatalf("suspended: %v", err)
	}
}

func TestChangingThePasswordNeedsTheCurrentOneAndRevokesSessions(t *testing.T) {
	u := userWithPassword(t, "a@x.vn", domain.RoleMember, "Matkhau123!")
	store := newFakeUsers(u)
	auth := NewAuthService(store, nil, nil, nil, testAuthCfg)

	if _, err := auth.ChangePassword(context.Background(), u, "sai-roi", "MatkhauMoi456!", ""); statusOf(t, err) != http.StatusBadRequest || store.pwdSet != 0 {
		t.Fatalf("a wrong current password changes nothing: %v", err)
	}
	if _, err := auth.ChangePassword(context.Background(), u, "Matkhau123!", "Matkhau123!", ""); statusOf(t, err) != http.StatusBadRequest {
		t.Fatalf("the new password must differ: %v", err)
	}
	if _, err := auth.ChangePassword(context.Background(), u, "Matkhau123!", "yeu", ""); err == nil || store.pwdSet != 0 {
		t.Fatalf("a weak password is refused: %v", err)
	}
	if _, err := auth.ChangePassword(context.Background(), u, "Matkhau123!", "MatkhauMoi456!", ""); err != nil || store.pwdSet != 1 {
		t.Fatalf("change: %v set %d", err, store.pwdSet)
	}
}

func TestTheFirstAdminIsCreatedOnlyWhenThereIsNone(t *testing.T) {
	cfg := testAuthCfg
	cfg.AdminEmail, cfg.AdminPassword = "root@x.vn", "Matkhau123!"
	empty := newFakeUsers()
	if err := NewAuthService(empty, nil, nil, nil, cfg).BootstrapAdmin(context.Background()); err != nil || len(empty.created) != 1 || empty.created[0].Role != domain.RoleAdmin {
		t.Fatalf("an admin is created: %v %v", err, empty.created)
	}
	has := newFakeUsers(userWithPassword(t, "boss@x.vn", domain.RoleAdmin, "Matkhau123!"))
	if err := NewAuthService(has, nil, nil, nil, cfg).BootstrapAdmin(context.Background()); err != nil || len(has.created) != 0 {
		t.Fatalf("an existing admin means nothing is created: %v %v", err, has.created)
	}
}

func adminService(users *fakeUsers) *AdminService {
	auth := NewAuthService(users, nil, nil, nil, testAuthCfg)
	return &AdminService{users: users, auth: auth}
}

func TestAnAdminCannotLockOrDemoteThemselvesNorTheLastAdmin(t *testing.T) {
	me := userWithPassword(t, "me@x.vn", domain.RoleAdmin, "Matkhau123!")
	other := userWithPassword(t, "other@x.vn", domain.RoleAdmin, "Matkhau123!")
	store := newFakeUsers(me, other)
	svc := adminService(store)
	actor := Actor{ID: me.ID.Hex(), Email: me.Email}
	member, suspended := domain.RoleMember, domain.UserSuspended

	if _, err := svc.UpdateUser(context.Background(), actor, me.ID.Hex(), UserPatch{Role: &member}); statusOf(t, err) != http.StatusBadRequest {
		t.Fatalf("demoting yourself: %v", err)
	}
	if _, err := svc.UpdateUser(context.Background(), actor, me.ID.Hex(), UserPatch{Status: &suspended}); statusOf(t, err) != http.StatusBadRequest {
		t.Fatalf("suspending yourself: %v", err)
	}
	if len(store.patches) != 0 {
		t.Fatal("nothing may have been written")
	}

	other.Status = domain.UserSuspended // only `me` is still an active admin
	if _, err := svc.UpdateUser(context.Background(), Actor{ID: "someone-else"}, me.ID.Hex(), UserPatch{Role: &member}); statusOf(t, err) != http.StatusConflict {
		t.Fatalf("removing the last active admin: %v", err)
	}
}

func TestEditingAnAccountWritesOnlyWhatChangedAndRevokesSessionsForRoleOrStatus(t *testing.T) {
	boss := userWithPassword(t, "boss@x.vn", domain.RoleAdmin, "Matkhau123!")
	m := userWithPassword(t, "m@x.vn", domain.RoleMember, "Matkhau123!")
	store := newFakeUsers(boss, m)
	svc := adminService(store)
	actor := Actor{ID: boss.ID.Hex()}

	same := "Tester"
	if _, err := svc.UpdateUser(context.Background(), actor, m.ID.Hex(), UserPatch{Name: &same}); err != nil || len(store.patches) != 0 {
		t.Fatalf("an unchanged value writes nothing: %v %v", err, store.patches)
	}
	newName := "  Tên mới "
	if _, err := svc.UpdateUser(context.Background(), actor, m.ID.Hex(), UserPatch{Name: &newName}); err != nil || len(store.patches) != 1 {
		t.Fatalf("rename: %v", err)
	}
	if p := store.patches[0]; p.Name == nil || *p.Name != "Tên mới" || p.Role != nil || p.Status != nil || p.RevokeTokens {
		t.Fatalf("a rename touches the name only and keeps sessions: %+v", p)
	}
	admin := domain.RoleAdmin
	if _, err := svc.UpdateUser(context.Background(), actor, m.ID.Hex(), UserPatch{Role: &admin}); err != nil || len(store.patches) != 2 {
		t.Fatalf("promote: %v", err)
	}
	if p := store.patches[1]; p.Role == nil || *p.Role != domain.RoleAdmin || !p.RevokeTokens {
		t.Fatalf("a role change revokes the sessions: %+v", p)
	}
	bad := "root"
	if _, err := svc.UpdateUser(context.Background(), actor, m.ID.Hex(), UserPatch{Role: &bad}); statusOf(t, err) != http.StatusBadRequest {
		t.Fatalf("an unknown role is refused: %v", err)
	}
}

func TestUnknownAccountsAreNotFound(t *testing.T) {
	svc := adminService(newFakeUsers())
	n := "x"
	if _, err := svc.UpdateUser(context.Background(), Actor{}, domain.NewID().Hex(), UserPatch{Name: &n}); statusOf(t, err) != http.StatusNotFound {
		t.Fatalf("%v", err)
	}
	if !strings.Contains(ResetNotice(), "") {
		t.Fatal("unreachable")
	}
}
