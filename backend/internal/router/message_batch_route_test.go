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

func TestTheBulkSwitchIsReadByMembersChangedOnlyByAnAdminAndOffByDefault(t *testing.T) {
	app, admin, member := newApp(t)

	if code, _ := do(t, app, "GET", "/api/v1/bulk-switch", "", ""); code != 401 {
		t.Fatalf("the switch is not public: %d", code)
	}
	code, body := do(t, app, "GET", "/api/v1/bulk-switch", member, "")
	if data, _ := body["data"].(map[string]any); code != 200 || data["enabled"] != false {
		t.Fatalf("a member sees it and it starts off: %d %v", code, body)
	}
	if code, _ := do(t, app, "PUT", "/api/v1/bulk-switch", member, `{"enabled":true,"confirm":true}`); code != 403 {
		t.Fatalf("a member cannot turn it on: %d", code)
	}
	if code, _ := do(t, app, "PUT", "/api/v1/bulk-switch", admin, `{"enabled":true}`); code != 400 {
		t.Fatalf("turning on needs a confirmation: %d", code)
	}
	if code, _ := do(t, app, "PUT", "/api/v1/bulk-switch", admin, `{"enabled":true,"confirm":true}`); code != 200 {
		t.Fatalf("an admin turns it on: %d", code)
	}
	_, body = do(t, app, "GET", "/api/v1/automation", member, "")
	if data, _ := body["data"].(map[string]any); data["enabled"] != false {
		t.Fatalf("the master switch of the chatbot is untouched: %v", body)
	}
}

func TestTheBulkSendLogNeedsALoginAndAnswersAList(t *testing.T) {
	app, _, member := newApp(t)
	if code, _ := do(t, app, "GET", "/api/v1/kocs/message-log", "", ""); code != 401 {
		t.Fatalf("needs a login: %d", code)
	}
	code, body := do(t, app, "GET", "/api/v1/kocs/message-log", member, "")
	if rows, ok := body["data"].([]any); code != 200 || !ok || len(rows) != 0 {
		t.Fatalf("an empty list: %d %v", code, body)
	}
}
