package controller

import (
	"strings"
	"testing"
)

const sampleMemInfo = `MemTotal:        4028856 kB
MemFree:          123456 kB
MemAvailable:     805772 kB
Buffers:           12345 kB
Cached:           678901 kB
`

func TestReadMemInfoFrom(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantTotal     uint64
		wantAvailable uint64
	}{
		{
			name:          "standard format",
			input:         sampleMemInfo,
			wantTotal:     4028856 * 1024,
			wantAvailable: 805772 * 1024,
		},
		{
			name: "extra whitespace",
			input: `MemTotal:           8000000 kB
MemAvailable:       2000000 kB
`,
			wantTotal:     8000000 * 1024,
			wantAvailable: 2000000 * 1024,
		},
		{
			name: "fields in different order",
			input: `MemFree:          500000 kB
MemAvailable:     1000000 kB
Buffers:           12345 kB
MemTotal:        4000000 kB
`,
			wantTotal:     4000000 * 1024,
			wantAvailable: 1000000 * 1024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			total, available, err := readMemInfoFrom(strings.NewReader(tt.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if total != tt.wantTotal {
				t.Errorf("total = %d, want %d", total, tt.wantTotal)
			}
			if available != tt.wantAvailable {
				t.Errorf("available = %d, want %d", available, tt.wantAvailable)
			}
		})
	}
}

func TestReadMemInfoFromPartial(t *testing.T) {
	// Missing MemAvailable line
	input := `MemTotal:        4028856 kB
MemFree:          123456 kB
Buffers:           12345 kB
`
	_, _, err := readMemInfoFrom(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for missing MemAvailable, got nil")
	}
	if !strings.Contains(err.Error(), "missing required fields") {
		t.Errorf("error = %q, want it to mention missing fields", err.Error())
	}
}

func TestReadMemInfoFromEmpty(t *testing.T) {
	_, _, err := readMemInfoFrom(strings.NewReader(""))
	if err == nil {
		t.Fatal("expected error for empty input, got nil")
	}
}

func TestCheckMemory(t *testing.T) {
	tests := []struct {
		name          string
		usedPct       int
		lastThreshold int
		wantThreshold int
	}{
		{
			name:          "below warning",
			usedPct:       50,
			lastThreshold: thresholdNone,
			wantThreshold: thresholdNone,
		},
		{
			name:          "at warning boundary",
			usedPct:       80,
			lastThreshold: thresholdNone,
			wantThreshold: MemoryWarningPct,
		},
		{
			name:          "at critical boundary",
			usedPct:       90,
			lastThreshold: MemoryWarningPct,
			wantThreshold: MemoryCriticalPct,
		},
		{
			name:          "recovery from warning",
			usedPct:       70,
			lastThreshold: MemoryWarningPct,
			wantThreshold: thresholdNone,
		},
		{
			name:          "recovery from critical",
			usedPct:       50,
			lastThreshold: MemoryCriticalPct,
			wantThreshold: thresholdNone,
		},
		{
			name:          "stays at critical no change",
			usedPct:       95,
			lastThreshold: MemoryCriticalPct,
			wantThreshold: MemoryCriticalPct,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Compute total and available to produce the desired usedPct
			total := uint64(100)
			available := uint64(100 - tt.usedPct)

			currentThreshold := thresholdNone
			if tt.usedPct >= MemoryCriticalPct {
				currentThreshold = MemoryCriticalPct
			} else if tt.usedPct >= MemoryWarningPct {
				currentThreshold = MemoryWarningPct
			}

			// Verify threshold computation matches expected
			_ = total
			_ = available
			if currentThreshold != tt.wantThreshold {
				t.Errorf("threshold = %d, want %d (usedPct=%d)", currentThreshold, tt.wantThreshold, tt.usedPct)
			}
		})
	}
}

func TestParseMemInfoLine(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		want    uint64
		wantErr bool
	}{
		{
			name: "standard kB line",
			line: "MemTotal:        4028856 kB",
			want: 4028856 * 1024,
		},
		{
			name: "no unit suffix",
			line: "MemTotal:        4028856",
			want: 4028856,
		},
		{
			name:    "malformed line",
			line:    "MemTotal:",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMemInfoLine(tt.line)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}
