package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var targetNow = time.Date(2026, 11, 5, 10, 0, 0, 0, vnZone)

func validInvite() TargetInviteInput {
	return TargetInviteInput{
		Name:          "Khăn lạnh T11",
		EndDate:       "2026-11-30",
		Email:         "shop@example.com",
		Zalo:          "0981097380",
		ContentPref:   "video",
		Message:       "Chào bạn! Bên mình muốn hợp tác.",
		HasFreeSample: true,
		SampleExempt:  true,
		Products:      []TargetProductInput{{ID: "1736294693654136704", Title: "Khăn lạnh", CommissionPct: 10, AdsCommissionPct: 5}},
		Creators:      []TargetCreatorInput{{OpenID: "uAbc123", Username: "trlyng.review"}},
	}
}

func TestBuildTargetPayloadUsesTikTokFieldNamesAndUnits(t *testing.T) {
	p, problems := BuildTargetPayload(validInvite(), targetNow, 10)
	if len(problems) != 0 {
		t.Fatalf("a valid form must have no problems: %v", problems)
	}
	raw, _ := json.Marshal(p)
	var got map[string]any
	_ = json.Unmarshal(raw, &got)

	if got["name"] != "Khăn lạnh T11" {
		t.Fatalf("name: %v", got["name"])
	}
	// 2026-11-30 23:59:59 Vietnam time = 16:59:59 UTC
	if got["end_time"] != "1796057999" {
		t.Fatalf("end_time must be the end of the chosen day in Vietnam time as a Unix string, got %v", got["end_time"])
	}
	products := got["products"].([]any)
	first := products[0].(map[string]any)
	if first["id"] != "1736294693654136704" || first["target_commission_rate"].(float64) != 1000 || first["shop_ads_commission_rate"].(float64) != 500 {
		t.Fatalf("product must carry rates in 1/10,000: %v", first)
	}
	if got["seller_contact_info"].(map[string]any)["email"] != "shop@example.com" {
		t.Fatalf("contact: %v", got["seller_contact_info"])
	}
	rule := got["free_sample_rule"].(map[string]any)
	if rule["has_free_sample"] != true || rule["is_sample_approval_exempt"] != true {
		t.Fatalf("free sample rule: %v", rule)
	}
	ids := got["creator_user_open_ids"].([]any)
	if len(ids) != 1 || ids[0] != "uAbc123" {
		t.Fatalf("creators: %v", ids)
	}
	msg := got["message"].(string)
	for _, want := range []string{"Chào bạn!", "Zalo: 0981097380", "Ưu tiên: video giới thiệu sản phẩm"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message must contain %q, got %q", want, msg)
		}
	}
	if strings.Contains(string(raw), "target_ad_commission_rate") || strings.Contains(string(raw), `"commission_rate"`) {
		t.Fatalf("the update-only field names must not appear in a create body: %s", raw)
	}
}

func TestBuildTargetPayloadOmitsAdsRateWhenUnsetAndExemptNeedsSamples(t *testing.T) {
	in := validInvite()
	in.Products[0].AdsCommissionPct = 0
	in.HasFreeSample, in.SampleExempt = false, true
	p, problems := BuildTargetPayload(in, targetNow, 10)
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	raw, _ := json.Marshal(p)
	if strings.Contains(string(raw), "shop_ads_commission_rate") {
		t.Fatalf("an unset ads rate must not be sent: %s", raw)
	}
	if p.FreeSampleRule.IsSampleApprovalExempt {
		t.Fatal("approval exemption must be off when there are no free samples")
	}
}

