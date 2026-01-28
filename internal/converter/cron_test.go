package converter

import (
	"testing"
	"time"
)

func TestIntervalToCron(t *testing.T) {
	// Example timestamp: 1753735965 (2025-07-28 20:52:45 UTC)
	timestamp := int64(1753735965)
	refTime := time.Unix(timestamp, 0).UTC()
	t.Logf("Reference time: %s (minute=%d, hour=%d)", refTime.Format(time.RFC3339), refTime.Minute(), refTime.Hour())

	tests := []struct {
		name          string
		intervalHours int
		timestamp     int64
		wantCron      string
		wantErr       bool
	}{
		{
			name:          "24 hours - daily at same time",
			intervalHours: 24,
			timestamp:     timestamp,
			wantCron:      "0 52 20 ? * *", // Every day at 20:52 UTC (Quartz format)
			wantErr:       false,
		},
		{
			name:          "12 hours - twice daily",
			intervalHours: 12,
			timestamp:     timestamp,
			wantCron:      "0 52 8,20 ? * *", // At 08:52 and 20:52 UTC (Quartz format)
			wantErr:       false,
		},
		{
			name:          "6 hours - four times daily",
			intervalHours: 6,
			timestamp:     timestamp,
			wantCron:      "0 52 2,8,14,20 ? * *", // At 02:52, 08:52, 14:52, 20:52 UTC (Quartz format)
			wantErr:       false,
		},
		{
			name:          "3 hours - eight times daily",
			intervalHours: 3,
			timestamp:     timestamp,
			wantCron:      "0 52 2,5,8,11,14,17,20,23 ? * *", // Quartz format
			wantErr:       false,
		},
		{
			name:          "2 hours - twelve times daily",
			intervalHours: 2,
			timestamp:     timestamp,
			wantCron:      "0 52 0,2,4,6,8,10,12,14,16,18,20,22 ? * *", // Quartz format
			wantErr:       false,
		},
		{
			name:          "1 hour - every hour",
			intervalHours: 1,
			timestamp:     timestamp,
			wantCron:      "0 52 * ? * *", // Every hour at minute 52 (Quartz format)
			wantErr:       false,
		},
		{
			name:          "8 hours - three times daily",
			intervalHours: 8,
			timestamp:     timestamp,
			wantCron:      "0 52 4,12,20 ? * *", // At 04:52, 12:52, 20:52 UTC (Quartz format)
			wantErr:       false,
		},
		{
			name:          "invalid interval",
			intervalHours: 5,
			timestamp:     timestamp,
			wantCron:      "",
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCron, err := IntervalToCron(tt.intervalHours, tt.timestamp)
			if (err != nil) != tt.wantErr {
				t.Errorf("IntervalToCron() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotCron != tt.wantCron {
				t.Errorf("IntervalToCron() = %v, want %v", gotCron, tt.wantCron)
			}
		})
	}
}

func TestIntervalToCronDifferentTimestamps(t *testing.T) {
	tests := []struct {
		name          string
		intervalHours int
		timestamp     int64
		wantCron      string
	}{
		{
			name:          "midnight start - 24 hours",
			intervalHours: 24,
			timestamp:     1753660800, // 2025-09-27 00:00:00 UTC
			wantCron:      "0 0 0 ? * *", // Quartz format
		},
		{
			name:          "noon start - 12 hours",
			intervalHours: 12,
			timestamp:     1753704000, // 2025-09-27 12:00:00 UTC
			wantCron:      "0 0 0,12 ? * *", // Quartz format
		},
		{
			name:          "3am start - 6 hours",
			intervalHours: 6,
			timestamp:     1753671600, // 2025-09-27 03:00:00 UTC
			wantCron:      "0 0 3,9,15,21 ? * *", // Quartz format
		},
		{
			name:          "1:30am start - 1 hour",
			intervalHours: 1,
			timestamp:     1753666200, // 2025-09-27 01:30:00 UTC
			wantCron:      "0 30 * ? * *", // Quartz format
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refTime := time.Unix(tt.timestamp, 0).UTC()
			t.Logf("Reference time: %s", refTime.Format(time.RFC3339))

			gotCron, err := IntervalToCron(tt.intervalHours, tt.timestamp)
			if err != nil {
				t.Errorf("IntervalToCron() unexpected error = %v", err)
				return
			}
			if gotCron != tt.wantCron {
				t.Errorf("IntervalToCron() = %v, want %v", gotCron, tt.wantCron)
			}
		})
	}
}

