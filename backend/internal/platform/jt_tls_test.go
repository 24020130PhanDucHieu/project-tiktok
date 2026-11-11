package platform

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The bundled intermediate only completes a chain: a server whose certificate no trusted root signed is still refused.
func TestTheJTClientStillRefusesAnUntrustedCertificate(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	if resp, err := jtHTTPClient().Get(srv.URL); err == nil {
		resp.Body.Close()
		t.Fatal("a self-signed server must be refused")
	}
}
