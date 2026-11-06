package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"runtime"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/metrics"
	"github.com/tiktoday/kocflow-api/internal/repository"
)

// AdminService implements account management and system monitoring for administrators.
type AdminService struct {
	users   *repository.UserRepository
	auth    *AuthService
	audit   *AuditService
	dbInfo  DatabaseInspector
	bridge  PersonalZalo
	mailer  *SMTPMailer
	metrics *metrics.Collector
	jobs    *metrics.Jobs
	version string
}

// DatabaseInspector reports the database statistics for the system page (implemented by the repository package).
type DatabaseInspector interface {
	Inspect(ctx context.Context) domain.DatabaseInfo
}

// NewAdminService builds an AdminService.
func NewAdminService(users *repository.UserRepository, auth *AuthService, audit *AuditService, dbInfo DatabaseInspector,
	bridge PersonalZalo, mailer *SMTPMailer, m *metrics.Collector, jobs *metrics.Jobs, version string) *AdminService {
	return &AdminService{users: users, auth: auth, audit: audit, dbInfo: dbInfo, bridge: bridge, mailer: mailer, metrics: m, jobs: jobs, version: version}
}

// Actor identifies who performs an admin action.
type Actor struct{ ID, Email, IP string }

// UserPage is a page of users.
type UserPage struct {
	Items      []domain.User `json:"items"`
	Pagination Pagination    `json:"pagination"`
}

// ListUsers returns users for the admin table.
func (s *AdminService) ListUsers(ctx context.Context, q, role, status string, page, limit int64) (*UserPage, error) {
	page, limit = max(page, 1), min(max(limit, 1), 100)
	items, total, err := s.users.List(ctx, repository.UserFilter{Q: q, Role: role, Status: status, Skip: (page - 1) * limit, Limit: limit})
	if err != nil {
		return nil, err
	}
	return &UserPage{Items: items, Pagination: Pagination{Page: page, Limit: limit, Total: total, Pages: (total + limit - 1) / limit}}, nil
}

func randomPassword() (string, error) {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	// guarantees letters + digits for the policy
	return "Kf1" + base64.RawURLEncoding.EncodeToString(b), nil
}

// CreatedUser is the result of an admin creating an account.
type CreatedUser struct {
	User              *domain.User `json:"user"`
	InviteSent        bool         `json:"inviteSent"`
	TemporaryPassword string       `json:"temporaryPassword,omitempty"`
}

// CreateUser creates an account. Without an explicit password the user gets an invitation e-mail with a
// set-password link; if e-mail is unavailable the one-time temporary password is returned to the admin.
func (s *AdminService) CreateUser(ctx context.Context, a Actor, name, email, role, password string) (*CreatedUser, error) {
	if role != domain.RoleAdmin && role != domain.RoleMember {
		return nil, httpx.NewError(400, "Vai trò không hợp lệ")
	}
	generated := password == ""
	if generated {
		var err error
		if password, err = randomPassword(); err != nil {
			return nil, err
		}
	}
	u, err := s.auth.CreateUser(ctx, name, email, password, role)
	if err != nil {
		return nil, err
	}
	s.audit.Log(ctx, a.ID, a.Email, "admin.user_created", "user", u.ID.Hex(), u.Email+" ("+role+")", a.IP)
	res := &CreatedUser{User: u}
	if generated {
		if err := s.auth.IssueResetCode(ctx, u, true); err == nil {
			res.InviteSent = true
		} else {
			res.TemporaryPassword = password
		}
	}
	return res, nil
}

// UserPatch is a partial account update.
type UserPatch struct {
	Name   *string
	Role   *string
	Status *string
}

func (s *AdminService) activeAdmins(ctx context.Context) (int64, error) {
	return s.users.Count(ctx, bson.M{"role": domain.RoleAdmin, "status": bson.M{"$ne": domain.UserSuspended}})
}

// UpdateUser edits name/role/status. Admins cannot demote/suspend themselves or remove the last admin.
func (s *AdminService) UpdateUser(ctx context.Context, a Actor, id string, p UserPatch) (*domain.User, error) {
	u, err := s.users.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, httpx.NewError(404, "Không tìm thấy tài khoản")
	}
	set, detail := bson.M{}, []string{}
	revoke := false
	if p.Name != nil && strings.TrimSpace(*p.Name) != u.Name {
		set["name"] = strings.TrimSpace(*p.Name)
		detail = append(detail, "name")
	}
	if p.Role != nil && *p.Role != u.Role {
		if *p.Role != domain.RoleAdmin && *p.Role != domain.RoleMember {
			return nil, httpx.NewError(400, "Vai trò không hợp lệ")
		}
		if u.ID.Hex() == a.ID {
			return nil, httpx.NewError(400, "Không thể tự đổi vai trò của chính mình")
		}
		set["role"], revoke = *p.Role, true
		detail = append(detail, "role: "+u.Role+" → "+*p.Role)
	}
	if p.Status != nil && *p.Status != u.Status && !(u.Status == "" && *p.Status == domain.UserActive) {
		if *p.Status != domain.UserActive && *p.Status != domain.UserSuspended {
			return nil, httpx.NewError(400, "Trạng thái không hợp lệ")
		}
		if u.ID.Hex() == a.ID {
			return nil, httpx.NewError(400, "Không thể tự khóa tài khoản của chính mình")
		}
		set["status"], revoke = *p.Status, true
		detail = append(detail, "status → "+*p.Status)
	}
	if len(set) == 0 {
		return u, nil
	}
	// never leave the system without an active admin
	losesAdmin := u.Role == domain.RoleAdmin && ((set["role"] != nil && set["role"] != domain.RoleAdmin) || set["status"] == domain.UserSuspended)
	if losesAdmin {
		if n, err := s.activeAdmins(ctx); err != nil {
			return nil, err
		} else if n <= 1 {
			return nil, httpx.NewError(409, "Phải còn ít nhất một quản trị viên đang hoạt động")
		}
	}
	inc := bson.M(nil)
	if revoke {
		inc = bson.M{"tokenVersion": 1}
	}
	if err := s.users.Update(ctx, u.ID, set, inc); err != nil {
		return nil, err
	}
	s.auth.Invalidate(u.ID.Hex())
	s.audit.Log(ctx, a.ID, a.Email, "admin.user_updated", "user", u.ID.Hex(), strings.Join(detail, "; "), a.IP)
	return s.users.FindByID(ctx, id)
}

