// Package system collects host-level resource metrics (CPU, memory, disk,
// network) for the machine the binary runs on. This is the Go equivalent of
// the original Java project's oshi-based dashboard / Telegram "资源监控"
// (system metrics) feature.
package system

import (
	"math"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

// Snapshot is a point-in-time view of host resources.
type Snapshot struct {
	Hostname string      `json:"hostname"`
	OS       string      `json:"os"`
	Platform string      `json:"platform"`
	Arch     string      `json:"arch"`
	Uptime   uint64      `json:"uptime_seconds"`
	CPUCount int         `json:"cpu_count"`
	CPUModel string      `json:"cpu_model"`
	CPU      CPUInfo     `json:"cpu"`
	Memory   MemoryInfo  `json:"memory"`
	Disks    []DiskInfo  `json:"disks"`
	Network  NetworkInfo `json:"network"`
}

type CPUInfo struct {
	UsedPercent float64 `json:"used_percent"`
	User        float64 `json:"user_percent"`
	System      float64 `json:"system_percent"`
	Idle        float64 `json:"idle_percent"`
}

type MemoryInfo struct {
	Total       uint64  `json:"total_bytes"`
	Available   uint64  `json:"available_bytes"`
	Used        uint64  `json:"used_bytes"`
	UsedPercent float64 `json:"used_percent"`
}

type DiskInfo struct {
	Mount       string  `json:"mount"`
	FSType      string  `json:"fstype"`
	Total       uint64  `json:"total_bytes"`
	Used        uint64  `json:"used_bytes"`
	Free        uint64  `json:"free_bytes"`
	UsedPercent float64 `json:"used_percent"`
}

type NetworkInfo struct {
	BytesSent  uint64  `json:"bytes_sent"`
	BytesRecv  uint64  `json:"bytes_recv"`
	TxRateKBps float64 `json:"tx_rate_kbps"`
	RxRateKBps float64 `json:"rx_rate_kbps"`
}

// pseudo filesystems that carry no meaningful "disk usage" number.
var skipFSTypes = map[string]bool{
	"proc": true, "sysfs": true, "devtmpfs": true, "devpts": true,
	"tmpfs": true, "ramfs": true, "cgroup": true, "cgroup2": true,
	"mqueue": true, "debugfs": true, "tracefs": true, "securityfs": true,
	"fusectl": true, "autofs": true, "binfmt_misc": true, "pstore": true,
	"efivarfs": true, "hugetlbfs": true, "configfs": true, "selinuxfs": true,
	"rpc_pipefs": true, "nsfs": true,
	// NOTE: overlay (Docker) is intentionally NOT skipped — it is a real mount.
}

var (
	cpuMu       sync.Mutex
	lastCPUTime cpu.TimesStat
	lastCPUSeen time.Time

	netMu       sync.Mutex
	lastNetIO   net.IOCountersStat
	lastNetSeen time.Time
)

// Collect returns a fresh host metrics snapshot. CPU utilization is computed
// from tick deltas (like the Java original) so it is meaningful without a
// blocking sampling interval; network rates are deltas against the previous
// call.
func Collect() (*Snapshot, error) {
	snap := &Snapshot{}

	if hi, err := host.Info(); err == nil {
		snap.Hostname = hi.Hostname
		snap.OS = hi.OS
		snap.Platform = hi.Platform
		snap.Arch = hi.KernelArch
		snap.Uptime = hi.Uptime
	}
	if n, err := cpu.Counts(true); err == nil {
		snap.CPUCount = n
	}
	if infos, err := cpu.Info(); err == nil && len(infos) > 0 {
		snap.CPUModel = infos[0].ModelName
	}

	// CPU percent via tick delta (first call returns 0).
	// gopsutil cpu.Times on darwin needs CGO; when the static (CGO_ENABLED=0)
	// build hits ErrNotImplementedError, fall back to reading instantaneous
	// percentages via the platform fallback.
	cpuMu.Lock()
	gotTicks := false
	if times, err := readCPUTimes(); err == nil && len(times) > 0 {
		gotTicks = true
		now := time.Now()
		if !lastCPUSeen.IsZero() {
			t := times[0]
			last := lastCPUTime
			idle := t.Idle - last.Idle
			busy := (t.User + t.System + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal) -
				(last.User + last.System + last.Nice + last.Iowait + last.Irq + last.Softirq + last.Steal)
			all := busy + idle
			if all > 0 {
				snap.CPU.UsedPercent = round2(busy / all * 100)
				snap.CPU.User = round2((t.User - last.User) / all * 100)
				snap.CPU.System = round2((t.System - last.System) / all * 100)
				snap.CPU.Idle = round2(idle / all * 100)
			}
		}
		lastCPUTime = times[0]
		lastCPUSeen = now
	}
	if !gotTicks {
		if user, sys, idle, ok := readCPUPercentDirect(); ok {
			snap.CPU = CPUInfo{
				UsedPercent: round2(user + sys),
				User:        round2(user),
				System:      round2(sys),
				Idle:        round2(idle),
			}
		}
	}
	cpuMu.Unlock()

	if vm, err := mem.VirtualMemory(); err == nil {
		snap.Memory = MemoryInfo{
			Total:       vm.Total,
			Available:   vm.Available,
			Used:        vm.Used,
			UsedPercent: round2(vm.UsedPercent),
		}
	}

	if parts, err := disk.Partitions(false); err == nil {
		for _, p := range parts {
			if skipFSTypes[p.Fstype] {
				continue
			}
			if isPseudoMount(p.Mountpoint) {
				continue
			}
			u, err := disk.Usage(p.Mountpoint)
			if err != nil {
				continue
			}
			snap.Disks = append(snap.Disks, DiskInfo{
				Mount:       p.Mountpoint,
				FSType:      p.Fstype,
				Total:       u.Total,
				Used:        u.Used,
				Free:        u.Free,
				UsedPercent: round2(u.UsedPercent),
			})
			if len(snap.Disks) >= 8 {
				break
			}
		}
	}

	// Network totals + rates (KB/s deltas).
	netMu.Lock()
	if io, err := net.IOCounters(false); err == nil && len(io) > 0 {
		now := time.Now()
		snap.Network.BytesSent = io[0].BytesSent
		snap.Network.BytesRecv = io[0].BytesRecv
		if !lastNetSeen.IsZero() {
			dt := now.Sub(lastNetSeen).Seconds()
			if dt > 0 {
				snap.Network.TxRateKBps = round2(float64(io[0].BytesSent-lastNetIO.BytesSent) / 1024 / dt)
				snap.Network.RxRateKBps = round2(float64(io[0].BytesRecv-lastNetIO.BytesRecv) / 1024 / dt)
			}
		}
		lastNetIO = io[0]
		lastNetSeen = now
	}
	netMu.Unlock()

	return snap, nil
}

// readCPUTimes returns total CPU tick counts, preferring gopsutil and falling
// back to the platform-specific sysctl reader when needed.
func readCPUTimes() ([]cpu.TimesStat, error) {
	if times, err := cpu.Times(false); err == nil && len(times) > 0 {
		return times, nil
	}
	return readCPUTimesFallback()
}

func isPseudoMount(mount string) bool {
	switch mount {
	case "/proc", "/sys", "/dev", "/run", "/etc", "/lib", "/bin", "/sbin", "/usr", "/boot", "/snap":
		return true
	}
	return false
}

func round2(f float64) float64 {
	return math.Round(f*100) / 100
}
