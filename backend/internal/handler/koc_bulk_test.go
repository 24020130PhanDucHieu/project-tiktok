package handler

import (
	"context"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/service"
)

// bulkKocAPI records the rows the bulk route hands to the service.
type bulkKocAPI struct {
	KocAPI
	calls [][]service.BulkItem
}

func (b *bulkKocAPI) BulkImport(_ context.Context, items []service.BulkItem, _ string) (*service.BulkReport, error) {
	b.calls = append(b.calls, items)
	return &service.BulkReport{Imported: nil}, nil
}

func bulkApp(api *bulkKocAPI) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: httpx.ErrorHandler})
	app.Post("/kocs/bulk", NewKocHandler(api, nil, nil, nil).BulkImport)
	return app
}

func bulkBody(n int, extra string) string {
	rows := make([]string, n)
	for i := range rows {
		rows[i] = `{"tiktokId":"kenh` + strings.Repeat("a", i%5) + `"` + extra + `}`
	}
	return `{"items":[` + strings.Join(rows, ",") + `]}`
}

func TestBulkImportRefusesMoreThan20ItemsInOneRequest(t *testing.T) {
	api := &bulkKocAPI{}
	if code, _ := postJSON(bulkApp(api), "/kocs/bulk", bulkBody(21, "")); code != 400 || len(api.calls) != 0 {
		t.Fatalf("21 rows: want 400 and no call, got %d %d", code, len(api.calls))
	}
	if code, _ := postJSON(bulkApp(api), "/kocs/bulk", bulkBody(20, "")); code != 201 || len(api.calls) != 1 {
		t.Fatalf("20 rows are fine: %d %d", code, len(api.calls))
	}
}

func TestBulkImportRefusesABadEmail(t *testing.T) {
	api := &bulkKocAPI{}
	if code, _ := postJSON(bulkApp(api), "/kocs/bulk", bulkBody(1, `,"email":"abc"`)); code != 400 || len(api.calls) != 0 {
		t.Fatalf("a bad email: want 400 and no call, got %d", code)
	}
}

func TestBulkImportPassesTheContactFieldsToTheService(t *testing.T) {
	api := &bulkKocAPI{}
	body := `{"items":[{"tiktokId":"abc","email":"a@x.vn","recipient":"Lan","shippingAddress":"1 A, Hà Nội","phone":"0976451571","castFee":500000,"category":"Làm đẹp"}]}`
	if code, _ := postJSON(bulkApp(api), "/kocs/bulk", body); code != 201 || len(api.calls) != 1 || len(api.calls[0]) != 1 {
		t.Fatalf("one row goes through: %d %v", code, api.calls)
	}
	it := api.calls[0][0]
	if it.Email != "a@x.vn" || it.Recipient != "Lan" || it.ShippingAddress != "1 A, Hà Nội" || it.Phone != "0976451571" || it.Category != "Làm đẹp" || it.CastFee == nil || *it.CastFee != 500000 {
		t.Fatalf("the service gets every field of the row: %+v", it)
	}
}