func TestConvertUnixCronToQuartz(t *testing.T) {
	tests := []struct {
		name      string
		unixCron  string
		wantCron  string
		wantErr   bool
	}{
		{
			name:     "standard unix cron - 5 parts",
			unixCron: "14 18 * * *",
			wantCron: "0 14 18 ? * *",
			wantErr:  false,
		},
		{
			name:     "unix cron with specific days",
			unixCron: "30 15 1,15 * *",
			wantCron: "0 30 15 1,15 * *",
			wantErr:  false,
		},
		{
			name:     "unix cron with day of week",
			unixCron: "0 9 * * 1-5",
			wantCron: "0 0 9 ? * 1-5",
			wantErr:  false,
		},
		{
			name:     "already quartz format - 6 parts",
			unixCron: "0 14 18 ? * *",
			wantCron: "0 14 18 ? * *",
			wantErr:  false,
		},
		{
			name:     "quartz with * instead of ? - needs fix",
			unixCron: "0 14 18 * * *",
			wantCron: "0 14 18 ? * *",
			wantErr:  false,
		},
		{
			name:     "invalid - too few parts",
			unixCron: "14 18",
			wantCron: "",
			wantErr:  true,
		},
		{
			name:     "hourly schedule",
			unixCron: "15 * * * *",
			wantCron: "0 15 * ? * *",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCron, err := ConvertUnixCronToQuartz(tt.unixCron)
			if (err != nil) != tt.wantErr {
				t.Errorf("ConvertUnixCronToQuartz() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotCron != tt.wantCron {
				t.Errorf("ConvertUnixCronToQuartz() = %v, want %v", gotCron, tt.wantCron)
			}
		})
	}
}

func TestFormatHours(t *testing.T) {
	tests := []struct {
		name  string
		hours []int
		want  string
	}{
		{
			name:  "single hour",
			hours: []int{5},
			want:  "5",
		},
		{
			name:  "multiple hours in order",
			hours: []int{1, 5, 9, 13},
			want:  "1,5,9,13",
		},
		{
			name:  "multiple hours out of order",
			hours: []int{13, 1, 9, 5},
			want:  "1,5,9,13",
		},
		{
			name:  "hours wrapping around midnight",
			hours: []int{22, 4, 10, 16},
			want:  "4,10,16,22",
		},
		{
			name:  "empty slice",
			hours: []int{},
			want:  "*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatHours(tt.hours)
			if got != tt.want {
				t.Errorf("formatHours() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseBasicTimingToCron(t *testing.T) {
	timestamp := int64(1753735965) // 2025-07-28 20:52:45 UTC

	tests := []struct {
		name        string
		basicTiming string
		timestamp   int64
		wantCron    string
		wantErr     bool
	}{
		{
			name:        "24 hours",
			basicTiming: "Every 24 HOURS",
			timestamp:   timestamp,
			wantCron:    "0 52 20 ? * *", // Quartz format
			wantErr:     false,
		},
		{
			name:        "12 hours",
			basicTiming: "Every 12 HOURS",
			timestamp:   timestamp,
			wantCron:    "0 52 8,20 ? * *", // Quartz format
			wantErr:     false,
		},
		{
			name:        "6 hours",
			basicTiming: "Every 6 HOURS",
			timestamp:   timestamp,
			wantCron:    "0 52 2,8,14,20 ? * *", // Quartz format
			wantErr:     false,
		},
		{
			name:        "1 hour",
			basicTiming: "Every 1 HOURS",
			timestamp:   timestamp,
			wantCron:    "0 52 * ? * *", // Quartz format
			wantErr:     false,
		},
		{
			name:        "1 hour singular",
			basicTiming: "Every 1 HOUR",
			timestamp:   timestamp,
			wantCron:    "0 52 * ? * *", // Quartz format
			wantErr:     false,
		},
		{
			name:        "lowercase",
			basicTiming: "every 24 hours",
			timestamp:   timestamp,
			wantCron:    "0 52 20 ? * *", // Quartz format
			wantErr:     false,
		},
		{
			name:        "mixed case",
			basicTiming: "Every 12 Hours",
			timestamp:   timestamp,
			wantCron:    "0 52 8,20 ? * *", // Quartz format
			wantErr:     false,
		},
		{
			name:        "extra whitespace",
			basicTiming: "  Every   24   HOURS  ",
			timestamp:   timestamp,
			wantCron:    "0 52 20 ? * *", // Quartz format
			wantErr:     false,
		},
		{
			name:        "invalid format - missing parts",
			basicTiming: "Every 24",
			timestamp:   timestamp,
			wantCron:    "",
			wantErr:     true,
		},
		{
			name:        "invalid format - wrong start",
			basicTiming: "Each 24 HOURS",
			timestamp:   timestamp,
			wantCron:    "",
			wantErr:     true,
		},
		{
			name:        "invalid format - wrong unit",
			basicTiming: "Every 24 MINUTES",
			timestamp:   timestamp,
			wantCron:    "",
			wantErr:     true,
		},
		{
			name:        "invalid interval - not a number",
			basicTiming: "Every X HOURS",
			timestamp:   timestamp,
			wantCron:    "",
			wantErr:     true,
		},
		{
			name:        "invalid interval - unsupported",
			basicTiming: "Every 5 HOURS",
			timestamp:   timestamp,
			wantCron:    "",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCron, err := ParseBasicTimingToCron(tt.basicTiming, tt.timestamp)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseBasicTimingToCron() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotCron != tt.wantCron {
				t.Errorf("ParseBasicTimingToCron() = %v, want %v", gotCron, tt.wantCron)
			}
		})
	}
}
