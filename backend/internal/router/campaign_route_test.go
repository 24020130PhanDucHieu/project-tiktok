package router_test

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// memCampaigns keeps campaigns and pictures in memory.
type memCampaigns struct {
	mu     sync.Mutex
	images map[domain.ID]*domain.CampaignImage
	steps  []domain.CampaignStep
}

func newMemCampaigns() *memCampaigns {
	return &memCampaigns{images: map[domain.ID]*domain.CampaignImage{}}
}

func (m *memCampaigns) List(context.Context) ([]domain.Campaign, error) {
	return []domain.Campaign{{ID: domain.NewID(), Name: "A", Steps: m.steps}}, nil
}
func (m *memCampaigns) SaveSteps(_ context.Context, _ string, steps []domain.CampaignStep, _ time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.steps = steps
	return nil
}
func (m *memCampaigns) AddImage(_ context.Context, img *domain.CampaignImage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	img.ID = domain.NewID()
	m.images[img.ID] = img
	return nil
}
func (m *memCampaigns) Image(_ context.Context, id string) (*domain.CampaignImage, error) {
	oid, err := domain.ParseID(id)
	if err != nil || m.images[oid] == nil {
		return nil, domain.NotFound("Không tìm thấy ảnh.")
	}
	return m.images[oid], nil
}
func (m *memCampaigns) ImagesExist(_ context.Context, ids []domain.ID) (bool, error) {
	for _, id := range ids {
		if m.images[id] == nil {
			return false, nil
		}
	}
	return true, nil
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func upload(t *testing.T, app *fiber.App, token, filename string, data []byte) (int, string) {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, _ := w.CreateFormFile("file", filename)
	_, _ = part.Write(data)
	_ = w.Close()
	req := httptest.NewRequest("POST", "/api/v1/campaigns/images", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

func TestCampaignRoutesNeedALogin(t *testing.T) {
	app, _, _ := newApp(t)
	for _, r := range [][2]string{{"GET", "/api/v1/campaigns"}, {"PUT", "/api/v1/campaigns/x"}, {"GET", "/api/v1/campaigns/images/x"}} {
		if code, _ := do(t, app, r[0], r[1], "", ""); code != 401 {
			t.Errorf("%s %s needs a login: %d", r[0], r[1], code)
		}
	}
	if code, _ := upload(t, app, "", "a.png", pngBytes(t)); code != 401 {
		t.Fatalf("upload needs a login: %d", code)
	}
}

func TestOnlyAnAdminChangesCampaignsButAMemberReadsThem(t *testing.T) {
	app, _, member := newApp(t)
	if code, _ := do(t, app, "GET", "/api/v1/campaigns", member, ""); code != 200 {
		t.Fatalf("a member reads: %d", code)
	}
	if code, _ := do(t, app, "PUT", "/api/v1/campaigns/x", member, `{"steps":[{"kind":"text","text":"a"}]}`); code != 403 {
		t.Fatalf("a member cannot edit: %d", code)
	}
	if code, _ := upload(t, app, member, "a.png", pngBytes(t)); code != 403 {
		t.Fatalf("a member cannot upload: %d", code)
	}
}

func TestAdminSavesValidStepsOnly(t *testing.T) {
	app, admin, _ := newApp(t)
	if code, _ := do(t, app, "PUT", "/api/v1/campaigns/x", admin, `{"steps":[]}`); code != 400 {
		t.Fatalf("no steps: %d", code)
	}
	if code, _ := do(t, app, "PUT", "/api/v1/campaigns/x", admin, `{"steps":[{"kind":"image","imageId":"`+domain.NewID().Hex()+`"}]}`); code != 400 {
		t.Fatalf("a picture that is not stored: %d", code)
	}
	if code, _ := do(t, app, "PUT", "/api/v1/campaigns/x", admin, `{"steps":[{"kind":"text","text":"Chào {ten}"}]}`); code != 200 {
		t.Fatalf("a text step is fine: %d", code)
	}
}

func TestUploadKeepsOnlyRealPicturesAndServesThemBack(t *testing.T) {
	app, admin, member := newApp(t)
	if code, _ := upload(t, app, admin, "fake.png", []byte("this is only text")); code != 400 {
		t.Fatalf("text named .png is refused: %d", code)
	}
	if code, _ := upload(t, app, admin, "big.png", append(pngBytes(t), make([]byte, domain.MaxImageBytes)...)); code != 400 {
		t.Fatalf("over 2 MB is refused: %d", code)
	}
	code, raw := upload(t, app, admin, "ok.png", pngBytes(t))
	if code != 201 || !strings.Contains(raw, `"width":3`) {
		t.Fatalf("a real picture is stored: %d %s", code, raw)
	}
	i := strings.Index(raw, `"_id":"`) + len(`"_id":"`)
	id := raw[i : i+24]
	req := httptest.NewRequest("GET", "/api/v1/campaigns/images/"+id, nil)
	req.Header.Set("Authorization", "Bearer "+member)
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/png" || resp.Header.Get("X-Content-Type-Options") != "nosniff" || !bytes.Equal(got, pngBytes(t)) {
		t.Fatalf("the bytes come back whole with their type: %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
}

func TestOtherRoutesStillRefuseABodyOver1MB(t *testing.T) {
	app, admin, _ := newApp(t)
	big := `{"steps":[{"kind":"text","text":"` + strings.Repeat("a", 1<<20+10) + `"}]}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/campaigns/x", strings.NewReader(big))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+admin)
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 413 {
		t.Fatalf("only the upload may be bigger than 1 MB: %d", resp.StatusCode)
	}
}
