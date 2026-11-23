package scheduler

import (
	"fmt"
	"hash/fnv"
	"time"

	"github.com/robfig/cron/v3"
)

// Frequency is the only scheduling choice users get. The time of day is not
// user-configurable: each user is assigned a fixed slot (see Slot).
type Frequency string

const (
	Off    Frequency = "off"
	Daily  Frequency = "daily"
	Weekly Frequency = "weekly"
	// Custom is reported for a stored schedule that matches no preset (e.g. one
	// set through the API before presets existed). It still runs as stored.
	Custom Frequency = "custom"
)

// Syncs run after the TSX/NYSE close (4:00 PM Eastern) so they post settled
// closing balances. Each user gets a slot in [4:30 PM, 7:30 PM) Eastern.
const (
	marketTimeZone = "America/Toronto"
	windowStart    = 16*60 + 30 // minutes after midnight, Eastern
	windowMinutes  = 180
)

// Slot is the user's sync time: hour and minute, Eastern. It's a hash of the
// user ID, so it is stable across requests and processes without being stored,
// and a large user base spreads evenly over the window instead of everyone
// syncing in the same minute.
func Slot(userID string) (hour, minute int) {
	h := fnv.New32a()
	h.Write([]byte(userID))
	m := windowStart + int(h.Sum32()%windowMinutes)
	return m / 60, m % 60
}

// CronFor returns the cron expression for a user's preset frequency: weekdays
// for daily (balances don't move when markets are closed), Fridays for weekly.
func CronFor(f Frequency, userID string) (string, error) {
	var days string
	switch f {
	case Daily:
		days = "1-5"
	case Weekly:
		days = "5"
	default:
		return "", fmt.Errorf("unknown frequency %q", f)
	}
	hour, minute := Slot(userID)
	return fmt.Sprintf("CRON_TZ=%s %d %d * * %s", marketTimeZone, minute, hour, days), nil
}

// FrequencyOf maps a user's stored schedule back to the preset it was created from.
func FrequencyOf(cronExpr string, enabled bool, userID string) Frequency {
	if !enabled {
		return Off
	}
	for _, f := range []Frequency{Daily, Weekly} {
		if expr, _ := CronFor(f, userID); expr == cronExpr {
			return f
		}
	}
	return Custom
}

// NextRun returns when a schedule next fires after t, or nil if the expression
// doesn't parse.
func NextRun(cronExpr string, t time.Time) *time.Time {
	sched, err := cron.ParseStandard(cronExpr)
	if err != nil {
		return nil
	}
	next := sched.Next(t)
	return &next
}

// isDue reports whether cronExpr fires in the window (from, to].
func isDue(cronExpr string, from, to time.Time) (bool, error) {
	sched, err := cron.ParseStandard(cronExpr)
	if err != nil {
		return false, err
	}
	next := sched.Next(from)
	return !next.IsZero() && !next.After(to), nil
}
