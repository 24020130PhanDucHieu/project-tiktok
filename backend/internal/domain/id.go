package domain

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"sync/atomic"
	"time"
)

// ID identifies a stored entity (KOC, conversation, message, user...). It has the same 12 bytes and the same 24
// character hex form as a MongoDB ObjectID, so documents already stored keep working untouched, but the domain does
// not depend on the database driver: the database package translates it (see internal/database/id_codec.go).
type ID [12]byte

// ErrBadID is returned for text that is not a 24 character hex id.
var ErrBadID = errors.New("mã không hợp lệ")

var (
	idProcess [5]byte
	idCounter atomic.Uint32
)

func init() {
	_, _ = rand.Read(idProcess[:])
	var c [4]byte
	_, _ = rand.Read(c[:])
	idCounter.Store(binary.BigEndian.Uint32(c[:]))
}

// NewID makes a new unique id: 4 bytes of time, 5 bytes unique to this process, 3 bytes of counter.
func NewID() ID {
	var id ID
	binary.BigEndian.PutUint32(id[0:4], uint32(time.Now().Unix()))
	copy(id[4:9], idProcess[:])
	n := idCounter.Add(1)
	id[9], id[10], id[11] = byte(n>>16), byte(n>>8), byte(n)
	return id
}

// ParseID reads the 24 character hex form.
func ParseID(s string) (ID, error) {
	var id ID
	if len(s) != 24 {
		return id, ErrBadID
	}
	if _, err := hex.Decode(id[:], []byte(s)); err != nil {
		return ID{}, ErrBadID
	}
	return id, nil
}

// Hex is the 24 character hex form.
func (id ID) Hex() string { return hex.EncodeToString(id[:]) }

// String is the hex form too, so ids print readably.
func (id ID) String() string { return id.Hex() }

// IsZero reports the empty id (also what makes `omitempty` drop it when stored).
func (id ID) IsZero() bool { return id == ID{} }

// Time is the moment the id was made.
func (id ID) Time() time.Time { return time.Unix(int64(binary.BigEndian.Uint32(id[0:4])), 0) }

// MarshalJSON writes the hex string, like an ObjectID did.
func (id ID) MarshalJSON() ([]byte, error) {
	return []byte(`"` + id.Hex() + `"`), nil
}

// UnmarshalJSON reads a hex string; null and "" give the empty id.
func (id *ID) UnmarshalJSON(b []byte) error {
	s := string(b)
	if s == "null" || s == `""` {
		*id = ID{}
		return nil
	}
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return ErrBadID
	}
	v, err := ParseID(s[1 : len(s)-1])
	if err != nil {
		return err
	}
	*id = v
	return nil
}
