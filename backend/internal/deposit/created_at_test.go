package deposit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveCreatedAt(t *testing.T) {
	ct, err := time.LoadLocation("America/Chicago")
	require.NoError(t, err)

	tests := []struct {
		override   string
		wantHour   int
		wantMinute int
		desc       string
	}{
		{"before_cutoff", 15, 0, "3:00 PM CT — before 6:30 PM cutoff"},
		{"after_cutoff", 19, 15, "7:15 PM CT — after 6:30 PM cutoff"},
		{"yesterday", 14, 0, "yesterday 2:00 PM CT"},
		{"", -1, -1, "empty string returns current time (hour not asserted)"},
	}

	for _, tt := range tests {
		t.Run(tt.override, func(t *testing.T) {
			before := time.Now().Add(-time.Second)
			result := resolveCreatedAt(tt.override)
			after := time.Now().Add(time.Second)

			ctResult := result.In(ct)

			if tt.override == "" {
				// No override: result must be approximately now
				assert.True(t, result.After(before) && result.Before(after),
					"empty override should return current time")
				return
			}

			assert.Equal(t, tt.wantHour, ctResult.Hour(),
				"%s: expected hour %d in CT, got %d", tt.desc, tt.wantHour, ctResult.Hour())
			assert.Equal(t, tt.wantMinute, ctResult.Minute(),
				"%s: expected minute %d in CT, got %d", tt.desc, tt.wantMinute, ctResult.Minute())

			if tt.override == "yesterday" {
				expectedDate := time.Now().In(ct).AddDate(0, 0, -1)
				assert.Equal(t, expectedDate.Day(), ctResult.Day(),
					"yesterday override must be on the previous calendar day (CT)")
			} else {
				today := time.Now().In(ct)
				assert.Equal(t, today.Day(), ctResult.Day(),
					"before/after_cutoff must be on today's calendar day (CT)")
			}
		})
	}
}
