package domain

import (
	"encoding/json"
	"testing"
)

// The system page of the web app reads these exact JSON names; moving the types must not rename them.
func TestDatabaseInfoKeepsTheNamesTheSystemPageReads(t *testing.T) {
	raw, err := json.Marshal(DatabaseInfo{Collections: []CollectionInfo{{Name: "kocs"}}})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"ok", "pingMs", "name", "serverVersion", "limited", "uptimeSeconds", "dataSizeMB", "storageSizeMB",
		"indexSizeMB", "connections", "opcounters", "memory", "cache", "network", "collections"} {
		if _, ok := m[key]; !ok {
			t.Fatalf("field %q is missing: %s", key, raw)
		}
	}
	cols, _ := m["collections"].([]any)
	first, _ := cols[0].(map[string]any)
	if first["name"] != "kocs" {
		t.Fatalf("collection entry: %v", first)
	}
}
