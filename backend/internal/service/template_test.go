package service

import (
	"testing"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

func TestRenderTemplateUsesTheLastWordOfTheName(t *testing.T) {
	koc := &domain.Koc{DisplayName: "Nguyễn Thị Lan"}
	if got := RenderTemplate("Chào {ten}, {ten} ơi", koc); got != "Chào Lan, Lan ơi" {
		t.Fatalf("every {ten} becomes the last word of the name, got %q", got)
	}
}

func TestRenderTemplateFallsBackToBanWhenTheNameIsEmpty(t *testing.T) {
	koc := &domain.Koc{DisplayName: "  "}
	if got := RenderTemplate("Chào {ten}!", koc); got != "Chào bạn!" {
		t.Fatalf("a KOC without a name is greeted as \"bạn\", not \"Chào !\", got %q", got)
	}
}

func TestRenderTemplateLeavesATemplateWithoutTheTagUntouched(t *testing.T) {
	if got := RenderTemplate("Xin chào", &domain.Koc{DisplayName: "Lan"}); got != "Xin chào" {
		t.Fatalf("a template without {ten} is sent as it is, got %q", got)
	}
}
