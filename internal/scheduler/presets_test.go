package scheduler

import (
	"fmt"
	"testing"
	"time"
)

func TestFrequencyRoundTrip(t *testing.T) {
	const user = "39ca58de-fa06-4d78-9ffc-cb1cac44912b"
	for _, f := range []Frequency{Daily, Weekly} {
		expr, err := CronFor(f, user)
		if err != nil {
			t.Fatal(err)
		}
		if got := FrequencyOf(expr, true, user); got != f {
			t.Errorf("FrequencyOf(CronFor(%s)) = %s", f, got)
		}
		if got := FrequencyOf(expr, false, user); got != Off {
			t.Errorf("disabled %s reported as %s", f, got)
		}
		// Another user's expression is not this user's preset.
		other, _ := CronFor(f, "someone-else")
		if other != expr && FrequencyOf(other, true, user) != Custom {
			t.Errorf("another user's %s cron should read as custom", f)
		}
	}
	if _, err := CronFor(Custom, user); err == nil {
		t.Error("CronFor(custom) should fail")
	}
}

func TestSlotsStayInWindowAndSpread(t *testing.T) {
	const users = 10000
	perMinute := map[int]int{}
	for i := 0; i < users; i++ {
		h, m := Slot(fmt.Sprintf("user-%d", i))
		mins := h*60 + m
		if mins < windowStart || mins >= windowStart+windowMinutes {
			t.Fatalf("slot %02d:%02d outside window", h, m)
		}
		perMinute[mins]++
	}
	if len(perMinute) != windowMinutes {
		t.Errorf("only %d of %d minutes used", len(perMinute), windowMinutes)
	}
	avg := users / windowMinutes
	for mins, n := range perMinute {
		if n > 2*avg {
			t.Errorf("%02d:%02d has %d users, more than twice the average %d", mins/60, mins%60, n, avg)
		}
	}
}

func TestIsDueFiveMinuteTicks(t *testing.T) {
	eastern, err := time.LoadLocation(marketTimeZone)
	if err != nil {
		t.Fatal(err)
	}
	const user = "39ca58de-fa06-4d78-9ffc-cb1cac44912b"
	hour, minute := Slot(user)
	daily, _ := CronFor(Daily, user)
	weekly, _ := CronFor(Weekly, user)

	// Every 5-minute tick of one day; a schedule must fire in exactly one window.
	countRuns := func(expr string, day time.Time) int {
		n := 0
		for to := day.Add(5 * time.Minute); !to.After(day.AddDate(0, 0, 1)); to = to.Add(5 * time.Minute) {
			due, err := isDue(expr, to.Add(-5*time.Minute), to)
			if err != nil {
				t.Fatal(err)
			}
			if due {
				n++
				slot := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, eastern)
				if slot.After(to) || !slot.After(to.Add(-5*time.Minute)) {
					t.Errorf("%q due in window ending %v, slot is %v", expr, to, slot)
				}
			}
		}
		return n
	}

	friday := time.Date(2026, 11, 27, 0, 0, 0, 0, eastern)
	saturday := friday.AddDate(0, 0, 1)
	monday := friday.AddDate(0, 0, 3)
	for _, c := range []struct {
		name string
		expr string
		day  time.Time
		want int
	}{
		{"daily Friday", daily, friday, 1},
		{"daily Saturday", daily, saturday, 0},
		{"daily Monday", daily, monday, 1},
		{"weekly Friday", weekly, friday, 1},
		{"weekly Monday", weekly, monday, 0},
	} {
		if n := countRuns(c.expr, c.day); n != c.want {
			t.Errorf("%s: ran %d times, want %d", c.name, n, c.want)
		}
	}
}
