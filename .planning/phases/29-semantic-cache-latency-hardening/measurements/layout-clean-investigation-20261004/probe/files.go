package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

const fixtureMount = "/fixture"
const oldTemporaryBinary = "/tmp/veloxmesh-phase29-acceptance-20261001.test"
const residencyWindow = 64 * 1024 * 1024
const scopeCount = 323

type evictionTotals struct {
	Files  int   `json:"files"`
	Bytes  int64 `json:"logical_bytes"`
	Before int64 `json:"resident_before_bytes"`
	After  int64 `json:"resident_after_bytes"`
}

func cleanupTemporary(config options) error {
	info, err := os.Lstat(oldTemporaryBinary)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Getuid()) || !info.Mode().IsRegular() {
		return fmt.Errorf("unsafe temporary-file owner or type")
	}
	processes, err := os.ReadDir("/proc")
	if err != nil {
		return err
	}
	for _, process := range processes {
		executable, _ := os.Readlink(filepath.Join("/proc", process.Name(), "exe"))
		if executable == oldTemporaryBinary {
			return fmt.Errorf("temporary binary still executing in PID %s", process.Name())
		}
	}
	file, err := os.Open(oldTemporaryBinary)
	if err != nil {
		return err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, file)
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	if digest != config.ExpectedDigest {
		return fmt.Errorf("old binary digest does not match retained evidence")
	}
	if err := os.Remove(oldTemporaryBinary); err != nil {
		return err
	}
	return emit(map[string]any{"type": "cleanup", "path": oldTemporaryBinary, "bytes": info.Size(), "sha256": digest, "removed": true})
}

func evictVolume() error {
	var totals evictionTotals
	err := filepath.WalkDir(fixtureMount, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in fixture; refusing traversal")
		}
		result, err := evictFile(path)
		if err != nil {
			return fmt.Errorf("fixture page eviction: %w", err)
		}
		totals.Files++
		totals.Bytes += result.Bytes
		totals.Before += result.Before
		totals.After += result.After
		return nil
	})
	if err != nil {
		return err
	}
	if err := emit(map[string]any{"type": "eviction", "totals": totals, "verified": totals.After == 0}); err != nil {
		return err
	}
	if totals.After != 0 {
		return fmt.Errorf("fixture cache still has %d resident bytes", totals.After)
	}
	return nil
}

func evictFile(path string) (evictionTotals, error) {
	file, err := os.Open(path)
	if err != nil {
		return evictionTotals{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return evictionTotals{}, err
	}
	if !info.Mode().IsRegular() {
		return evictionTotals{}, fmt.Errorf("nonregular fixture file")
	}
	before, err := residentBytes(file, info.Size())
	if err != nil {
		return evictionTotals{}, err
	}
	if err := file.Sync(); err != nil {
		return evictionTotals{}, err
	}
	page := int64(os.Getpagesize())
	aligned := (info.Size() + page - 1) / page * page
	if err := unix.Fadvise(int(file.Fd()), 0, aligned, unix.FADV_DONTNEED); err != nil {
		return evictionTotals{}, err
	}
	after, err := residentBytes(file, info.Size())
	return evictionTotals{Bytes: info.Size(), Before: before, After: after}, err
}

func residentBytes(file *os.File, size int64) (int64, error) {
	var resident int64
	for offset := int64(0); offset < size; offset += residencyWindow {
		length := min(int64(residencyWindow), size-offset)
		mapping, err := unix.Mmap(int(file.Fd()), offset, int(length), unix.PROT_NONE, unix.MAP_SHARED)
		if err != nil {
			return 0, err
		}
		pages := make([]byte, (length+int64(os.Getpagesize())-1)/int64(os.Getpagesize()))
		_, _, errno := unix.Syscall(unix.SYS_MINCORE, uintptr(unsafe.Pointer(&mapping[0])), uintptr(length), uintptr(unsafe.Pointer(&pages[0])))
		runtime.KeepAlive(mapping)
		runtime.KeepAlive(pages)
		unmapErr := unix.Munmap(mapping)
		if errno != 0 {
			return 0, errno
		}
		if unmapErr != nil {
			return 0, unmapErr
		}
		for _, page := range pages {
			if page&1 != 0 {
				resident += int64(os.Getpagesize())
			}
		}
	}
	return resident, nil
}

func scope(index int) string { return fmt.Sprintf("scope-%03d", index%scopeCount) }
func collection(config options, index int) string {
	if config.Collections == 1 {
		return "layout_shared"
	}
	return "layout_" + strings.ReplaceAll(scope(index), "-", "_")
}
