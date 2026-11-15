package service

import (
	"strings"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// RenderTemplate fills {ten} with the last word of the KOC's name, or "bạn" when the KOC has no name, and {kenh} with the
// TikTok channel with its @ (nothing when the KOC has no TikTok id).
func RenderTemplate(body string, koc *domain.Koc) string {
	name := "bạn"
	if words := strings.Fields(koc.DisplayName); len(words) > 0 {
		name = words[len(words)-1]
	}
	channel := ""
	if id := strings.TrimSpace(koc.TiktokID); id != "" {
		channel = "@" + strings.TrimPrefix(id, "@")
	}
	return strings.NewReplacer("{ten}", name, "{kenh}", channel).Replace(body)
}
