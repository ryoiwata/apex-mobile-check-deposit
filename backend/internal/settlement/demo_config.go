package settlement

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
)

// DemoCutoff holds an in-memory EOD cutoff time override used during demos.
// Thread-safe. The zero value means no override is active.
//
// Priority when resolving the effective cutoff:
//  1. In-memory override (Set by the demo control API)
//  2. EOD_CUTOFF_OVERRIDE env var (format "HH:MM", e.g. "10:00")
//  3. Default: 18:30 CT (6:30 PM)
type DemoCutoff struct {
	mu         sync.Mutex
	hour       int
	minute     int
	overridden bool
}

// Set overrides the effective cutoff to the given hour:minute (CT, 24-hour clock).
func (d *DemoCutoff) Set(hour, minute int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.hour = hour
	d.minute = minute
	d.overridden = true
}

// Reset clears the in-memory override; GetEffective falls back to env var or default.
func (d *DemoCutoff) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.overridden = false
}

// GetEffective returns the active cutoff hour and minute (CT, 24-hour clock).
func (d *DemoCutoff) GetEffective() (hour, minute int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.overridden {
		return d.hour, d.minute
	}

	if v := os.Getenv("EOD_CUTOFF_OVERRIDE"); v != "" {
		parts := strings.SplitN(v, ":", 2)
		if len(parts) == 2 {
			h, errH := strconv.Atoi(strings.TrimSpace(parts[0]))
			m, errM := strconv.Atoi(strings.TrimSpace(parts[1]))
			if errH == nil && errM == nil && h >= 0 && h <= 23 && m >= 0 && m <= 59 {
				return h, m
			}
		}
	}

	return 18, 30 // default: 6:30 PM CT
}

// IsOverridden returns true when any override (in-memory or env var) is active.
func (d *DemoCutoff) IsOverridden() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.overridden {
		return true
	}
	return os.Getenv("EOD_CUTOFF_OVERRIDE") != ""
}

// Label returns a human-readable cutoff description for API responses.
func (d *DemoCutoff) Label() string {
	h, m := d.GetEffective()
	base := fmt.Sprintf("%02d:%02d CT", h, m)
	if d.IsOverridden() {
		return base + " (demo override)"
	}
	return base + " (default)"
}
