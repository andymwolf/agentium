package cli

import (
	"reflect"
	"testing"
)

func TestExpandRanges(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		want    []string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "simple range",
			input:   []string{"1-5"},
			want:    []string{"1", "2", "3", "4", "5"},
			wantErr: false,
		},
		{
			name:    "single number",
			input:   []string{"42"},
			want:    []string{"42"},
			wantErr: false,
		},
		{
			name:    "multiple single numbers",
			input:   []string{"1", "3", "5"},
			want:    []string{"1", "3", "5"},
			wantErr: false,
		},
		{
			name:    "mixed ranges and numbers",
			input:   []string{"1", "3-5", "8"},
			want:    []string{"1", "3", "4", "5", "8"},
			wantErr: false,
		},
		{
			name:    "comma-separated mixed",
			input:   []string{"1,3-5,8"},
			want:    []string{"1", "3", "4", "5", "8"},
			wantErr: false,
		},
		{
			name:    "range with same start and end",
			input:   []string{"5-5"},
			want:    []string{"5"},
			wantErr: false,
		},
		{
			name:    "large range",
			input:   []string{"122-130"},
			want:    []string{"122", "123", "124", "125", "126", "127", "128", "129", "130"},
			wantErr: false,
		},
		{
			name:    "multiple comma-separated entries",
			input:   []string{"1,2", "5-7"},
			want:    []string{"1", "2", "5", "6", "7"},
			wantErr: false,
		},
		{
			name:    "whitespace handling",
			input:   []string{" 1 , 3-5 , 8 "},
			want:    []string{"1", "3", "4", "5", "8"},
			wantErr: false,
		},
		{
			name:    "empty input",
			input:   []string{},
			want:    nil,
			wantErr: false,
		},
		{
			name:    "empty string in input",
			input:   []string{""},
			want:    nil,
			wantErr: false,
		},
		// Error cases
		{
			name:    "invalid range - start greater than end",
			input:   []string{"5-2"},
			wantErr: true,
			errMsg:  "start (5) is greater than end (2)",
		},
		{
			name:    "invalid range - non-numeric start",
			input:   []string{"abc-5"},
			wantErr: true,
			errMsg:  "start value \"abc\" is not a valid number",
		},
		{
			name:    "invalid range - non-numeric end",
			input:   []string{"1-xyz"},
			wantErr: true,
			errMsg:  "end value \"xyz\" is not a valid number",
		},
		{
			name:    "invalid single value - non-numeric",
			input:   []string{"abc"},
			wantErr: true,
			errMsg:  "not a valid number",
		},
		{
			name:    "invalid mixed - one bad value",
			input:   []string{"1", "bad", "3"},
			wantErr: true,
			errMsg:  "not a valid number",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExpandRanges(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExpandRanges() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if err == nil {
					t.Error("ExpandRanges() expected error but got nil")
					return
				}
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("ExpandRanges() error = %q, want error containing %q", err.Error(), tt.errMsg)
				}
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExpandRanges() = %v, want %v", got, tt.want)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
