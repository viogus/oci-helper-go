package system

import (
	"testing"

	"github.com/shirou/gopsutil/v3/cpu"
)

func TestRound2(t *testing.T) {
	cases := []struct {
		in   float64
		want float64
	}{
		{0, 0},
		{1.234, 1.23},
		{1.235, 1.24},
		{-1.236, -1.24},
	}
	for _, c := range cases {
		if got := round2(c.in); got != c.want {
			t.Errorf("round2(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

// cpuDeltaPercent mirrors the delta math inside Collect so the tick-based
// utilization calculation is unit-testable without a real kernel.
func cpuDeltaPercent(prev, cur cpu.TimesStat) (used, user, system, idle float64) {
	idleDelta := cur.Idle - prev.Idle
	busy := (cur.User + cur.System + cur.Nice + cur.Iowait + cur.Irq + cur.Softirq + cur.Steal) -
		(prev.User + prev.System + prev.Nice + prev.Iowait + prev.Irq + prev.Softirq + prev.Steal)
	all := busy + idleDelta
	if all <= 0 {
		return 0, 0, 0, 0
	}
	return round2(busy / all * 100),
		round2((cur.User - prev.User) / all * 100),
		round2((cur.System - prev.System) / all * 100),
		round2(idleDelta / all * 100)
}

func TestCPUDeltaPercent(t *testing.T) {
	// 10s window: 2s busy (1.5 user + 0.5 sys), 8s idle.
	prev := cpu.TimesStat{User: 0, System: 0, Nice: 0, Idle: 0}
	cur := cpu.TimesStat{User: 1.5, System: 0.5, Nice: 0, Idle: 8}
	used, user, system, idle := cpuDeltaPercent(prev, cur)
	if used != 20 {
		t.Errorf("used = %v, want 20", used)
	}
	if user != 15 || system != 5 || idle != 80 {
		t.Errorf("user/sys/idle = %v/%v/%v, want 15/5/80", user, system, idle)
	}

	// Zero delta (no time passed) must not divide by zero.
	used, user, system, idle = cpuDeltaPercent(cur, cur)
	if used != 0 || user != 0 || system != 0 || idle != 0 {
		t.Errorf("zero-delta = %v/%v/%v/%v, want all 0", used, user, system, idle)
	}
}

func TestCPUUsageRegex(t *testing.T) {
	// Mirrors darwin `top -l 1` output format.
	sample := "Processes: 421 total, 5 running, 416 sleeping...\n" +
		"CPU usage: 5.55% user, 12.34% sys, 82.10% idle\n" +
		"PhysMem: 11G used"
	m := cpuUsageRe.FindStringSubmatch(sample)
	if m == nil || len(m) != 4 {
		t.Fatalf("regex did not match sample: %v", m)
	}
	if m[1] != "5.55" || m[2] != "12.34" || m[3] != "82.10" {
		t.Errorf("captures = %v, want 5.55/12.34/82.10", m[1:])
	}
}