func TestBuildTargetPayloadRejectsBadInput(t *testing.T) {
	cases := []struct {
		label  string
		mutate func(*TargetInviteInput)
		want   string
	}{
		{"empty name", func(i *TargetInviteInput) { i.Name = "  " }, "đặt tên"},
		{"long name", func(i *TargetInviteInput) { i.Name = strings.Repeat("a", 31) }, "tối đa 30"},
		{"no end date", func(i *TargetInviteInput) { i.EndDate = "" }, "ngày hết hạn"},
		{"end date today", func(i *TargetInviteInput) { i.EndDate = "2026-11-05" }, "từ ngày mai"},
		{"end date past", func(i *TargetInviteInput) { i.EndDate = "2026-10-01" }, "từ ngày mai"},
		{"end date too far", func(i *TargetInviteInput) { i.EndDate = "2028-01-01" }, "không quá"},
		{"bad email", func(i *TargetInviteInput) { i.Email = "nope" }, "Email"},
		{"bad zalo", func(i *TargetInviteInput) { i.Zalo = "abc" }, "Zalo"},
		{"bad content pref", func(i *TargetInviteInput) { i.ContentPref = "x" }, "ưu tiên"},
		{"empty message", func(i *TargetInviteInput) { i.Message = "" }, "nội dung lời mời"},
		{"message too long", func(i *TargetInviteInput) { i.Message = strings.Repeat("a", 500) }, "tối đa 500"},
		{"no products", func(i *TargetInviteInput) { i.Products = nil }, "ít nhất một sản phẩm"},
		{"too many products", func(i *TargetInviteInput) {
			for n := 0; n < TargetMaxProducts+1; n++ {
				i.Products = append(i.Products, TargetProductInput{ID: "1" + strings.Repeat("0", n+1), CommissionPct: 10})
			}
		}, "tối đa 20 sản phẩm"},
		{"bad product id", func(i *TargetInviteInput) { i.Products[0].ID = "../x" }, "không hợp lệ"},
		{"duplicate product", func(i *TargetInviteInput) { i.Products = append(i.Products, i.Products[0]) }, "hai lần"},
		{"commission below 10", func(i *TargetInviteInput) { i.Products[0].CommissionPct = 9.99 }, "từ 10% đến 80%"},
		{"commission above 80", func(i *TargetInviteInput) { i.Products[0].CommissionPct = 80.01 }, "từ 10% đến 80%"},
		{"ads below 1", func(i *TargetInviteInput) { i.Products[0].AdsCommissionPct = 0.5 }, "quảng cáo"},
		{"ads above 80", func(i *TargetInviteInput) { i.Products[0].AdsCommissionPct = 81 }, "quảng cáo"},
		{"no creators", func(i *TargetInviteInput) { i.Creators = nil }, "ít nhất một nhà sáng tạo"},
		{"too many creators", func(i *TargetInviteInput) {
			for n := 0; n < 3; n++ {
				i.Creators = append(i.Creators, TargetCreatorInput{OpenID: "u" + string(rune('a'+n)), Username: "x" + string(rune('a'+n))})
			}
		}, "tối đa 3 nhà sáng tạo"},
		{"creator without id", func(i *TargetInviteInput) { i.Creators[0].OpenID = "" }, "Thiếu thông tin"},
		{"duplicate creator", func(i *TargetInviteInput) { i.Creators = append(i.Creators, i.Creators[0]) }, "hai lần"},
	}
	for _, c := range cases {
		in := validInvite()
		c.mutate(&in)
		_, problems := BuildTargetPayload(in, targetNow, 3)
		joined := strings.Join(problems, " | ")
		if len(problems) == 0 || !strings.Contains(joined, c.want) {
			t.Fatalf("%s: expected a problem containing %q, got %q", c.label, c.want, joined)
		}
	}
}

func TestBuildTargetPayloadAcceptsTheLimitValues(t *testing.T) {
	in := validInvite()
	in.Products[0].CommissionPct = 10
	in.Products = append(in.Products, TargetProductInput{ID: "2", CommissionPct: 80, AdsCommissionPct: 1})
	in.EndDate = "2026-11-06" // tomorrow is the earliest allowed day
	if _, problems := BuildTargetPayload(in, targetNow, 10); len(problems) != 0 {
		t.Fatalf("limit values must be accepted: %v", problems)
	}
}

func TestOnlyTheThreeTargetWritesAreAllowed(t *testing.T) {
	allowed := [][2]string{
		{"POST", "/affiliate_seller/202508/target_collaborations"},
		{"POST", "/affiliate_seller/202509/target_collaboration/123/link"},
		{"DELETE", "/affiliate_seller/202409/target_collaborations/123"},
	}
	for _, a := range allowed {
		if !ttsWriteAllowed(a[0], a[1]) {
			t.Fatalf("%s %s must be allowed", a[0], a[1])
		}
	}
	refused := [][2]string{
		{"PUT", "/affiliate_seller/202508/target_collaborations/123"},                     // edit
		{"DELETE", "/affiliate_seller/202508/target_collaborations"},                      // no id
		{"DELETE", "/affiliate_seller/202409/target_collaborations/12a"},                  // non numeric id
		{"DELETE", "/affiliate_seller/202409/target_collaborations/123/x"},                // extra segment
		{"POST", "/affiliate_seller/202508/target_collaborations/123"},                    // wrong method
		{"POST", "/affiliate_seller/202405/open_collaborations"},                          // another write
		{"DELETE", "/affiliate_seller/202409/open_collaborations/products/1"},             // remove open collaboration
		{"POST", "/affiliate_seller/202508/target_collaborations/../open_collaborations"}, // traversal
		{"POST", "/product/202309/products/1/prices/update"},
		{"POST", "/affiliate_seller/202509/target_collaboration/1%2F2/link"},
	}
	for _, r := range refused {
		if ttsWriteAllowed(r[0], r[1]) {
			t.Fatalf("%s %s must be refused", r[0], r[1])
		}
	}

	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
	defer srv.Close()
	s := newTTShopForTest(srv.URL)
	if _, err := s.callWrite(context.Background(), "PUT", "/affiliate_seller/202508/target_collaborations/1", nil, map[string]string{}); err == nil {
		t.Fatal("callWrite must refuse a route outside the list")
	}
	if hits != 0 {
		t.Fatalf("a refused write must never reach TikTok, got %d requests", hits)
	}
}

