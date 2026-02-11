package controller

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

const (
	// ResourceMonitorInterval is how often the resource monitor checks memory usage.
	ResourceMonitorInterval = 30 * time.Second

	// MemoryWarningPct is the memory usage percentage that triggers a warning log.
	MemoryWarningPct = 80

	// MemoryCriticalPct is the memory usage percentage that triggers an error log.
	MemoryCriticalPct = 90
)

// thresholdNone represents no threshold crossed.
const thresholdNone = 0

// readMemInfoFrom parses /proc/meminfo-formatted content from the given reader,
// returning MemTotal and MemAvailable in bytes.
func readMemInfoFrom(r io.Reader) (total, available uint64, err error) {
	var foundTotal, foundAvailable bool
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			val, parseErr := parseMemInfoLine(line)
			if parseErr != nil {
				return 0, 0, fmt.Errorf("parsing MemTotal: %w", parseErr)
			}
			total = val
			foundTotal = true
		} else if strings.HasPrefix(line, "MemAvailable:") {
			val, parseErr := parseMemInfoLine(line)
			if parseErr != nil {
				return 0, 0, fmt.Errorf("parsing MemAvailable: %w", parseErr)
			}
			available = val
			foundAvailable = true
		}
		if foundTotal && foundAvailable {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, fmt.Errorf("reading meminfo: %w", err)
	}
	if !foundTotal || !foundAvailable {
		return 0, 0, fmt.Errorf("missing required fields (MemTotal=%t, MemAvailable=%t)", foundTotal, foundAvailable)
	}
	return total, available, nil
}

// parseMemInfoLine extracts the value in bytes from a /proc/meminfo line like "MemTotal:  8000000 kB".
func parseMemInfoLine(line string) (uint64, error) {
	// Format: "FieldName:     <value> kB"
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return 0, fmt.Errorf("unexpected format: %q", line)
	}
	var val uint64
	if _, err := fmt.Sscanf(parts[1], "%d", &val); err != nil {
		return 0, fmt.Errorf("parsing value from %q: %w", line, err)
	}
	// /proc/meminfo reports in kB (1024 bytes)
	if len(parts) >= 3 && strings.EqualFold(parts[2], "kB") {
		val *= 1024
	}
	return val, nil
}

// readMemInfo reads /proc/meminfo and returns MemTotal and MemAvailable in bytes.
func readMemInfo() (total, available uint64, err error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = f.Close() }()
	return readMemInfoFrom(f)
}

// startResourceMonitor runs a background loop that periodically checks memory
// usage via /proc/meminfo and logs warnings when thresholds are crossed.
// It exits when ctx is cancelled.
func (c *Controller) startResourceMonitor(ctx context.Context) {
	// Initial read to detect already-pressured VMs
	lastThreshold := thresholdNone
	lastThreshold = c.checkMemory(lastThreshold)

	ticker := time.NewTicker(ResourceMonitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			lastThreshold = c.checkMemory(lastThreshold)
		}
	}
}

// checkMemory reads memory info and logs if a threshold is crossed.
// Returns the current threshold level for tracking state between calls.
func (c *Controller) checkMemory(lastThreshold int) int {
	total, available, err := readMemInfo()
	if err != nil {
		// Expected on non-Linux (macOS dev), log once then silently ignore
		c.logInfo("Resource monitor: /proc/meminfo unavailable (%v), disabling", err)
		return -1 // sentinel: caller should stop calling
	}

	if total == 0 {
		return lastThreshold
	}

	usedPct := int((total - available) * 100 / total)
	currentThreshold := thresholdNone
	if usedPct >= MemoryCriticalPct {
		currentThreshold = MemoryCriticalPct
	} else if usedPct >= MemoryWarningPct {
		currentThreshold = MemoryWarningPct
	}

	// Only log on threshold crossings to avoid spam
	if currentThreshold != lastThreshold {
		totalMB := total / (1024 * 1024)
		availMB := available / (1024 * 1024)
		switch {
		case currentThreshold == MemoryCriticalPct:
			c.logError("Memory usage CRITICAL: %d%% used (%d MB available / %d MB total)", usedPct, availMB, totalMB)
		case currentThreshold == MemoryWarningPct:
			c.logWarning("Memory usage HIGH: %d%% used (%d MB available / %d MB total)", usedPct, availMB, totalMB)
		case currentThreshold == thresholdNone && lastThreshold > thresholdNone:
			c.logInfo("Memory usage recovered: %d%% used (%d MB available / %d MB total)", usedPct, availMB, totalMB)
		}
	}

	return currentThreshold
}
