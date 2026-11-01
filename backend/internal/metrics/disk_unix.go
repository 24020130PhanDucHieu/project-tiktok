//go:build linux || darwin

package metrics

import "syscall"

func diskUsage(path string) (totalGB, freeGB float64) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0
	}
	bs := float64(st.Bsize)
	return float64(st.Blocks) * bs / 1e9, float64(st.Bavail) * bs / 1e9
}
