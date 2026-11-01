package metrics

import (
	"bufio"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// Host is a best-effort view of the machine (Linux /proc); fields stay zero elsewhere.
type Host struct {
	OS           string  `json:"os"`
	Hostname     string  `json:"hostname"`
	CPUs         int     `json:"cpus"`
	Load1        float64 `json:"load1"`
	Load5        float64 `json:"load5"`
	Load15       float64 `json:"load15"`
	MemTotalMB   float64 `json:"memTotalMB"`
	MemAvailMB   float64 `json:"memAvailableMB"`
	SwapTotalMB  float64 `json:"swapTotalMB"`
	SwapFreeMB   float64 `json:"swapFreeMB"`
	ProcessRSSMB float64 `json:"processRssMB"`
	UptimeSec    float64 `json:"uptimeSeconds"`
	DiskTotalGB  float64 `json:"diskTotalGB"`
	DiskFreeGB   float64 `json:"diskFreeGB"`
}

// ReadHost collects host metrics.
func ReadHost() Host {
	h := Host{OS: runtime.GOOS + "/" + runtime.GOARCH, CPUs: runtime.NumCPU()}
	h.Hostname, _ = os.Hostname()
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		f := strings.Fields(string(b))
		if len(f) >= 3 {
			h.Load1, _ = strconv.ParseFloat(f[0], 64)
			h.Load5, _ = strconv.ParseFloat(f[1], 64)
			h.Load15, _ = strconv.ParseFloat(f[2], 64)
		}
	}
	if b, err := os.ReadFile("/proc/uptime"); err == nil {
		if f := strings.Fields(string(b)); len(f) > 0 {
			h.UptimeSec, _ = strconv.ParseFloat(f[0], 64)
		}
	}
	mem := readKB("/proc/meminfo")
	h.MemTotalMB, h.MemAvailMB = mem["MemTotal"]/1024, mem["MemAvailable"]/1024
	h.SwapTotalMB, h.SwapFreeMB = mem["SwapTotal"]/1024, mem["SwapFree"]/1024
	h.ProcessRSSMB = readKB("/proc/self/status")["VmRSS"] / 1024
	h.DiskTotalGB, h.DiskFreeGB = diskUsage("/")
	return h
}

func readKB(path string) map[string]float64 {
	out := map[string]float64{}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		fields := strings.Fields(v)
		if len(fields) == 0 {
			continue
		}
		if n, err := strconv.ParseFloat(fields[0], 64); err == nil {
			out[k] = n
		}
	}
	return out
}