func TestParseTargetCreated(t *testing.T) {
	id, err := parseTargetCreated([]byte(`{"target_collaboration":{"id":"7777"}}`))
	if err != nil || id != "7777" {
		t.Fatalf("got %q %v", id, err)
	}
	_, err = parseTargetCreated([]byte(`{"target_collaboration_conflicts":[{"creator_user_open_id":"uA","product_id":"1"}]}`))
	var conflict *ErrTargetConflict
	if !errors.As(err, &conflict) || len(conflict.Conflicts) != 1 || conflict.Conflicts[0].CreatorOpenID != "uA" {
		t.Fatalf("conflicts must be reported, got %v", err)
	}
	if _, err := parseTargetCreated([]byte(`{}`)); err == nil {
		t.Fatal("no id and no conflicts must be an error, never a success")
	}
	if _, err := parseTargetCreated([]byte(`nope`)); err == nil {
		t.Fatal("invalid JSON must fail")
	}
}

func TestParseCreatorsAndProducts(t *testing.T) {
	cs, err := parseCreators([]byte(`{"creators":[
		{"creator_open_id":"uA","username":"trlyng.review","nickname":"Tủ đồ","follower_count":4000,"avatar":{"url":"https://a/1.jpg"}},
		{"creator_open_id":"","username":"ghost"}]}`))
	if err != nil || len(cs) != 1 || cs[0].OpenID != "uA" || cs[0].Username != "trlyng.review" || cs[0].Followers != 4000 || cs[0].AvatarURL != "https://a/1.jpg" {
		t.Fatalf("creators: %+v %v", cs, err)
	}
	local, err := parseShopProducts([]byte(`{"products":[{"id":"2","title":"B","skus":[{"price":{"currency":"VND","tax_exclusive_price":"99000"}}]}]}`))
	if err != nil || local.Items[0].MinPrice != "99000" {
		t.Fatalf("the price must fall back to tax_exclusive_price: %+v %v", local, err)
	}
	page, err := parseShopProducts([]byte(`{"next_page_token":"n","total_count":2,"products":[
		{"id":"1","title":"A","status":"ACTIVATE","skus":[
			{"price":{"currency":"VND","sale_price":"180000"},"inventory":[{"quantity":10}]},
			{"price":{"currency":"VND","sale_price":"520000"},"inventory":[{"quantity":5}]}]}]}`))
	if err != nil || len(page.Items) != 1 || page.Items[0].MinPrice != "180000" || page.Items[0].MaxPrice != "520000" || page.Items[0].Inventory != 15 || page.NextPageToken != "n" {
		t.Fatalf("products: %+v %v", page, err)
	}
}

func TestBuildTargetSearchBody(t *testing.T) {
	body, err := buildTargetSearchBody(TargetListParams{Status: "ONGOING", AcceptStatus: "ACCEPT", Keyword: " khăn "})
	if err != nil {
		t.Fatal(err)
	}
	if body["collaboration_status"] != "ONGOING" || body["creator_accept_status"] != "ACCEPT" {
		t.Fatalf("filters: %v", body)
	}
	sp := body["search_param"].(map[string]string)
	if sp["keyword"] != "khăn" || sp["keyword_type"] != "TARGET_COLLABORATION_NAME" {
		t.Fatalf("keyword defaults to the invitation name: %v", sp)
	}
	empty, _ := buildTargetSearchBody(TargetListParams{})
	if len(empty) != 3 || empty["collaboration_status"] != "VALID" || empty["creator_accept_status"] != "ALL" || empty["free_sample_setting"] != "ALL" {
		t.Fatalf("TikTok needs all three filters, so no filter means VALID/ALL/ALL: %v", empty)
	}
	if _, err := buildTargetSearchBody(TargetListParams{Status: "WHATEVER"}); err == nil {
		t.Fatal("an unknown status must be refused")
	}
	if _, err := buildTargetSearchBody(TargetListParams{Keyword: "x", KeywordType: "DROP"}); err == nil {
		t.Fatal("an unknown keyword type must be refused")
	}
}

