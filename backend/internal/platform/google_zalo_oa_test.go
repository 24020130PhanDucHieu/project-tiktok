package platform

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/tiktoday/kocflow-api/internal/service"
)

type stubTransport func(*http.Request) (int, string)

func (f stubTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	code, body := f(r)
	return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
}

func TestGoogleClientTokenMapsInvalidGrantToBroken(t *testing.T) {
	c := NewGoogleClient()
	c.http.Transport = stubTransport(func(r *http.Request) (int, string) {
		if r.URL.String() != googleTokenURL {
			t.Errorf("wrong url %s", r.URL)
		}
		return 400, `{"error":"invalid_grant"}`
	})
	if _, err := c.Token(context.Background(), url.Values{}); !errors.Is(err, service.ErrGmailBroken) {
		t.Fatalf("invalid_grant must be ErrGmailBroken, got %v", err)
	}
	c.http.Transport = stubTransport(func(*http.Request) (int, string) { return 200, `{"access_token":"a"}` })
	if out, err := c.Token(context.Background(), url.Values{}); err != nil || out["access_token"] != "a" {
		t.Fatalf("token: %v %v", out, err)
	}
}

func TestGoogleClientSendRawAndGetJSON(t *testing.T) {
	c := NewGoogleClient()
	c.http.Transport = stubTransport(func(r *http.Request) (int, string) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			return 401, `{}`
		}
		if r.Method == http.MethodPost {
			return 200, `{"id":"m1"}`
		}
		return 200, `{"email":"a@b.c"}`
	})
	if id, err := c.SendRaw(context.Background(), "tok", "raw"); err != nil || id != "m1" {
		t.Fatalf("send: %q %v", id, err)
	}
	var info struct{ Email string }
	if err := c.GetJSON(context.Background(), "tok", "https://x/y", &info); err != nil || info.Email != "a@b.c" {
		t.Fatalf("get: %+v %v", info, err)
	}
	if _, err := c.SendRaw(context.Background(), "bad", "raw"); err == nil {
		t.Fatal("a refused send must fail")
	}
}

func TestZaloOAClientReportsApiErrors(t *testing.T) {
	c := NewZaloOAClient()
	c.http.Transport = stubTransport(func(r *http.Request) (int, string) {
		if r.Header.Get("access_token") != "tk" {
			return 200, `{"error":-216,"message":"token invalid"}`
		}
		return 200, `{"error":0}`
	})
	if err := c.SendText(context.Background(), "tk", "42", "hi"); err != nil {
		t.Fatalf("ok send: %v", err)
	}
	if err := c.SendText(context.Background(), "no", "42", "hi"); err == nil || err.Error() != "token invalid" {
		t.Fatalf("api error must surface its message, got %v", err)
	}
}
