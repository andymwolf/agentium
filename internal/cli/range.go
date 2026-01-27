package cli

import (
	"fmt"
	"strconv"
	"strings"
)

// ExpandRanges takes a slice of strings that may contain ranges (e.g., "1-5")
// and/or single numbers (e.g., "3") and expands them into a flat slice of
// number strings.
//
// Examples:
//   - ["1-5"] → ["1", "2", "3", "4", "5"]
//   - ["1", "3-5", "8"] → ["1", "3", "4", "5", "8"]
//   - ["1,3-5,8"] → ["1", "3", "4", "5", "8"] (handles comma-separated within single string)
func ExpandRanges(input []string) ([]string, error) {
	var result []string

	for _, item := range input {
		// Handle comma-separated values within a single string
		// (cobra's StringSlice already splits on commas, but this handles edge cases)
		segments := strings.Split(item, ",")

		for _, segment := range segments {
			segment = strings.TrimSpace(segment)
			if segment == "" {
				continue
			}

			expanded, err := expandSegment(segment)
			if err != nil {
				return nil, err
			}
			result = append(result, expanded...)
		}
	}

	return result, nil
}

// expandSegment handles a single segment which may be a number ("5") or a range ("1-5")
func expandSegment(segment string) ([]string, error) {
	// Check if this is a range (contains "-" between two numbers)
	if idx := strings.Index(segment, "-"); idx > 0 && idx < len(segment)-1 {
		startStr := strings.TrimSpace(segment[:idx])
		endStr := strings.TrimSpace(segment[idx+1:])

		start, err := strconv.Atoi(startStr)
		if err != nil {
			return nil, fmt.Errorf("invalid range %q: start value %q is not a valid number", segment, startStr)
		}

		end, err := strconv.Atoi(endStr)
		if err != nil {
			return nil, fmt.Errorf("invalid range %q: end value %q is not a valid number", segment, endStr)
		}

		if start > end {
			return nil, fmt.Errorf("invalid range %q: start (%d) is greater than end (%d)", segment, start, end)
		}

		var result []string
		for i := start; i <= end; i++ {
			result = append(result, strconv.Itoa(i))
		}
		return result, nil
	}

	// Single number
	_, err := strconv.Atoi(segment)
	if err != nil {
		return nil, fmt.Errorf("invalid value %q: not a valid number", segment)
	}

	return []string{segment}, nil
}