// SendReset e-mails a password-reset link to a user.
func (s *AdminService) SendReset(ctx context.Context, a Actor, id string) error {
	u, err := s.users.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if u == nil {
		return httpx.NewError(404, "Không tìm thấy tài khoản")
	}
	if err := s.auth.IssueResetCode(ctx, u, false); err != nil {
		if err == ErrEmailNotConfigured {
			return httpx.NewError(422, "Chưa cấu hình SMTP nên không thể gửi email đặt lại mật khẩu")
		}
		return err
	}
	s.audit.Log(ctx, a.ID, a.Email, "admin.reset_sent", "user", u.ID.Hex(), u.Email, a.IP)
	return nil
}

// DeleteUser removes an account (never yourself nor the last active admin).
func (s *AdminService) DeleteUser(ctx context.Context, a Actor, id string) error {
	u, err := s.users.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if u == nil {
		return httpx.NewError(404, "Không tìm thấy tài khoản")
	}
	if u.ID.Hex() == a.ID {
		return httpx.NewError(400, "Không thể tự xóa tài khoản của chính mình")
	}
	if u.Role == domain.RoleAdmin {
		if n, err := s.activeAdmins(ctx); err != nil {
			return err
		} else if n <= 1 {
			return httpx.NewError(409, "Phải còn ít nhất một quản trị viên đang hoạt động")
		}
	}
	if err := s.users.Delete(ctx, u.ID); err != nil {
		return err
	}
	s.auth.Invalidate(u.ID.Hex())
	s.audit.Log(ctx, a.ID, a.Email, "admin.user_deleted", "user", u.ID.Hex(), u.Email, a.IP)
	return nil
}

// SystemInfo is the monitoring payload.
type SystemInfo struct {
	Version  string              `json:"version"`
	GoVer    string              `json:"goVersion"`
	Host     metrics.Host        `json:"host"`
	Runtime  map[string]any      `json:"runtime"`
	HTTP     metrics.Snapshot    `json:"http"`
	Database domain.DatabaseInfo `json:"database"`
	Jobs     []metrics.JobRun    `json:"jobs"`
	Services map[string]any      `json:"services"`
	Users    map[string]int64    `json:"users"`
}

// System gathers host, runtime, database, background jobs, integrations and account counters.
func (s *AdminService) System(ctx context.Context) (*SystemInfo, error) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	zalo := s.bridge.Status(ctx)
	users := map[string]int64{}
	users["total"], _ = s.users.Count(ctx, bson.M{})
	users["admins"], _ = s.users.Count(ctx, bson.M{"role": domain.RoleAdmin})
	users["suspended"], _ = s.users.Count(ctx, bson.M{"status": domain.UserSuspended})
	users["activeLast7d"], _ = s.users.Count(ctx, bson.M{"lastLoginAt": bson.M{"$gte": time.Now().AddDate(0, 0, -7)}})

	jobs := s.jobs.List()
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].Name < jobs[j].Name })

	return &SystemInfo{
		Version: s.version, GoVer: runtime.Version(), Host: metrics.ReadHost(),
		Runtime: map[string]any{
			"goroutines": runtime.NumGoroutine(), "cpus": runtime.NumCPU(),
			"heapAllocMB": float64(mem.HeapAlloc) / 1048576, "sysMB": float64(mem.Sys) / 1048576, "gcRuns": mem.NumGC,
			"lastGcPauseMs": float64(mem.PauseNs[(mem.NumGC+255)%256]) / 1e6,
		},
		HTTP: s.metrics.Snapshot(), Database: s.dbInfo.Inspect(ctx), Jobs: jobs,
		Services: map[string]any{
			"smtpConfigured": s.mailer.Configured(),
			"zaloPersonal":   map[string]any{"state": zalo.State, "connected": zalo.Connected, "bridgeOnline": zalo.BridgeOnline, "error": zalo.Error},
		},
		Users: users,
	}, nil
}
