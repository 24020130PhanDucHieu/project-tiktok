package campaignseed

import (
	"strings"
	"testing"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

func TestTheThreeCampaignsAndTheFirstOnesSteps(t *testing.T) {
	cs := Campaigns()
	want := []string{"Liên hệ KOC mới", "Nhắc KOC phản hồi thông tin hợp tác", "Gửi thêm thông tin sản phẩm"}
	if len(cs) != 3 {
		t.Fatalf("three campaigns, got %d", len(cs))
	}
	for i, n := range want {
		if cs[i].Name != n {
			t.Fatalf("campaign %d is %q, want %q", i, cs[i].Name, n)
		}
	}
	first := cs[0].Steps
	if len(first) != 9 {
		t.Fatalf("nine steps, got %d", len(first))
	}
	if first[0].Text != "Chào bạn" {
		t.Fatalf("step 1: %q", first[0].Text)
	}
	if !strings.Contains(first[2].Text, "{kenh}") || strings.Contains(first[2].Text, "emilyhuong69") {
		t.Fatalf("step 3 names the channel by the placeholder: %q", first[2].Text)
	}
	if first[3].Kind != "image" || first[3].File != "anh-san-pham.png" || first[4].Kind != "image" || first[4].File != "loi-moi-hop-tac.png" {
		t.Fatalf("steps 4 and 5 are the two pictures: %+v %+v", first[3], first[4])
	}
}

func TestEverySeededStepIsValidAndEveryPictureIsThere(t *testing.T) {
	for _, c := range Campaigns() {
		steps := make([]domain.CampaignStep, len(c.Steps))
		for i, s := range c.Steps {
			steps[i] = domain.CampaignStep{Kind: s.Kind, Text: s.Text, Caption: s.Caption}
			if s.Kind == "image" {
				data, err := Image(s.File)
				if err != nil || len(data) == 0 || len(data) > domain.MaxImageBytes {
					t.Fatalf("%s: picture %s unusable (%d bytes, %v)", c.Name, s.File, len(data), err)
				}
				steps[i].ImageID = domain.NewID() // stands in for the stored picture
			}
			if strings.Contains(strings.ToLower(s.Text), "tặng") {
				t.Fatalf("%s: the project never says \"tặng\": %q", c.Name, s.Text)
			}
		}
		if err := domain.ValidateSteps(steps); err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
	}
}
