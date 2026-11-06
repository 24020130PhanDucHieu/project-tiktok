package router_test

import (
	"context"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/service"
)

type noTTS struct{}

func (noTTS) SearchCreators(context.Context, string, int) ([]service.TargetCreator, error) {
	return nil, nil
}
func (noTTS) ListProducts(context.Context, string, int) (*service.ShopProductPage, error) {
	return &service.ShopProductPage{}, nil
}
func (noTTS) PreviewTarget(context.Context, service.TargetInviteInput, time.Time, int) (*service.TargetPreview, error) {
	return &service.TargetPreview{}, nil
}
func (noTTS) CreateTarget(context.Context, *service.TargetAPIPayload) (string, error) {
	panic("the route guards must stop this before it is sent")
}
func (noTTS) SearchTargets(context.Context, service.TargetListParams) (*service.TargetPage, error) {
	return &service.TargetPage{}, nil
}
func (noTTS) GetTarget(context.Context, string) (*service.TargetDetail, error) {
	return &service.TargetDetail{}, nil
}
func (noTTS) TargetLink(context.Context, string) (string, error) {
	panic("the route guards must stop this before it is sent")
}
func (noTTS) RemoveTarget(context.Context, string) error {
	panic("the route guards must stop this before it is sent")
}

type noStore struct{}

func (noStore) Insert(context.Context, *domain.TargetInvite) (*domain.TargetInvite, error) {
	panic("nothing may be stored by these tests")
}
func (noStore) SetResult(context.Context, domain.ID, string, string, string) error {
	return nil
}
func (noStore) MarkCancelled(context.Context, string) error { return nil }
func (noStore) ByTargetIDs(context.Context, []string) (map[string]domain.TargetInvite, error) {
	return map[string]domain.TargetInvite{}, nil
}
func (noStore) ByTargetID(context.Context, string) (*domain.TargetInvite, error) { return nil, nil }
func (noStore) SendingEnabled(context.Context) (bool, error)                     { return false, nil }
func (noStore) SetSendingEnabled(context.Context, bool, string) error            { return nil }
func (noStore) ShopContact(context.Context) (domain.ShopContact, error) {
	return domain.ShopContact{}, nil
}
func (noStore) SetShopContact(context.Context, domain.ShopContact, string) error { return nil }

func TestTargetCollabRoutesAreGuarded(t *testing.T) {
	app, admin, member := newApp(t)

	// everything needs a login
	for _, p := range []string{"/api/v1/target-collabs/config", "/api/v1/target-collabs", "/api/v1/target-collabs/123"} {
		if code, _ := do(t, app, "GET", p, "", ""); code != 401 {
			t.Errorf("GET %s without token must be 401, got %d", p, code)
		}
	}
	// members may read and preview
	if code, body := do(t, app, "GET", "/api/v1/target-collabs/config", member, ""); code != 200 || body["success"] != true {
		t.Errorf("member must read the config: %d %v", code, body)
	}
	// sending, link and cancel reach real creators: members are refused before the handler runs
	for _, c := range [][2]string{
		{"POST", "/api/v1/target-collabs"},
		{"POST", "/api/v1/target-collabs/123/link"},
		{"POST", "/api/v1/target-collabs/123/cancel"},
	} {
		if code, _ := do(t, app, c[0], c[1], member, `{"confirm":true}`); code != 403 {
			t.Errorf("%s %s as a member must be 403, got %d", c[0], c[1], code)
		}
		if code, _ := do(t, app, c[0], c[1], "", `{"confirm":true}`); code != 401 {
			t.Errorf("%s %s without login must be 401, got %d", c[0], c[1], code)
		}
	}
	// an admin reaches the handler, and the server switch (off in tests) still blocks sending
	code, body := do(t, app, "POST", "/api/v1/target-collabs", admin, `{"confirm":true,"requestKey":"0123456789abcdef-0123","invite":{}}`)
	if code != 409 || body["message"] == nil {
		t.Errorf("an admin must be stopped by the server switch, got %d %v", code, body)
	}
	// cancelling still needs explicit confirmation even for an admin
	if code, _ := do(t, app, "POST", "/api/v1/target-collabs/123/cancel", admin, `{"confirm":false}`); code != 400 {
		t.Errorf("cancel without confirmation must be 400, got %d", code)
	}
}

func TestTargetSwitchRouteIsAdminOnly(t *testing.T) {
	app, admin, member := newApp(t)
	if code, _ := do(t, app, "PUT", "/api/v1/target-collabs/switch", "", `{"enabled":true,"confirm":true}`); code != 401 {
		t.Errorf("anonymous must be 401, got %d", code)
	}
	if code, _ := do(t, app, "PUT", "/api/v1/target-collabs/switch", member, `{"enabled":true,"confirm":true}`); code != 403 {
		t.Errorf("a member must not flip the switch (403), got %d", code)
	}
	if code, _ := do(t, app, "PUT", "/api/v1/target-collabs/switch", admin, `{"enabled":true}`); code != 400 {
		t.Errorf("an admin turning it on without confirmation must get 400, got %d", code)
	}
	if code, body := do(t, app, "PUT", "/api/v1/target-collabs/switch", admin, `{"enabled":true,"confirm":true}`); code != 200 || body["success"] != true {
		t.Errorf("a confirmed admin request must work: %d %v", code, body)
	}
}
