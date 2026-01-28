package converter

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// IntervalToCron converts an interval in hours and a Unix timestamp to a Quartz cron expression
// The cron expression will be scheduled to run at the same minute and hour (for daily)
// as the reference timestamp, repeating at the specified interval.
//
// Supported intervals: 1, 2, 3, 6, 8, 12, 24 hours
//
// Returns Quartz format cron expressions (6 parts: second minute hour day month day-of-week)
//
// Examples:
//   - 24 hours: "0 15 10 ? * *" (runs daily at 10:15 AM)
//   - 12 hours: "0 15 10,22 ? * *" (runs at 10:15 AM and 10:15 PM)
//   - 6 hours: "0 15 4,10,16,22 ? * *" (runs every 6 hours starting from the reference time)
//   - 1 hour: "0 15 * ? * *" (runs every hour at minute 15)
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
		// Quartz format: second minute hour day month day-of-week
		return fmt.Sprintf("0 %d * ? * *", minute), nil

	case 2:
		// Every 2 hours starting from the reference hour
		// Generate hours: hour, hour+2, hour+4, etc. (mod 24)
		hours := make([]int, 0, 12)
		for h := hour; len(hours) < 12; h = (h + 2) % 24 {
			hours = append(hours, h)
		}
		return fmt.Sprintf("0 %d %s ? * *", minute, formatHours(hours)), nil

	case 3:
		// Every 3 hours starting from the reference hour
		hours := make([]int, 0, 8)
		for h := hour; len(hours) < 8; h = (h + 3) % 24 {
			hours = append(hours, h)
		}
		return fmt.Sprintf("0 %d %s ? * *", minute, formatHours(hours)), nil

	case 6:
		// Every 6 hours starting from the reference hour
		hours := make([]int, 0, 4)
		for h := hour; len(hours) < 4; h = (h + 6) % 24 {
			hours = append(hours, h)
		}
		return fmt.Sprintf("0 %d %s ? * *", minute, formatHours(hours)), nil

	case 8:
		// Every 8 hours starting from the reference hour
		hours := make([]int, 0, 3)
		for h := hour; len(hours) < 3; h = (h + 8) % 24 {
			hours = append(hours, h)
		}
		return fmt.Sprintf("0 %d %s ? * *", minute, formatHours(hours)), nil

	case 12:
		// Every 12 hours starting from the reference hour
		hours := []int{hour, (hour + 12) % 24}
		return fmt.Sprintf("0 %d %s ? * *", minute, formatHours(hours)), nil

	case 24:
		// Once per day at the specified time
		return fmt.Sprintf("0 %d %d ? * *", minute, hour), nil

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

// ConvertUnixCronToQuartz converts a Unix cron expression (5 parts) to Quartz format (6 parts)
// Unix format: minute hour day month day-of-week
// Quartz format: second minute hour day month day-of-week
func ConvertUnixCronToQuartz(unixCron string) (string, error) {
	parts := strings.Fields(strings.TrimSpace(unixCron))

	// If already 6 or 7 parts, assume it's already Quartz format
	if len(parts) >= 6 {
		// Check if it needs the ? fix
		if parts[3] == "*" && parts[5] == "*" {
			parts[3] = "?"
		}
		return strings.Join(parts, " "), nil
	}

	// If not 5 parts, it's invalid
	if len(parts) != 5 {
		return "", fmt.Errorf("invalid cron expression: expected 5 parts (Unix) or 6 parts (Quartz), got %d", len(parts))
	}

	// Convert Unix to Quartz by prepending "0" for seconds and replacing day "*" with "?"
	minute := parts[0]
	hour := parts[1]
	day := parts[2]
	month := parts[3]
	dayOfWeek := parts[4]

	// In Quartz, one of day or dayOfWeek must be "?" (not specified)
	// If dayOfWeek is specified (not *), use ? for day
	// If both are *, use ? for day (default behavior)
	if day == "*" {
		day = "?"
	} else if dayOfWeek != "*" {
		// If day has a value and dayOfWeek also has a value, that's invalid
		// But in Unix cron, this typically means dayOfWeek takes precedence
		// So we set day to ? when dayOfWeek is specified
		day = "?"
	}

	// Prepend 0 for seconds
	return fmt.Sprintf("0 %s %s %s %s %s", minute, hour, day, month, dayOfWeek), nil
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
