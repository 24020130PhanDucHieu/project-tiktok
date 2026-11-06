package database

import (
	"bytes"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

type newDoc struct {
	ID    domain.ID   `bson:"_id,omitempty"`
	Ref   domain.ID   `bson:"ref"`
	List  []domain.ID `bson:"list"`
	Name  string      `bson:"name"`
	Maybe domain.ID   `bson:"maybe,omitempty"`
	At    time.Time   `bson:"at"`
}

type oldDoc struct {
	ID    primitive.ObjectID   `bson:"_id,omitempty"`
	Ref   primitive.ObjectID   `bson:"ref"`
	List  []primitive.ObjectID `bson:"list"`
	Name  string               `bson:"name"`
	Maybe primitive.ObjectID   `bson:"maybe,omitempty"`
	At    time.Time            `bson:"at"`
}

// The documents already in the database were written with ObjectIDs. The new model must write the same bytes and read
// the old ones, otherwise the refactoring would corrupt or hide existing data.
func TestDomainIDIsStoredExactlyLikeAnObjectID(t *testing.T) {
	id1, id2 := domain.NewID(), domain.NewID()
	at := time.Unix(1700000000, 0).UTC()
	newBytes, err := bson.MarshalWithRegistry(Registry(), newDoc{ID: id1, Ref: id2, List: []domain.ID{id1, id2}, Name: "x", At: at})
	if err != nil {
		t.Fatal(err)
	}
	oldBytes, err := bson.Marshal(oldDoc{ID: primitive.ObjectID(id1), Ref: primitive.ObjectID(id2), List: []primitive.ObjectID{primitive.ObjectID(id1), primitive.ObjectID(id2)}, Name: "x", At: at})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(newBytes, oldBytes) {
		t.Fatalf("different bytes:\nnew %x\nold %x", newBytes, oldBytes)
	}
}

func TestOldDocumentsReadIntoDomainIDs(t *testing.T) {
	oid := primitive.NewObjectID()
	raw, err := bson.Marshal(oldDoc{ID: oid, Ref: oid, List: []primitive.ObjectID{oid}, Name: "old", At: time.Unix(1700000000, 0).UTC()})
	if err != nil {
		t.Fatal(err)
	}
	var d newDoc
	if err := bson.UnmarshalWithRegistry(Registry(), raw, &d); err != nil {
		t.Fatal(err)
	}
	if d.ID != domain.ID(oid) || d.Ref != domain.ID(oid) || len(d.List) != 1 || d.List[0] != domain.ID(oid) || d.Name != "old" {
		t.Fatalf("%+v", d)
	}
}

func TestAnEmptyIDIsLeftOutWhenOmitemptyAndNullReadsAsEmpty(t *testing.T) {
	raw, err := bson.MarshalWithRegistry(Registry(), newDoc{Name: "x"})
	if err != nil {
		t.Fatal(err)
	}
	var m bson.M
	if err := bson.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if _, has := m["_id"]; has {
		t.Fatal("omitempty must drop the empty _id so the database makes one")
	}
	if _, has := m["maybe"]; has {
		t.Fatal("omitempty must drop an empty optional id")
	}
	nullRaw, _ := bson.Marshal(bson.M{"_id": nil, "ref": nil, "name": "n"})
	var d newDoc
	if err := bson.UnmarshalWithRegistry(Registry(), nullRaw, &d); err != nil || !d.ID.IsZero() || !d.Ref.IsZero() {
		t.Fatalf("null must read as the empty id: %v %+v", err, d)
	}
}

func TestIDsInFiltersEncodeAsObjectIDs(t *testing.T) {
	id := domain.NewID()
	got, err := bson.MarshalWithRegistry(Registry(), bson.M{"_id": bson.M{"$in": []domain.ID{id}}})
	if err != nil {
		t.Fatal(err)
	}
	want, _ := bson.Marshal(bson.M{"_id": bson.M{"$in": []primitive.ObjectID{primitive.ObjectID(id)}}})
	if !bytes.Equal(got, want) {
		t.Fatalf("filter bytes differ:\n%x\n%x", got, want)
	}
}
