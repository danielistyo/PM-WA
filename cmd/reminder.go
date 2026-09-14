package cmd

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// ReminderSpec is the parsed result of a task's reminder field.
//   - Enabled=false  -> task is excluded from reminders.
//   - Cron!=""       -> recurring reminder driven by a 5-field cron expression.
//   - At>0           -> one-time reminder at a specific unix timestamp.
//   - Cron=="" && At==0 -> use the scheduler's default schedule.
type ReminderSpec struct {
	Enabled bool
	Cron    string
	At      int64
}

var reminderUsage = "Invalid reminder. Use one of:\n" +
	"- yes / no\n" +
	"- every day at HH:MM\n" +
	"- every monday at HH:MM (any weekday)\n" +
	"- every 3rd at HH:MM (day of month 1-31)\n" +
	"- a raw cron expression (e.g. 0 20 * * 1)\n" +
	"- an absolute time: 2029-03-29 20:00 or 29 march 2029 20:00"

var weekdayToCron = map[string]string{
	"sunday":    "0",
	"monday":    "1",
	"tuesday":   "2",
	"wednesday": "3",
	"thursday":  "4",
	"friday":    "5",
	"saturday":  "6",
}

var recurringRe = regexp.MustCompile(`^every\s+([a-z]+|\d{1,2})(?:st|nd|rd|th)?\s+at\s+(\d{1,2})[:.](\d{2})$`)

var absoluteLayouts = []string{
	"2006-01-02 15:04",
	"2006-01-02 15.04",
	"2 January 2006 15:04",
	"2 January 2006 15.04",
	"2 Jan 2006 15:04",
	"2 Jan 2006 15.04",
}

// ParseReminderSpec converts a user-supplied reminder field into a ReminderSpec.
// loc is the timezone used to interpret absolute datetimes.
func ParseReminderSpec(input string, loc *time.Location) (ReminderSpec, error) {
	raw := strings.TrimSpace(input)
	lower := strings.ToLower(raw)

	switch lower {
	case "", "yes":
		return ReminderSpec{Enabled: true}, nil
	case "no":
		return ReminderSpec{Enabled: false}, nil
	}

	if spec, ok, err := parseRecurring(lower); ok {
		if err != nil {
			return ReminderSpec{}, err
		}
		return spec, nil
	}

	if at, ok, err := parseAbsolute(raw, loc); ok {
		if err != nil {
			return ReminderSpec{}, err
		}
		return ReminderSpec{Enabled: true, At: at}, nil
	}

	if expr, ok := parseRawCron(raw); ok {
		return ReminderSpec{Enabled: true, Cron: expr}, nil
	}

	return ReminderSpec{}, errors.New(reminderUsage)
}

func parseRecurring(lower string) (ReminderSpec, bool, error) {
	m := recurringRe.FindStringSubmatch(lower)
	if m == nil {
		return ReminderSpec{}, false, nil
	}

	unit := m[1]
	hour, _ := strconv.Atoi(m[2])
	minute, _ := strconv.Atoi(m[3])
	if hour > 23 || minute > 59 {
		return ReminderSpec{}, true, errors.New("Invalid reminder time: hours must be 0-23 and minutes 0-59.")
	}

	if unit == "day" {
		return ReminderSpec{Enabled: true, Cron: fmt.Sprintf("%d %d * * *", minute, hour)}, true, nil
	}

	if dow, ok := weekdayToCron[unit]; ok {
		return ReminderSpec{Enabled: true, Cron: fmt.Sprintf("%d %d * * %s", minute, hour, dow)}, true, nil
	}

	if dom, err := strconv.Atoi(unit); err == nil {
		if dom < 1 || dom > 31 {
			return ReminderSpec{}, true, errors.New("Invalid reminder: day of month must be 1-31.")
		}
		return ReminderSpec{Enabled: true, Cron: fmt.Sprintf("%d %d %d * *", minute, hour, dom)}, true, nil
	}

	return ReminderSpec{}, true, errors.New(reminderUsage)
}

func parseAbsolute(raw string, loc *time.Location) (int64, bool, error) {
	for _, layout := range absoluteLayouts {
		t, err := time.ParseInLocation(layout, raw, loc)
		if err != nil {
			continue
		}
		if !t.After(time.Now()) {
			return 0, true, errors.New("Invalid reminder: the absolute time is in the past.")
		}
		return t.Unix(), true, nil
	}
	return 0, false, nil
}

func parseRawCron(raw string) (string, bool) {
	if len(strings.Fields(raw)) != 5 {
		return "", false
	}
	if _, err := cron.ParseStandard(raw); err != nil {
		return "", false
	}
	return raw, true
}
