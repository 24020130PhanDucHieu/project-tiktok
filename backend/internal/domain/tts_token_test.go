package domain

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

// The stored authorization of the shop must keep these exact field names: renaming one would make the token already
// saved in MongoDB unreadable and force the shop to authorize again.
func TestTTSTokenKeepsTheStoredFieldNames(t *testing.T) {
	raw, err := bson.Marshal(TTSToken{ID: "default", Access: "a", Refresh: "r", AccessExpireAt: time.Now(), RefreshExpireAt: time.Now(),
		OpenID: "o", SellerName: "s", UpdatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	var m bson.M
	if err := bson.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"_id", "access", "refresh", "accessExpireAt", "refreshExpireAt", "openId", "sellerName", "updatedAt"} {
		if _, ok := m[key]; !ok {
			t.Fatalf("field %q is missing from the stored document: %v", key, m)
		}
	}
	if len(m) != 8 {
		t.Fatalf("the stored document must have exactly the 8 known fields, got %v", m)
	}
}
