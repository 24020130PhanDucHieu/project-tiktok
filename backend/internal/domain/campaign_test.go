package domain

import (
	"strings"
	"testing"
)

func TestValidateSteps(t *testing.T) {
	img := NewID()
	long := strings.Repeat("a", MaxStepText+1)
	twelvePlusOne := make([]CampaignStep, MaxCampaignSteps+1)
	for i := range twelvePlusOne {
		twelvePlusOne[i] = CampaignStep{Kind: StepText, Text: "x"}
	}
	bad := map[string][]CampaignStep{
		"none":             nil,
		"too many":         twelvePlusOne,
		"blank text":       {{Kind: StepText, Text: "   "}},
		"text too long":    {{Kind: StepText, Text: long}},
		"image no id":      {{Kind: StepImage}},
		"caption too long": {{Kind: StepImage, ImageID: img, Caption: long}},
		"unknown kind":     {{Kind: "video", Text: "x"}},
	}
	for name, steps := range bad {
		if err := ValidateSteps(steps); err == nil {
			t.Errorf("%s: must be refused", name)
		}
	}
	if err := ValidateSteps([]CampaignStep{{Kind: StepText, Text: "Chào {ten}"}, {Kind: StepImage, ImageID: img}}); err != nil {
		t.Fatalf("a text and a picture are fine: %v", err)
	}
	if err := ValidateSteps([]CampaignStep{{Kind: StepText, Text: "  "}, {Kind: StepText, Text: "x"}}); err == nil || !strings.Contains(err.Error(), "số 1") {
		t.Fatalf("the message names the step: %v", err)
	}
}
