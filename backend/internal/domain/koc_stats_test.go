package domain

import (
	"encoding/json"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// The dashboard JSON and the aggregation documents use these exact names; moving the types must not rename them.
func TestDashboardShapesKeepTheirJSONAndBSONNames(t *testing.T) {
	check := func(v any, keys ...string) {
		t.Helper()
		raw, _ := json.Marshal(v)
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		for _, k := range keys {
			if _, ok := m[k]; !ok {
				t.Fatalf("%T: json field %q missing in %s", v, k, raw)
			}
		}
	}
	check(StageStat{}, "stage", "count", "videoGmv", "castFee")
	check(BatchFunnel{}, "campaign", "pipeline", "contacted", "replied", "agreed", "delivered", "posted", "gmv")
	check(TaskKoc{}, "id", "tiktokId", "displayName", "avatarUrl", "phone", "stage", "trackingCode")

	// the aggregation groups by "_id": it must still decode into Stage / Campaign / ID
	raw, _ := bson.Marshal(bson.M{"_id": "AGREED", "count": int64(3), "videoGmv": 1.5})
	var st StageStat
	if err := bson.Unmarshal(raw, &st); err != nil || st.Stage != "AGREED" || st.Count != 3 || st.VideoGmv != 1.5 {
		t.Fatalf("%+v %v", st, err)
	}
	raw, _ = bson.Marshal(bson.M{"_id": "Thang 11", "pipeline": int64(5)})
	var bf BatchFunnel
	if err := bson.Unmarshal(raw, &bf); err != nil || bf.Campaign != "Thang 11" || bf.Pipeline != 5 {
		t.Fatalf("%+v %v", bf, err)
	}
}
