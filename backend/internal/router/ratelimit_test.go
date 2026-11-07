package router_test

import (
	"net/http"
	"testing"
)

func TestRoutesThatSpendMoneyOrReachRealPeopleAreRateLimited(t *testing.T) {
	for _, c := range []struct {
		name, method, path, body string
		limit                    int
	}{
		{"J&T lookup", "POST", "/api/v1/kocs/track-shipment/lookup", `{}`, 30},
		{"TikTok video lookup", "POST", "/api/v1/kocs/video/lookup", `{}`, 30},
		{"Zalo login", "POST", "/api/v1/zalo/connect", `{}`, 10},
		{"Zalo logout", "POST", "/api/v1/zalo/disconnect", `{}`, 10},
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
