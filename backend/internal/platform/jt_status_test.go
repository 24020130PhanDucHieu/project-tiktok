package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tiktoday/kocflow-api/internal/config"
)

// jtServers starts a fake J&T Open API and a fake public portal and points a client at them.
func jtServers(t *testing.T, api, portal http.HandlerFunc) *JTClient {
	t.Helper()
	apiSrv := httptest.NewServer(api)
	portalSrv := httptest.NewServer(portal)
	t.Cleanup(apiSrv.Close)
	t.Cleanup(portalSrv.Close)
	c := NewJTClient(config.Config{JTAPIURL: apiSrv.URL, JTAPIAccount: "acc", JTPrivateKey: "key"})
	c.portalURL = portalSrv.URL
	return c
}

func TestJTOpenAPIRefusalFallsBackToThePublicPortal(t *testing.T) {
	c := jtServers(t,
		func(w http.ResponseWriter, r *http.Request) { // bad credentials: a JSON body with an error status
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":"401","msg":"invalid digest"}`))
		},
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("<html>Giao hàng thành công</html>"))
		})
	res, err := c.QueryTracking(context.Background(), "841000098765", "1234")
	if err != nil {
		t.Fatalf("a refused official API must fall back, not fail the caller: %v", err)
	}
	if res.Source != "jtexpress.vn" || !res.Delivered {
		t.Fatalf("expected the portal answer, got %+v", res)
	}
}

func TestJTNeverClaimsTransitWhenEverySourceFailed(t *testing.T) {
	fail := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) }
	c := jtServers(t, fail, fail)
	res, err := c.QueryTracking(context.Background(), "841000098765", "1234")
	if err != nil {
		t.Fatal(err)
	}
	if res.Delivered || strings.Contains(strings.ToLower(res.StatusText), "luân chuyển") {
		t.Fatalf("with no data the status must not say the parcel is moving: %+v", res)
	}
}

func TestJTOpenAPIWithoutAnyJourneyIsNotReportedAsInTransit(t *testing.T) {
	c := jtServers(t,
		func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"data":[]}`)) },
		func(w http.ResponseWriter, r *http.Request) {})
	res, err := c.QueryTracking(context.Background(), "841000098765", "1234")
	if err != nil {
		t.Fatal(err)
	}
	if res.Delivered || strings.Contains(strings.ToLower(res.StatusText), "luân chuyển") {
		t.Fatalf("an unknown bill has no journey yet: %+v", res)
	}
}

func TestJTDeliveredWordsMatchWholeWordsOnly(t *testing.T) {
	cases := map[string]bool{
		"Ký nhận":                     true,
		"Giao hàng thành công":        true,
		"Delivered":                   true,
		"Signing":                     true,
		"sign":                        true,
		"Assigned to courier":         false, // contains "sign" inside another word
		"Design pickup":               false,
		"Đang vận chuyển":             false,
		"":                            false,
		"Arrived at transit hub":      false,
		"Parcel assigned for pick-up": false,
	}
	for text, want := range cases {
		if got := isJTDelivered(text); got != want {
			t.Errorf("isJTDelivered(%q) = %v, want %v", text, got, want)
		}
	}
}
