package domain

import (
	"encoding/json"
	"testing"
)

func TestAnIDRoundTripsThroughItsHexForm(t *testing.T) {
	id := NewID()
	if id.IsZero() || len(id.Hex()) != 24 {
		t.Fatalf("bad id %q", id.Hex())
	}
	back, err := ParseID(id.Hex())
	if err != nil || back != id {
		t.Fatalf("round trip: %v %v", back, err)
	}
	if NewID() == id {
		t.Fatal("two ids must differ")
	}
}

func TestParseIDRefusesAnythingButTwentyFourHexChars(t *testing.T) {
	for _, s := range []string{"", "abc", "zzzzzzzzzzzzzzzzzzzzzzzz", "5f8d0d55b54764421b7156c3a", "5f8d0d55b54764421b7156c"} {
		if _, err := ParseID(s); err == nil {
			t.Fatalf("%q must be refused", s)
		}
	}
	if _, err := ParseID("5f8d0d55b54764421b7156c3"); err != nil {
		t.Fatal(err)
	}
}

func TestIDsAreUniqueUnderLoad(t *testing.T) {
	seen := map[ID]bool{}
	for i := 0; i < 100000; i++ {
		id := NewID()
		if seen[id] {
			t.Fatalf("duplicate id after %d", i)
		}
		seen[id] = true
	}
}

func TestIDJSONIsAHexStringLikeAnObjectIDWas(t *testing.T) {
	type doc struct {
		ID  ID `json:"_id"`
		Ref ID `json:"ref"`
	}
	id := NewID()
	raw, err := json.Marshal(doc{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"_id":"` + id.Hex() + `","ref":"000000000000000000000000"}`
	if string(raw) != want {
		t.Fatalf("got %s want %s", raw, want)
	}
	var d doc
	if err := json.Unmarshal([]byte(`{"_id":"`+id.Hex()+`","ref":null}`), &d); err != nil || d.ID != id || !d.Ref.IsZero() {
		t.Fatalf("%v %+v", err, d)
	}
	if err := json.Unmarshal([]byte(`{"_id":"nope"}`), &d); err == nil {
		t.Fatal("a bad id must be an error")
	}
}
