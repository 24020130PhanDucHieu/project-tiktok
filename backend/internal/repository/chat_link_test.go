package repository

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestUnlinkedKocConditionTargetsKocThreadsWithAPhoneAndNoZaloId(t *testing.T) {
	c := unlinkedKocCondition()
	for _, k := range []string{"contactHandle", "contactPhone", "$or"} {
		if _, ok := c[k]; !ok {
			t.Fatalf("missing %q in %v", k, c)
		}
	}
	if len(c) != 3 {
		t.Fatalf("unexpected extra conditions: %v", c)
	}
	if or, ok := c["$or"].(bson.A); !ok || len(or) != 2 {
		t.Fatalf("a missing or empty Zalo id must both count as unlinked: %v", c["$or"])
	}
}
