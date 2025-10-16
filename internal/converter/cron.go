package converter

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// IntervalToCron converts an interval in hours and a Unix timestamp to a cron expression
// The cron expression will be scheduled to run at the same minute and hour (for daily)
// as the reference timestamp, repeating at the specified interval.
//
// Supported intervals: 1, 2, 3, 6, 8, 12, 24 hours
//
// Examples:
//   - 24 hours: "15 10 * * *" (runs daily at 10:15 AM)
//   - 12 hours: "15 10,22 * * *" (runs at 10:15 AM and 10:15 PM)
//   - 6 hours: "15 4,10,16,22 * * *" (runs every 6 hours starting from the reference time)
//   - 1 hour: "15 * * * *" (runs every hour at minute 15)
func IntervalToCron(intervalHours int, unixTimestamp int64) (string, error) {
	// Validate interval
	validIntervals := map[int]bool{
		1: true, 2: true, 3: true, 6: true, 8: true, 12: true, 24: true,
	}
	if !validIntervals[intervalHours] {
		return "", fmt.Errorf("invalid interval: %d hours (must be 1, 2, 3, 6, 8, 12, or 24)", intervalHours)
	}

	// Parse the timestamp to get the reference time
	refTime := time.Unix(unixTimestamp, 0).UTC()
	minute := refTime.Minute()
	hour := refTime.Hour()

	switch intervalHours {
	case 1:
		// Every hour at the specified minute
		return fmt.Sprintf("%d * * * *", minute), nil

	case 2:
		// Every 2 hours starting from the reference hour
		// Generate hours: hour, hour+2, hour+4, etc. (mod 24)
		hours := make([]int, 0, 12)
		for h := hour; len(hours) < 12; h = (h + 2) % 24 {
			hours = append(hours, h)
		}
		return fmt.Sprintf("%d %s * * *", minute, formatHours(hours)), nil

	case 3:
		// Every 3 hours starting from the reference hour
		hours := make([]int, 0, 8)
		for h := hour; len(hours) < 8; h = (h + 3) % 24 {
			hours = append(hours, h)
		}
		return fmt.Sprintf("%d %s * * *", minute, formatHours(hours)), nil

	case 6:
		// Every 6 hours starting from the reference hour
		hours := make([]int, 0, 4)
		for h := hour; len(hours) < 4; h = (h + 6) % 24 {
			hours = append(hours, h)
		}
		return fmt.Sprintf("%d %s * * *", minute, formatHours(hours)), nil

	case 8:
		// Every 8 hours starting from the reference hour
		hours := make([]int, 0, 3)
		for h := hour; len(hours) < 3; h = (h + 8) % 24 {
			hours = append(hours, h)
		}
		return fmt.Sprintf("%d %s * * *", minute, formatHours(hours)), nil

	case 12:
		// Every 12 hours starting from the reference hour
		hours := []int{hour, (hour + 12) % 24}
		return fmt.Sprintf("%d %s * * *", minute, formatHours(hours)), nil

	case 24:
		// Once per day at the specified time
		return fmt.Sprintf("%d %d * * *", minute, hour), nil

	default:
		return "", fmt.Errorf("unsupported interval: %d hours", intervalHours)
	}
}

// formatHours converts a slice of hours into a comma-separated string
// and sorts them in ascending order
func formatHours(hours []int) string {
	if len(hours) == 0 {
		return "*"
	}

	// Sort hours (simple bubble sort for small arrays)
	sorted := make([]int, len(hours))
	copy(sorted, hours)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Build comma-separated string
	result := fmt.Sprintf("%d", sorted[0])
	for i := 1; i < len(sorted); i++ {
		result += fmt.Sprintf(",%d", sorted[i])
	}
	return result
}

// ParseBasicTimingToCron parses Airbyte's BasicTiming format and converts it to a cron expression
// BasicTiming format examples: "Every 24 HOURS", "Every 12 HOURS", "Every 6 HOURS", etc.
// The referenceTimestamp is used to determine the exact minute and hour when the cron should run.
func ParseBasicTimingToCron(basicTiming string, referenceTimestamp int64) (string, error) {
	// Parse the basic timing string
	// Expected format: "Every X HOURS" (case insensitive)
	parts := strings.Fields(strings.ToUpper(strings.TrimSpace(basicTiming)))

	if len(parts) < 3 {
		return "", fmt.Errorf("invalid BasicTiming format: %s (expected 'Every X HOURS')", basicTiming)
	}

	if parts[0] != "EVERY" {
		return "", fmt.Errorf("invalid BasicTiming format: %s (expected to start with 'Every')", basicTiming)
	}

	if parts[2] != "HOURS" && parts[2] != "HOUR" {
		return "", fmt.Errorf("invalid BasicTiming format: %s (expected unit to be 'HOURS')", basicTiming)
	}

	// Parse the interval number
	intervalHours, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", fmt.Errorf("invalid interval in BasicTiming: %s (error: %w)", parts[1], err)
	}

	// Use the existing IntervalToCron function
	return IntervalToCron(intervalHours, referenceTimestamp)
}
