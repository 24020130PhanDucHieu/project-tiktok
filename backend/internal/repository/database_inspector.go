package repository

import (
	"context"
	"sort"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// DatabaseInspector reads the MongoDB health and size statistics shown on the system page.
type DatabaseInspector struct{ db *mongo.Database }

// NewDatabaseInspector builds a DatabaseInspector.
func NewDatabaseInspector(db *mongo.Database) *DatabaseInspector { return &DatabaseInspector{db: db} }

// Inspect gathers the statistics; a failure is reported inside the result, never as an error.
func num(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	}
	return 0
}

func sub(m bson.M, key string) bson.M {
	if v, ok := m[key].(bson.M); ok {
		return v
	}
	return bson.M{}
}

func (r *DatabaseInspector) Inspect(ctx context.Context) domain.DatabaseInfo {
	d := domain.DatabaseInfo{OK: true, Name: r.db.Name(), Connections: map[string]any{}, Opcounters: map[string]any{},
		Memory: map[string]any{}, Cache: map[string]any{}, Network: map[string]any{}, Collections: []domain.CollectionInfo{}}
	start := time.Now()
	if err := r.db.Client().Ping(ctx, nil); err != nil {
		d.OK, d.Error = false, err.Error()
		return d
	}
	d.PingMs = float64(time.Since(start).Microseconds()) / 1000

	var st bson.M
	if err := r.db.RunCommand(ctx, bson.D{{Key: "dbStats", Value: 1}}).Decode(&st); err == nil {
		d.DataSizeMB, d.StorageSizeMB, d.IndexSizeMB = toMB(st["dataSize"]), toMB(st["storageSize"]), toMB(st["indexSize"])
	}
	var ss bson.M
	if err := r.db.Client().Database("admin").RunCommand(ctx, bson.D{{Key: "serverStatus", Value: 1}}).Decode(&ss); err == nil {
		d.ServerVersion, _ = ss["version"].(string)
		d.UptimeSec = num(ss["uptime"])
		c := sub(ss, "connections")
		d.Connections = map[string]any{"current": num(c["current"]), "available": num(c["available"]), "totalCreated": num(c["totalCreated"])}
		o := sub(ss, "opcounters")
		d.Opcounters = map[string]any{"insert": num(o["insert"]), "query": num(o["query"]), "update": num(o["update"]),
			"delete": num(o["delete"]), "getmore": num(o["getmore"]), "command": num(o["command"])}
		m := sub(ss, "mem")
		d.Memory = map[string]any{"residentMB": num(m["resident"]), "virtualMB": num(m["virtual"])}
		wc := sub(sub(ss, "wiredTiger"), "cache")
		d.Cache = map[string]any{
			"usedMB":  num(wc["bytes currently in the cache"]) / 1048576,
			"maxMB":   num(wc["maximum bytes configured"]) / 1048576,
			"dirtyMB": num(wc["tracked dirty bytes in the cache"]) / 1048576,
		}
		n := sub(ss, "network")
		d.Network = map[string]any{"inMB": num(n["bytesIn"]) / 1048576, "outMB": num(n["bytesOut"]) / 1048576, "requests": num(n["numRequests"])}
	} else {
		d.Limited = true
		var bi bson.M
		if err := r.db.RunCommand(ctx, bson.D{{Key: "buildInfo", Value: 1}}).Decode(&bi); err == nil {
			d.ServerVersion, _ = bi["version"].(string)
		}
	}

	names, _ := r.db.ListCollectionNames(ctx, bson.M{})
	sort.Strings(names)
	for _, name := range names {
		ci := domain.CollectionInfo{Name: name}
		var cs bson.M
		if err := r.db.RunCommand(ctx, bson.D{{Key: "collStats", Value: name}}).Decode(&cs); err == nil {
			ci.Count, ci.SizeMB, ci.StorageMB = int64(num(cs["count"])), toMB(cs["size"]), toMB(cs["storageSize"])
			ci.AvgObjBytes, ci.Indexes, ci.IndexSizeMB = num(cs["avgObjSize"]), int64(num(cs["nindexes"])), toMB(cs["totalIndexSize"])
		} else if n, err := r.db.Collection(name).EstimatedDocumentCount(ctx); err == nil {
			ci.Count = n
		}
		d.Collections = append(d.Collections, ci)
	}
	return d
}

func toMB(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n / 1048576
	case int32:
		return float64(n) / 1048576
	case int64:
		return float64(n) / 1048576
	}
	return 0
}
