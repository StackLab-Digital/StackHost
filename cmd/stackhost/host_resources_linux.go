//go:build linux

package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func readHostMemory() (*uint64, *uint64) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return nil, nil
	}
	defer file.Close()
	var total, available uint64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		value, _ := strconv.ParseUint(fields[1], 10, 64)
		switch fields[0] {
		case "MemTotal:":
			total = value * 1024
		case "MemAvailable:":
			available = value * 1024
		}
	}
	if total == 0 {
		return nil, nil
	}
	used := total - available
	return &used, &total
}

func readHostUptime() *uint64 {
	file, err := os.Open("/proc/uptime")
	if err != nil {
		return nil
	}
	defer file.Close()
	var seconds float64
	if _, err := fmt.Fscan(file, &seconds); err != nil {
		return nil
	}
	value := uint64(seconds)
	return &value
}

func readCPUPercent(ctx context.Context) *float64 {
	first, ok := readCPUStat()
	if !ok {
		return nil
	}
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		return nil
	}
	second, ok := readCPUStat()
	if !ok {
		return nil
	}
	total, idle := second.total-first.total, second.idle-first.idle
	if total == 0 || idle > total {
		return nil
	}
	value := float64(total-idle) / float64(total) * 100
	return &value
}

type cpuStat struct{ total, idle uint64 }

func readCPUStat() (cpuStat, bool) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return cpuStat{}, false
	}
	defer file.Close()
	var label string
	var user, nice, system, idle, wait, irq, softIRQ, steal uint64
	if _, err := fmt.Fscan(file, &label, &user, &nice, &system, &idle, &wait, &irq, &softIRQ, &steal); err != nil || label != "cpu" {
		return cpuStat{}, false
	}
	return cpuStat{total: user + nice + system + idle + wait + irq + softIRQ + steal, idle: idle + wait}, true
}
