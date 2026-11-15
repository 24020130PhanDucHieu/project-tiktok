package router_test

import (
	"net/http"
	"testing"
)

func TestTheSignUpOptionsArePublicAndClosedByDefault(t *testing.T) {
	app, _, _ := newApp(t)
	code, body := do(t, app, "GET", "/api/v1/auth/options", "", "")
	data, _ := body["data"].(map[string]any)
	if code != 200 || data["allowRegistration"] != false {
		t.Fatalf("the login page reads this without a token, and sign-up stays closed unless configured: %d %v", code, body)
	}
}

func TestRoutesThatSpendMoneyOrReachRealPeopleAreRateLimited(t *testing.T) {
	for _, c := range []struct {
		name, method, path, body string
		limit                    int
	}{
		{"J&T lookup", "POST", "/api/v1/kocs/track-shipment/lookup", `{}`, 30},
		{"TikTok video lookup", "POST", "/api/v1/kocs/video/lookup", `{}`, 30},
		{"Zalo login", "POST", "/api/v1/zalo/connect", `{}`, 10},
		{"Zalo logout", "POST", "/api/v1/zalo/disconnect", `{}`, 10},
		{"sign-up", "POST", "/api/v1/auth/register", `{"name":"Bot","email":"bot@x.vn","password":"Matkhau123"}`, 5},
	} {
		// a rate limit lives in the app, so every case gets its own; the bodies fail validation, so no database is touched
		app, token, _ := newApp(t)
		for i := 1; i <= c.limit; i++ {
			if code, _ := do(t, app, c.method, c.path, token, c.body); code == http.StatusTooManyRequests {
				t.Fatalf("%s: call %d is within the limit of %d but was refused", c.name, i, c.limit)
			}
		}
		if code, body := do(t, app, c.method, c.path, token, c.body); code != http.StatusTooManyRequests {
			t.Errorf("%s: call %d must be refused with 429, got %d %v", c.name, c.limit+1, code, body)
		}
	}
}

// The API trusts X-Forwarded-For only from the reverse proxy in front of it (a private address). A caller that
// reaches the API directly could otherwise send a new fake address with every request and never hit a rate limit.
func TestAForgedForwardedAddressDoesNotEscapeTheRateLimit(t *testing.T) {
	app, _, _ := newApp(t)
	body := `{"name":"Bot","email":"bot@x.vn","password":"Matkhau123"}`
	for i := 1; i <= 5; i++ {
		do(t, app, "POST", "/api/v1/auth/register", "", body, [2]string{"X-Forwarded-For", "203.0.113." + string(rune('0'+i))})
	}
	if code, _ := do(t, app, "POST", "/api/v1/auth/register", "", body, [2]string{"X-Forwarded-For", "203.0.113.99"}); code != http.StatusTooManyRequests {
		t.Fatalf("a new forged address on call 6 must still be refused, got %d", code)
	}
}
