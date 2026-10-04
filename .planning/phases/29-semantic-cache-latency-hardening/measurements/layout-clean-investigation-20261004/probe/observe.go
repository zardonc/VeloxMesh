package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const sampleInterval = time.Second
const quietSamples = 10
const minimumAvailable = 6 * 1024 * 1024 * 1024
const maximumSwapIn = 64 * 1024
const maximumSystemCPU = 15
const maximumWaitCPU = 5
const maximumPSI = 0.1

type sample struct {
	UTC       time.Time          `json:"utc"`
	Available uint64             `json:"available_bytes"`
	PSI       float64            `json:"psi_avg10"`
	CPU       []uint64           `json:"cpu_ticks"`
	VM        map[string]uint64  `json:"vm_counters"`
	Group     map[string]uint64  `json:"cgroup,omitempty"`
	Delta     map[string]float64 `json:"delta,omitempty"`
	Valid     bool               `json:"delta_valid"`
}

func counters(path string) (map[string]uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	values := map[string]uint64{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		number, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return nil, err
		}
		values[strings.TrimSuffix(fields[0], ":")] = number
	}
	return values, nil
}

func capture(pid int) (sample, error) {
	current := sample{UTC: time.Now().UTC()}
	mem, err := counters("/proc/meminfo")
	if err != nil {
		return current, err
	}
	current.Available = mem["MemAvailable"] * 1024
	current.VM, err = counters("/proc/vmstat")
	if err != nil {
		return current, err
	}
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return current, err
	}
	fields := strings.Fields(strings.SplitN(string(data), "\n", 2)[0])
	for _, field := range fields[1:9] {
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return current, err
		}
		current.CPU = append(current.CPU, value)
	}
	data, err = os.ReadFile("/proc/pressure/memory")
	if err != nil {
		return current, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "some ") {
			continue
		}
		for _, field := range strings.Fields(line) {
			if strings.HasPrefix(field, "avg10=") {
				current.PSI, err = strconv.ParseFloat(strings.TrimPrefix(field, "avg10="), 64)
				if err != nil {
					return current, err
				}
			}
		}
	}
	if pid > 0 {
		current.Group, err = groupCounters(pid)
		if err != nil {
			return current, err
		}
	}
	return current, nil
}

func groupCounters(pid int) (map[string]uint64, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", pid))
	if err != nil {
		return nil, err
	}
	suffix := strings.TrimSpace(strings.TrimPrefix(string(data), "0::"))
	if !strings.HasPrefix(suffix, "/") || strings.Contains(suffix, "..") {
		return nil, fmt.Errorf("unsafe cgroup path")
	}
	base := filepath.Join("/sys/fs/cgroup", suffix)
	values, err := counters(filepath.Join(base, "memory.stat"))
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"memory.current", "memory.swap.current"} {
		data, err := os.ReadFile(filepath.Join(base, name))
		if err != nil {
			return nil, err
		}
		value, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
		if err != nil {
			return nil, err
		}
		values[name] = value
	}
	events, err := counters(filepath.Join(base, "memory.events"))
	if err != nil {
		return nil, err
	}
	for name, value := range events {
		values["event_"+name] = value
	}
	return values, nil
}

func differences(current, previous sample) sample {
	if previous.UTC.IsZero() {
		return current
	}
	seconds := current.UTC.Sub(previous.UTC).Seconds()
	total := uint64(0)
	for index, value := range current.CPU {
		total += value - previous.CPU[index]
	}
	if total == 0 || seconds <= 0 {
		return current
	}
	current.Valid = true
	current.Delta = map[string]float64{"system_pct": 100 * float64(current.CPU[2]-previous.CPU[2]) / float64(total), "wait_pct": 100 * float64(current.CPU[4]-previous.CPU[4]) / float64(total), "swap_in_bytes_sec": float64(current.VM["pswpin"]-previous.VM["pswpin"]) * float64(os.Getpagesize()) / seconds, "swap_out_bytes_sec": float64(current.VM["pswpout"]-previous.VM["pswpout"]) * float64(os.Getpagesize()) / seconds, "read_kib_sec": float64(current.VM["pgpgin"]-previous.VM["pgpgin"]) / seconds}
	return current
}

func quietGate(config options) error {
	deadline := time.Now().Add(startupTimeout)
	var previous sample
	streak := 0
	for time.Now().Before(deadline) {
		current, err := capture(0)
		if err != nil {
			return err
		}
		current = differences(current, previous)
		quiet := current.Valid && current.Available >= minimumAvailable && current.PSI <= maximumPSI && current.Delta["swap_in_bytes_sec"] <= maximumSwapIn && current.Delta["swap_out_bytes_sec"] == 0 && current.Delta["system_pct"] < maximumSystemCPU && current.Delta["wait_pct"] < maximumWaitCPU
		if quiet {
			streak++
		} else {
			streak = 0
		}
		if err := emit(map[string]any{"type": "gate-sample", "sample": current, "quiet": quiet, "streak": streak}); err != nil {
			return err
		}
		if streak >= quietSamples {
			return emit(map[string]any{"type": "gate-pass", "utc": time.Now().UTC(), "streak": streak})
		}
		previous = current
		time.Sleep(sampleInterval)
	}
	return fmt.Errorf("low-pressure baseline failed within 120s")
}

func observe(config options) error {
	if config.Seconds < 1 || config.Seconds > 3600 || !strings.HasPrefix(config.StopFile, "/tmp/veloxmesh-phase29-layout-20261004-01a103bf/") {
		return fmt.Errorf("invalid observation bounds")
	}
	var previous sample
	deadline := time.Now().Add(time.Duration(config.Seconds) * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(config.StopFile); err == nil {
			return emit(map[string]any{"type": "observer-stop", "utc": time.Now().UTC()})
		} else if !os.IsNotExist(err) {
			return err
		}
		current, err := capture(config.PID)
		if err != nil {
			return err
		}
		current = differences(current, previous)
		if err := emit(map[string]any{"type": "resource-sample", "sample": current}); err != nil {
			return err
		}
		previous = current
		time.Sleep(sampleInterval)
	}
	return fmt.Errorf("observer deadline reached without stop marker")
}
