package domain

import (
	"strings"
	"time"
	"unicode/utf8"
)

// The kinds of step of a campaign.
const (
	StepText  = "text"
	StepImage = "image"
)

// Limits of a campaign.
const (
	MaxCampaignSteps = 12
	MaxStepText      = 2000
	MaxImageBytes    = 2 << 20
)

// CampaignStep is one message of a campaign: a text, or a picture with an optional caption. Texts may use {ten} (the KOC's
// name) and {kenh} (the KOC's TikTok channel, with its @).
type CampaignStep struct {
	Kind    string `bson:"kind" json:"kind"`
	Text    string `bson:"text,omitempty" json:"text,omitempty"`
	ImageID ID     `bson:"imageId,omitempty" json:"imageId,omitempty"`
	Caption string `bson:"caption,omitempty" json:"caption,omitempty"`
}

// Campaign is a named series of steps sent one after the other to the same KOC.
type Campaign struct {
	ID        ID             `bson:"_id,omitempty" json:"_id"`
	Name      string         `bson:"name" json:"name"`
	Steps     []CampaignStep `bson:"steps" json:"steps"`
	UpdatedAt time.Time      `bson:"updatedAt" json:"updatedAt"`
}

// CampaignImage is a picture used by campaign steps. The bytes are kept with it and never put in a JSON answer.
type CampaignImage struct {
	ID          ID        `bson:"_id,omitempty" json:"_id"`
	Filename    string    `bson:"filename" json:"filename"`
	ContentType string    `bson:"contentType" json:"contentType"`
	Width       int       `bson:"width" json:"width"`
	Height      int       `bson:"height" json:"height"`
	Size        int       `bson:"size" json:"size"`
	Data        []byte    `bson:"data" json:"-"`
	CreatedAt   time.Time `bson:"createdAt" json:"createdAt"`
}

// ValidateSteps checks the steps of a campaign or of a run, and says in Vietnamese what is wrong.
func ValidateSteps(steps []CampaignStep) error {
	if len(steps) == 0 {
		return Invalid("Chiến dịch cần ít nhất một tin.")
	}
	if len(steps) > MaxCampaignSteps {
		return Invalid("Một chiến dịch có tối đa 12 tin.")
	}
	for i, s := range steps {
		n := i + 1
		switch s.Kind {
		case StepText:
			text := strings.TrimSpace(s.Text)
			if text == "" {
				return Invalid("Tin số " + itoa(n) + " đang trống.")
			}
			if utf8.RuneCountInString(text) > MaxStepText {
				return Invalid("Tin số " + itoa(n) + " dài quá 2000 ký tự.")
			}
		case StepImage:
			if s.ImageID.IsZero() {
				return Invalid("Tin số " + itoa(n) + " là ảnh nhưng chưa có ảnh.")
			}
			if utf8.RuneCountInString(s.Caption) > MaxStepText {
				return Invalid("Chú thích của tin số " + itoa(n) + " dài quá 2000 ký tự.")
			}
		default:
			return Invalid("Tin số " + itoa(n) + " có loại không hợp lệ.")
		}
	}
	return nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for ; n > 0; n /= 10 {
		i--
		b[i] = byte('0' + n%10)
	}
	return string(b[i:])
}
