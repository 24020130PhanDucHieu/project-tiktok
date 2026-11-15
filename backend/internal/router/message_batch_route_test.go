package router_test

import "testing"

func TestMessageBatchNeedsALogin(t *testing.T) {
	app, _, _ := newApp(t)
	code, _ := do(t, app, "POST", "/api/v1/kocs/message-batch", "", `{"body":"Chào","kocIds":["x"],"confirm":true}`)
	if code != 401 {
		t.Fatalf("a bulk message reaches real people, so it needs a login: %d", code)
	}
}

func TestMessageBatchRefusesAnUnconfirmedRequestForAMember(t *testing.T) {
	app, _, member := newApp(t)
	code, body := do(t, app, "POST", "/api/v1/kocs/message-batch", member, `{"body":"Chào","kocIds":["x"]}`)
	if code != 400 {
		t.Fatalf("a member may use the route, but only with confirm=true, and no database is touched before that: %d %v", code, body)
	}
}
