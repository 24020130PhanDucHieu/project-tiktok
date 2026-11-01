//go:build !linux && !darwin

package metrics

func diskUsage(string) (float64, float64) { return 0, 0 }
