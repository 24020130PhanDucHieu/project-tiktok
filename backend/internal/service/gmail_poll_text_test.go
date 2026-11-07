package service

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateUTF8NeverSplitsAVietnameseCharacter(t *testing.T) {
	// "ệ" takes 3 bytes: 2000 of them are 6000 bytes, and byte 4000 falls in the middle of a character
	text := strings.Repeat("ệ", 2000)
	got := truncateUTF8(text, 4000)
	if !utf8.ValidString(got) {
		t.Fatal("the cut text must still be valid UTF-8 (MongoDB refuses a broken string)")
	}
	if len(got) > 4000 {
		t.Fatalf("the cut text is %d bytes, over the 4000 limit", len(got))
	}
	if !strings.HasPrefix(text, got) || len(got) < 4000-3 {
		t.Fatalf("only the last partial character may be dropped, kept %d bytes", len(got))
	}
}

func TestTruncateUTF8LeavesShortAndExactTextAlone(t *testing.T) {
	for _, s := range []string{"", "ngắn", strings.Repeat("a", 4000)} {
		if got := truncateUTF8(s, 4000); got != s {
			t.Fatalf("text within the limit must be kept as it is, got %d bytes of %d", len(got), len(s))
		}
	}
}