func TestParseTargetPageAndDetail(t *testing.T) {
	page, err := parseTargetPage([]byte(`{"total_count":1,"next_page_token":"p2","target_collaborations":[
		{"id":"9","name":"emanhthethao","type":"STANDARD","product_count":1,"creator_inivited_count":2,"showcase_creator_count":1,"content_creator_count":1,
		 "start_time":100,"end_time":200,"free_sample_rule":{"has_free_sample":true,"is_sample_approval_exempt":false}}]}`))
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("%+v %v", page, err)
	}
	it := page.Items[0]
	if it.InvitedCount != 2 || it.ShowcaseCreatorCount != 1 || it.ContentCreatorCount != 1 || !it.HasFreeSample || it.SampleExempt || page.NextPageToken != "p2" {
		t.Fatalf("summary: %+v", it)
	}
	d, err := parseTargetDetail([]byte(`{"target_collaboration":{"id":"9","name":"n","creator_invited_count":2,
		"seller_contact_info":{"email":"a@b.co"},
		"creators":[{"creator_open_id":"uA","username":"x","collaboration_status":"NORMAL","showcase_product_count":1,"avatar":{"url":"https://a"}}],
		"products":[{"id":"1","title":"T","status":"LIVE","main_image_url":"https://i","original_price":{"currency":"VND","minimum_amount":"1","maximum_amount":"2"},
		"commission":{"rate":1350,"shop_ads_commission_rate":500}}]}}`))
	if err != nil || d.Email != "a@b.co" || len(d.Creators) != 1 || d.Creators[0].Status != "NORMAL" || len(d.Products) != 1 {
		t.Fatalf("detail: %+v %v", d, err)
	}
	if d.Products[0].CommissionPct != 13.5 || d.Products[0].AdsPct != 5 || d.InvitedCount != 2 {
		t.Fatalf("rates must come back as percent: %+v", d.Products[0])
	}
}

func TestIDGuardForReadsAndWrites(t *testing.T) {
	s := newTTShopForTest("")
	for _, id := range []string{"", "12ab", "../x", "1/2"} {
		if _, err := s.GetTarget(context.Background(), id); err != ErrBadProductID {
			t.Fatalf("GetTarget %q must be refused, got %v", id, err)
		}
		if _, err := s.TargetLink(context.Background(), id); err != ErrBadProductID {
			t.Fatalf("TargetLink %q must be refused, got %v", id, err)
		}
		if err := s.RemoveTarget(context.Background(), id); err != ErrBadProductID {
			t.Fatalf("RemoveTarget %q must be refused, got %v", id, err)
		}
	}
}

func TestTTSRejectedKeepsTheMessageAndIsDetectable(t *testing.T) {
	var err error = &TTSRejected{Code: 123, Message: "denied"}
	var rej *TTSRejected
	if !errors.As(err, &rej) || rej.Code != 123 || !strings.Contains(err.Error(), "mã 123") || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestInviteMessageNeverRepeatsTheEmail(t *testing.T) {
	msg := composeInviteMessage(TargetInviteInput{Message: "Chào bạn", Email: "shop@example.com", ContentPref: "video"})
	if strings.Contains(msg, "shop@example.com") || strings.Contains(msg, "@") || strings.Contains(strings.ToLower(msg), "link") {
		t.Fatalf("the message must carry no email address or link: %q", msg)
	}
}

func TestNoEmailMeansNoSellerContactInfoInTheBody(t *testing.T) {
	in := TargetInviteInput{Name: "Khăn lạnh", EndDate: time.Now().AddDate(0, 0, 10).Format("2006-01-02"), Message: "Chào bạn",
		Products: []TargetProductInput{{ID: "1", CommissionPct: 10}}, Creators: []TargetCreatorInput{{OpenID: "u1", Username: "a"}}}
	p, problems := BuildTargetPayload(in, time.Now(), 10)
	if len(problems) != 0 || p == nil {
		t.Fatalf("an invitation without an email is valid: %v", problems)
	}
	b, _ := json.Marshal(p)
	if strings.Contains(string(b), "seller_contact_info") {
		t.Fatalf("no email: the field must be left out: %s", b)
	}
	in.Email = "not an email"
	if _, problems := BuildTargetPayload(in, time.Now(), 10); len(problems) == 0 {
		t.Fatal("an email that is given must still be valid")
	}
}
