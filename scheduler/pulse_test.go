package scheduler

import (
	"testing"
	"time"

	"pm-wa/db"
)

// minute returns the unix timestamp of a GMT+7 datetime, at minute precision.
func minute(t *testing.T, y int, mo time.Month, d, h, mi int) int64 {
	t.Helper()
	loc := time.FixedZone("GMT+7", 7*60*60)
	return time.Date(y, mo, d, h, mi, 0, 0, loc).Unix()
}

func TestTaskDueOnceAt(t *testing.T) {
	at := minute(t, 2026, 3, 10, 20, 0)
	tests := []struct {
		name           string
		reminderAt     int64
		lastRemindedAt int64
		nowMin         int64
		want           bool
	}{
		{
			name:       "fires at the reminder minute",
			reminderAt: at,
			nowMin:     at,
			want:       true,
		},
		{
			name:       "not due before the reminder minute",
			reminderAt: at,
			nowMin:     at - 60,
			want:       false,
		},
		{
			name:       "fires late once after a missed minute (bot offline)",
			reminderAt: at,
			nowMin:     at + 60,
			want:       true,
		},
		{
			name:       "fires late once well after a missed minute",
			reminderAt: at,
			nowMin:     at + 300,
			want:       true,
		},
		{
			name:           "not due again after being reminded",
			reminderAt:     at,
			lastRemindedAt: at + 300,
			nowMin:         at + 360,
			want:           false,
		},
		{
			name:       "seconds are truncated to the minute",
			reminderAt: at + 30,
			nowMin:     at,
			want:       true,
		},
		{
			name:       "deadline is ignored in mode 'at'",
			reminderAt: at,
			nowMin:     at - 600, // before reminder time but past the deadline
			want:       false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := db.Task{
				ReminderMode:   db.ModeAt,
				ReminderAt:     tt.reminderAt,
				Deadline:       at - 300, // before the reminder time; must be ignored
				LastRemindedAt: tt.lastRemindedAt,
			}
			minuteStart := time.Unix(tt.nowMin, 0)
			if got := taskDue(task, minuteStart, tt.nowMin); got != tt.want {
				t.Errorf("taskDue(reminderAt=%d, last=%d, now=%d) = %v, want %v",
					tt.reminderAt, tt.lastRemindedAt, tt.nowMin, got, tt.want)
			}
		})
	}
}

func TestTaskDueAtDeadline(t *testing.T) {
	deadline := minute(t, 2026, 3, 10, 20, 0)
	tests := []struct {
		name           string
		deadline       int64
		lastRemindedAt int64
		nowMin         int64
		want           bool
	}{
		{
			name:     "fires at the deadline minute",
			deadline: deadline,
			nowMin:   deadline,
			want:     true,
		},
		{
			name:     "not due one minute before the deadline",
			deadline: deadline,
			nowMin:   deadline - 60,
			want:     false,
		},
		{
			name:     "not due ten minutes before the deadline",
			deadline: deadline,
			nowMin:   deadline - 600,
			want:     false,
		},
		{
			name:     "fires late once after a missed minute (bot offline)",
			deadline: deadline,
			nowMin:   deadline + 300,
			want:     true,
		},
		{
			name:           "not due again once reminded",
			deadline:       deadline,
			lastRemindedAt: deadline,
			nowMin:         deadline + 300,
			want:           false,
		},
		{
			name:     "fires once immediately when deadline already passed",
			deadline: deadline - 600,
			nowMin:   deadline,
			want:     true,
		},
		{
			name:           "past deadline not re-fired after reminding",
			deadline:       deadline - 600,
			lastRemindedAt: deadline,
			nowMin:         deadline + 60,
			want:           false,
		},
		{
			name:     "seconds-only offset in deadline is truncated",
			deadline: deadline + 45,
			nowMin:   deadline,
			want:     true,
		},
		{
			name:     "stray reminder_at value is ignored in mode 'deadline'",
			deadline: deadline,
			nowMin:   deadline,
			want:     true, // even when now differs from the stray value's minute
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := db.Task{
				ReminderMode:   db.ModeDeadline,
				Deadline:       tt.deadline,
				LastRemindedAt: tt.lastRemindedAt,
				ReminderAt:     minute(t, 2030, 1, 1, 8, 0), // stray value; must be ignored
			}
			minuteStart := time.Unix(tt.nowMin, 0)
			if got := taskDue(task, minuteStart, tt.nowMin); got != tt.want {
				t.Errorf("taskDue(deadline=%d, last=%d, now=%d) = %v, want %v",
					tt.deadline, tt.lastRemindedAt, tt.nowMin, got, tt.want)
			}
		})
	}
}

func TestTaskDueCron(t *testing.T) {
	// "0 20 * * 1" -> Mondays 20:00. 2026-03-09 is a Monday, 2026-03-10 Tuesday.
	due := time.Date(2026, time.March, 9, 20, 0, 0, 0, gmt7).Unix()
	tests := []struct {
		name        string
		cron        string
		dueMinute   int64
		otherMinute int64
	}{
		{
			name:        "fires on the matching cron minute",
			cron:        "0 20 * * 1",
			dueMinute:   due,
			otherMinute: due + 60,
		},
		{
			name:        "invalid cron never fires",
			cron:        "not-a-cron",
			dueMinute:   due,
			otherMinute: due,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := db.Task{
				ReminderMode: db.ModeCron,
				ReminderCron: tt.cron,
			}
			cases := []struct {
				name string
				min  int64
				want bool
			}{
				{"cron minute", tt.dueMinute, tt.name == "fires on the matching cron minute"},
				{"other minute", tt.otherMinute, false},
			}
			for _, c := range cases {
				t.Run(c.name, func(t *testing.T) {
					minuteStart := time.Unix(c.min, 0)
					if got := taskDue(task, minuteStart, c.min); got != c.want {
						t.Errorf("taskDue(cron=%q, now=%d) = %v, want %v", tt.cron, c.min, got, c.want)
					}
				})
			}
		})
	}
}

func TestTaskDueDisabledAndUnknownModes(t *testing.T) {
	now := minute(t, 2026, 3, 10, 20, 0)
	deadline := now
	tests := []struct {
		name         string
		reminderMode string
	}{
		{name: "mode none never fires", reminderMode: db.ModeNone},
		{name: "unknown mode never fires", reminderMode: "mystery"},
		{name: "empty mode never fires", reminderMode: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := db.Task{
				ReminderMode: tt.reminderMode,
				Deadline:     deadline,
				ReminderAt:   now, // would fire if the mode were honored
			}
			minuteStart := time.Unix(now, 0)
			if got := taskDue(task, minuteStart, now); got {
				t.Errorf("taskDue(mode=%q) = true, want false", tt.reminderMode)
			}
		})
	}
}
