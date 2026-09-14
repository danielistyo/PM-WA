package cmd

import (
	"testing"
	"time"

	"github.com/robfig/cron/v3"
)

var testLoc = time.FixedZone("GMT+7", 7*60*60)

func TestParseReminderSpecKeywords(t *testing.T) {
	cases := map[string]ReminderSpec{
		"":    {Enabled: true},
		"yes": {Enabled: true},
		"YES": {Enabled: true},
		"no":  {Enabled: false},
		"No":  {Enabled: false},
	}
	for in, want := range cases {
		got, err := ParseReminderSpec(in, testLoc)
		if err != nil {
			t.Fatalf("ParseReminderSpec(%q) unexpected error: %v", in, err)
		}
		if got != want {
			t.Errorf("ParseReminderSpec(%q) = %+v, want %+v", in, got, want)
		}
	}
}

func TestParseReminderSpecRecurring(t *testing.T) {
	cases := map[string]string{
		"every day at 08:30":    "30 8 * * *",
		"every monday at 20:00": "0 20 * * 1",
		"every sunday at 7.05":  "5 7 * * 0",
		"every 3rd at 09:00":    "0 9 3 * *",
		"every 15 at 23:59":     "59 23 15 * *",
		"EVERY Friday AT 18:00": "0 18 * * 5",
	}
	for in, want := range cases {
		got, err := ParseReminderSpec(in, testLoc)
		if err != nil {
			t.Fatalf("ParseReminderSpec(%q) unexpected error: %v", in, err)
		}
		if !got.Enabled || got.At != 0 || got.Cron != want {
			t.Errorf("ParseReminderSpec(%q) = %+v, want cron %q", in, got, want)
		}
		if _, err := cron.ParseStandard(got.Cron); err != nil {
			t.Errorf("generated cron %q is invalid: %v", got.Cron, err)
		}
	}
}

func TestParseReminderSpecRawCron(t *testing.T) {
	got, err := ParseReminderSpec("0 20 * * 1", testLoc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Enabled || got.Cron != "0 20 * * 1" || got.At != 0 {
		t.Errorf("got %+v, want raw cron passthrough", got)
	}
}

func TestParseReminderSpecAbsolute(t *testing.T) {
	future := time.Now().In(testLoc).Add(48 * time.Hour)
	inputs := []string{
		future.Format("2006-01-02 15:04"),
		future.Format("2 January 2006 15:04"),
	}
	for _, in := range inputs {
		got, err := ParseReminderSpec(in, testLoc)
		if err != nil {
			t.Fatalf("ParseReminderSpec(%q) unexpected error: %v", in, err)
		}
		if !got.Enabled || got.At == 0 || got.Cron != "" {
			t.Errorf("ParseReminderSpec(%q) = %+v, want absolute At>0", in, got)
		}
		wantMinute := future.Truncate(time.Minute).Unix()
		if got.At != wantMinute {
			t.Errorf("ParseReminderSpec(%q) At = %d, want %d", in, got.At, wantMinute)
		}
	}
}

func TestParseReminderSpecAbsolutePastRejected(t *testing.T) {
	past := time.Now().In(testLoc).Add(-48 * time.Hour).Format("2006-01-02 15:04")
	if _, err := ParseReminderSpec(past, testLoc); err == nil {
		t.Errorf("ParseReminderSpec(%q) expected error for past time", past)
	}
}

func TestParseReminderSpecInvalid(t *testing.T) {
	inputs := []string{
		"every 40th at 09:00",
		"every day at 25:00",
		"sometime soon",
		"99 99 * * *",
	}
	for _, in := range inputs {
		if _, err := ParseReminderSpec(in, testLoc); err == nil {
			t.Errorf("ParseReminderSpec(%q) expected error, got nil", in)
		}
	}
}

// TestCronDueCheck verifies the "fires this minute" predicate used by the
// scheduler tick matches a generated weekly cron at the exact minute.
func TestCronDueCheck(t *testing.T) {
	spec, err := ParseReminderSpec("every monday at 20:00", testLoc)
	if err != nil {
		t.Fatal(err)
	}
	sched, err := cron.ParseStandard(spec.Cron)
	if err != nil {
		t.Fatal(err)
	}

	// Monday 2024-01-01 20:00 GMT+7.
	due := time.Date(2024, 1, 1, 20, 0, 0, 0, testLoc)
	if !sched.Next(due.Add(-time.Second)).Equal(due) {
		t.Errorf("expected schedule to be due at %v", due)
	}

	notDue := time.Date(2024, 1, 1, 20, 1, 0, 0, testLoc)
	if sched.Next(notDue.Add(-time.Second)).Equal(notDue) {
		t.Errorf("did not expect schedule to be due at %v", notDue)
	}
}
