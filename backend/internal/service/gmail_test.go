package service

import (
	"encoding/base64"
	"net/url"
	"strings"
	"testing"

	"github.com/tiktoday/kocflow-api/internal/config"
)

func testGmail() *GmailService {
	return &GmailService{cfg: config.Config{JWTSecret: "s3cret", GoogleClientID: "cid", GoogleClientSecret: "sec", GoogleRedirectURL: "http://x/cb"}}
}

func TestGmailStateAndSeal(t *testing.T) {
	g := testGmail()
	state := g.SignState("user-1")
	if id, ok := g.VerifyState(state); !ok || id != "user-1" {
		t.Fatalf("valid state rejected: %q %v", id, ok)
	}
	if _, ok := g.VerifyState(state + "x"); ok {
		t.Fatal("tampered state accepted")
	}
	other := &GmailService{cfg: config.Config{JWTSecret: "different"}}
	if _, ok := other.VerifyState(state); ok {
		t.Fatal("state signed with another secret accepted")
	}

	sealed, err := g.seal("refresh-token")
	if err != nil || strings.Contains(sealed, "refresh-token") {
		t.Fatalf("seal: %v %q", err, sealed)
	}
	if plain, err := g.open(sealed); err != nil || plain != "refresh-token" {
		t.Fatalf("open: %v %q", err, plain)
	}
	if _, err := other.open(sealed); err == nil {
		t.Fatal("token opened with the wrong key")
	}
}

func TestGmailAuthURLAndRawMail(t *testing.T) {
	g := testGmail()
	u, err := url.Parse(g.AuthURL("user-1"))
	if err != nil || u.Query().Get("client_id") != "cid" || u.Query().Get("access_type") != "offline" || !strings.Contains(u.Query().Get("scope"), "gmail.send") {
		t.Fatalf("auth url: %v %v", err, u)
	}
	raw, err := base64.URLEncoding.DecodeString(BuildRawMail("me@x.com", "koc@y.com", KocMailSubject, "Xin chào bạn"))
	if err != nil {
		t.Fatal(err)
	}
	mail := string(raw)
	if !strings.Contains(mail, "To: koc@y.com") || !strings.Contains(mail, "Subject: =?UTF-8?") || !strings.Contains(mail, "Content-Transfer-Encoding: base64") {
		t.Fatalf("headers: %q", mail)
	}
	body := mail[strings.Index(mail, "\r\n\r\n")+4:]
	if dec, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(body, "\r\n", "")); err != nil || string(dec) != "Xin chào bạn" {
		t.Fatalf("body: %v %q", err, body)
	}
}
