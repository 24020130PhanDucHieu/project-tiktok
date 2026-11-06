package httpx

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

func TestEveryFaultKindHasItsOwnStatus(t *testing.T) {
	want := map[domain.FaultKind]int{
		domain.FaultInvalid: 400, domain.FaultUnauthorized: 401, domain.FaultForbidden: 403, domain.FaultNotFound: 404,
		domain.FaultConflict: 409, domain.FaultUnprocessable: 422, domain.FaultLocked: 423, domain.FaultRateLimited: 429,
		domain.FaultUnavailable: 502,
	}
	for k, status := range want {
		if got := StatusOf(k); got != status {
			t.Fatalf("kind %d: status %d, want %d", k, got, status)
		}
	}
	if StatusOf(0) != 500 {
		t.Fatal("an unknown kind is a server error")
	}
}

func TestTheErrorHandlerTurnsAFaultIntoItsStatusAndMessage(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	app.Get("/nf", func(*fiber.Ctx) error { return domain.NotFound("Không thấy") })
	app.Get("/locked", func(*fiber.Ctx) error { return domain.Locked("Đã khóa") })
	for path, want := range map[string]int{"/nf": 404, "/locked": 423} {
		resp, err := app.Test(httptest.NewRequest("GET", path, nil), 5000)
		if err != nil || resp.StatusCode != want {
			t.Fatalf("%s: %v %v", path, resp, err)
		}
	}
}
