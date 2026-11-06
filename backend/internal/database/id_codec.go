package database

import (
	"fmt"
	"reflect"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsoncodec"
	"go.mongodb.org/mongo-driver/bson/bsonrw"
	"go.mongodb.org/mongo-driver/bson/bsontype"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// Registry is the BSON registry of the application: domain.ID is stored as a MongoDB ObjectID, exactly what the
// documents already in the database hold, so the domain model needs no knowledge of the driver.
func Registry() *bsoncodec.Registry {
	reg := bson.NewRegistry()
	reg.RegisterTypeEncoder(reflect.TypeOf(domain.ID{}), bsoncodec.ValueEncoderFunc(encodeID))
	reg.RegisterTypeDecoder(reflect.TypeOf(domain.ID{}), bsoncodec.ValueDecoderFunc(decodeID))
	return reg
}

func encodeID(_ bsoncodec.EncodeContext, vw bsonrw.ValueWriter, val reflect.Value) error {
	id, ok := val.Interface().(domain.ID)
	if !ok {
		return fmt.Errorf("id codec: unexpected type %s", val.Type())
	}
	return vw.WriteObjectID(primitive.ObjectID(id))
}

func decodeID(_ bsoncodec.DecodeContext, vr bsonrw.ValueReader, val reflect.Value) error {
	if !val.CanSet() {
		return fmt.Errorf("id codec: cannot set %s", val.Type())
	}
	switch vr.Type() {
	case bsontype.ObjectID:
		oid, err := vr.ReadObjectID()
		if err != nil {
			return err
		}
		val.Set(reflect.ValueOf(domain.ID(oid)))
	case bsontype.String: // ids written as text by hand
		s, err := vr.ReadString()
		if err != nil {
			return err
		}
		id, err := domain.ParseID(s)
		if err != nil {
			return err
		}
		val.Set(reflect.ValueOf(id))
	case bsontype.Null:
		val.Set(reflect.ValueOf(domain.ID{}))
		return vr.ReadNull()
	case bsontype.Undefined:
		val.Set(reflect.ValueOf(domain.ID{}))
		return vr.ReadUndefined()
	default:
		return fmt.Errorf("id codec: cannot read %s as an id", vr.Type())
	}
	return nil
}
