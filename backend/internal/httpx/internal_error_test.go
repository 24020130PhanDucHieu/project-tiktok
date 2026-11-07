package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/mongo"
)

func errorBody(t *testing.T, app *fiber.App, path string) (int, string) {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest("GET", path, nil), 5000)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var body struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(raw, &body)
	return resp.StatusCode, body.Message
}

func TestTheSafeHandlerKeepsInfrastructureErrorsOffTheWire(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: NewErrorHandler(false)})
	app.Get("/db", func(*fiber.Ctx) error {
		return fmt.Errorf("find kocs: %w", mongo.CommandError{Code: 13, Message: "not authorized on prod-db-host:27017"})
	})
	app.Get("/slow", func(*fiber.Ctx) error { return fmt.Errorf("read: %w", context.DeadlineExceeded) })

	for _, path := range []string{"/db", "/slow"} {
		status, msg := errorBody(t, app, path)
		if status != 500 {
			t.Fatalf("%s: status %d, want 500", path, status)
		}
		if strings.Contains(msg, "prod-db-host") || strings.Contains(msg, "deadline") || msg == "" {
			t.Fatalf("%s: the client must get a generic message, got %q", path, msg)
		}
	}
}

func TestTheSafeHandlerStillShowsAMessageWrittenForTheUser(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: NewErrorHandler(false)})
	app.Get("/plain", func(*fiber.Ctx) error { return errors.New("KOC chưa có email") })

	if _, msg := errorBody(t, app, "/plain"); msg != "KOC chưa có email" {
		t.Fatalf("a sentence written for the user must survive, got %q", msg)
	}
}

func TestTheDevelopmentHandlerShowsTheRealError(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: NewErrorHandler(true)})
	app.Get("/db", func(*fiber.Ctx) error {
		return mongo.CommandError{Code: 13, Message: "not authorized on prod-db-host:27017"}
	})

	if _, msg := errorBody(t, app, "/db"); !strings.Contains(msg, "prod-db-host") {
		t.Fatalf("development keeps the real text to help debugging, got %q", msg)
	}
}
