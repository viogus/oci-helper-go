//go:build !darwin

package system

import (
	"fmt"

	"github.com/shirou/gopsutil/v3/cpu"
)

// readCPUTimesFallback is only needed on darwin (where gopsutil's cpu.Times
// requires CGO). Other platforms work with the pure-Go gopsutil path.
func readCPUTimesFallback() ([]cpu.TimesStat, error) {
	return nil, fmt.Errorf("no fallback for this platform")
}

// readCPUPercentDirect is only used on darwin without CGO.
func readCPUPercentDirect() (user, system, idle float64, ok bool) {
	return 0, 0, 0, false
}
