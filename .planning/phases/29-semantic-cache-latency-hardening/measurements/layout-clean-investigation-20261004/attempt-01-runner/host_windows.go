package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"
)

var controlledKernel = syscall.NewLazyDLL("kernel32.dll")

type hostMemory struct {
	Length, Load                                                                         uint32
	Total, Available, PageTotal, PageAvailable, VirtualTotal, VirtualAvailable, Extended uint64
}

type hostCPU struct{ Idle, Total uint64 }

func sampleHost() (hostMemory, hostCPU, error) {
	memory := hostMemory{}
	memory.Length = uint32(unsafe.Sizeof(memory))
	if ok, _, err := controlledKernel.NewProc("GlobalMemoryStatusEx").Call(uintptr(unsafe.Pointer(&memory))); ok == 0 {
		return memory, hostCPU{}, err
	}
	var idle, kernel, user syscall.Filetime
	if ok, _, err := controlledKernel.NewProc("GetSystemTimes").Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user))); ok == 0 {
		return memory, hostCPU{}, err
	}
	value := func(t syscall.Filetime) uint64 { return uint64(t.HighDateTime)<<32 | uint64(t.LowDateTime) }
	return memory, hostCPU{Idle: value(idle), Total: value(kernel) + value(user)}, nil
}

func startHostMonitor() (func() error, error) {
	file, err := os.Create(filepath.Join(inventoryRoot, "host-resources.jsonl"))
	if err != nil {
		return nil, err
	}
	stop, done := make(chan struct{}), make(chan error, 1)
	go func() { done <- monitorHost(file, stop) }()
	return func() error { close(stop); return errors.Join(<-done, file.Close()) }, nil
}

func monitorHost(file *os.File, stop <-chan struct{}) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	encoder := json.NewEncoder(file)
	var previous hostCPU
	for {
		select {
		case <-stop:
			return nil
		case <-ticker.C:
			memory, cpu, err := sampleHost()
			if err != nil {
				return err
			}
			busy := 0.0
			if previous.Total != 0 && cpu.Total > previous.Total {
				busy = 100 * (1 - float64(cpu.Idle-previous.Idle)/float64(cpu.Total-previous.Total))
			}
			if err := encoder.Encode(map[string]any{"utc": time.Now().UTC(), "cpu_busy_pct": busy,
				"memory_load_pct": memory.Load, "memory_available_bytes": memory.Available,
				"memory_total_bytes": memory.Total}); err != nil {
				return err
			}
			previous = cpu
		}
	}
}
