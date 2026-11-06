package domain

// CollectionInfo describes one MongoDB collection.
type CollectionInfo struct {
	Name        string  `json:"name"`
	Count       int64   `json:"count"`
	SizeMB      float64 `json:"sizeMB"`
	StorageMB   float64 `json:"storageMB"`
	AvgObjBytes float64 `json:"avgObjBytes"`
	Indexes     int64   `json:"indexes"`
	IndexSizeMB float64 `json:"indexSizeMB"`
}

// DatabaseInfo is the MongoDB section of the monitoring page.
type DatabaseInfo struct {
	OK            bool             `json:"ok"`
	Error         string           `json:"error,omitempty"`
	PingMs        float64          `json:"pingMs"`
	Name          string           `json:"name"`
	ServerVersion string           `json:"serverVersion"`
	Limited       bool             `json:"limited"` // managed free tiers (e.g. Atlas M0) deny serverStatus
	UptimeSec     float64          `json:"uptimeSeconds"`
	DataSizeMB    float64          `json:"dataSizeMB"`
	StorageSizeMB float64          `json:"storageSizeMB"`
	IndexSizeMB   float64          `json:"indexSizeMB"`
	Connections   map[string]any   `json:"connections"`
	Opcounters    map[string]any   `json:"opcounters"`
	Memory        map[string]any   `json:"memory"`
	Cache         map[string]any   `json:"cache"`
	Network       map[string]any   `json:"network"`
	Collections   []CollectionInfo `json:"collections"`
}
