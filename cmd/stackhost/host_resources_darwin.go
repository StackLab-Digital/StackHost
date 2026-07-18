//go:build darwin

package main

import (
	"context"
	"encoding/binary"
	"syscall"
)

func readHostMemory() (*uint64, *uint64) {
	total, err := sysctlUint64("hw.memsize")
	if err != nil || total == 0 {
		return nil, nil
	}
	pageSize, err := sysctlUint64("hw.pagesize")
	if err != nil || pageSize == 0 {
		return nil, &total
	}
	free := uint64(0)
	for _, key := range []string{"vm.page_free_count", "vm.page_speculative_count", "vm.page_purgeable_count"} {
		if value, err := sysctlUint64(key); err == nil {
			free += value * pageSize
		}
	}
	used := total - minUint64(free, total)
	return &used, &total
}

func readHostUptime() *uint64                 { return nil }
func readCPUPercent(context.Context) *float64 { return nil }
func minUint64(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

func sysctlUint64(name string) (uint64, error) {
	data, err := syscall.Sysctl(name)
	if err != nil || len(data) < 8 {
		return 0, err
	}
	return binary.LittleEndian.Uint64([]byte(data[:8])), nil
}
