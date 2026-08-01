//go:build darwin

// CPU fallback for darwin. gopsutil's darwin implementation of cpu.Times()
// requires CGO (host_statistics), and the kern.cp_time sysctl was removed on
// modern macOS — so for the static CGO_ENABLED=0 build we parse the
// instantaneous percentages from `top -l 1` (the same "shell out" pattern
// gopsutil itself uses for vm_stat on darwin).
package system

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"

	"github.com/shirou/gopsutil/v3/cpu"
)

var cpuUsageRe = regexp.MustCompile(`CPU usage:\s*([0-9.]+)%\s*user,\s*([0-9.]+)%\s*sys,\s*([0-9.]+)%\s*idle`)

// readCPUTimesFallback: no cumulative tick source is available without CGO on
// darwin (kern.cp_time was removed; host_statistics needs libSystem), so we
// cannot produce a cumulative TimesStat.
func readCPUTimesFallback() ([]cpu.TimesStat, error) {
	return nil, fmt.Errorf("no cumulative cpu times without cgo on darwin")
}

// readCPUPercentDirect returns (user, system, idle) instantaneous percentages
// parsed from `top -l 1`.
func readCPUPercentDirect() (user, system, idle float64, ok bool) {
	out, err := exec.Command("top", "-l", "1", "-n", "0", "-s", "0").Output()
	if err != nil {
		return 0, 0, 0, false
	}
	m := cpuUsageRe.FindSubmatch(out)
	if m == nil {
		return 0, 0, 0, false
	}
	user, err1 := strconv.ParseFloat(string(m[1]), 64)
	system, err2 := strconv.ParseFloat(string(m[2]), 64)
	idle, err3 := strconv.ParseFloat(string(m[3]), 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, 0, 0, false
	}
	return user, system, idle, true
}
