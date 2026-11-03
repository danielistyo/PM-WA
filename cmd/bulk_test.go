package cmd

import (
	"fmt"
	"strings"
	"testing"

	"pm-wa/bot"
	"pm-wa/db"
)

var bulkExample = "add-tasks\nwa-group: Tim Marketing\nlist: Weekly Report\n\ntitle: Collect data\nassign: 6281234567890, 6289876543210\ndeadline: 2026-11-06 17:00\nreminder: yes\n\ntitle: Compile slides\nassign: 6281234567890\ndeadline: 2026-11-07 12:00\nreminder: every friday at 10:00"

func TestParseBulkPayloadValid(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		wantGroup  string
		wantList   string
		wantBlocks int
		wantTitles []string
	}{
		{
			name:       "two task blocks after headers",
			text:       bulkExample,
			wantGroup:  "Tim Marketing",
			wantList:   "Weekly Report",
			wantBlocks: 2,
			wantTitles: []string{"Collect data", "Compile slides"},
		},
		{
			name:       "single task block",
			text:       "add-tasks\nwa-group: G\nlist: L\n\ntitle: Only one\nassign: 6281200000000\ndeadline: 2026-01-01 09:00",
			wantGroup:  "G",
			wantList:   "L",
			wantBlocks: 1,
			wantTitles: []string{"Only one"},
		},
		{
			name:       "headers separated by extra blank lines",
			text:       "add-tasks\n\nwa-group: G\n\nlist: L\n\ntitle: A\nassign: 6281200000000\ndeadline: 2026-01-01 09:00\n\ntitle: B\nassign: 6281200000000\ndeadline: 2026-01-01 09:00",
			wantGroup:  "G",
			wantList:   "L",
			wantBlocks: 2,
			wantTitles: []string{"A", "B"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := ParseBulkPayload(tt.text)
			if err != nil {
				t.Fatalf("ParseBulkPayload() unexpected error: %v", err)
			}
			if payload.WaGroup != tt.wantGroup || payload.ListName != tt.wantList {
				t.Fatalf("headers = (%q, %q), want (%q, %q)", payload.WaGroup, payload.ListName, tt.wantGroup, tt.wantList)
			}
			if len(payload.Tasks) != tt.wantBlocks {
				t.Fatalf("len(Tasks) = %d, want %d", len(payload.Tasks), tt.wantBlocks)
			}
			for i, want := range tt.wantTitles {
				if got := payload.Tasks[i]["title"]; got != want {
					t.Errorf("Tasks[%d][title] = %q, want %q", i, got, want)
				}
			}
		})
	}
}

func TestParseBulkPayloadValuesWithColons(t *testing.T) {
	payload, err := ParseBulkPayload("add-tasks\nwa-group: G\nlist: L\n\ntitle: Buy: milk and eggs\nassign: 6281200000000\ndeadline: 2026-01-01 09:30")
	if err != nil {
		t.Fatalf("ParseBulkPayload() unexpected error: %v", err)
	}
	if got := payload.Tasks[0]["title"]; got != "Buy: milk and eggs" {
		t.Fatalf("title = %q, want %q", got, "Buy: milk and eggs")
	}
}

func TestParseBulkPayloadErrors(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		wantSub string // substring expected in the error
	}{
		{
			name:    "header line without colon",
			text:    "add-tasks\nwa-group G\nlist: L",
			wantSub: "Unexpected format near 'wa-group G'",
		},
		{
			name:    "task line without colon",
			text:    "add-tasks\nwa-group: G\nlist: L\n\ntitle: A\nno colon here",
			wantSub: "Task #1: line 'no colon here'",
		},
		{
			name:    "unexpected header field",
			text:    "add-tasks\nwa-group: G\nlist: L\ndelete: yes\n\ntitle: A\nassign: 6281200000000\ndeadline: 2026-01-01 09:00",
			wantSub: "Unexpected field 'delete' in the header",
		},
		{
			name:    "unexpected task field",
			text:    "add-tasks\nwa-group: G\nlist: L\n\ntitle: A\nassign: 6281200000000\ndeadline: 2026-01-01 09:00\npublish: yes",
			wantSub: "unexpected field 'publish'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseBulkPayload(tt.text)
			if err == nil {
				t.Fatalf("ParseBulkPayload() expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantSub)
			}
		})
	}
}

func TestParseBulkPayloadTaskNumberingSkipsEmptyChunks(t *testing.T) {
	// Consecutive blank lines between tasks must not shift the task numbering.
	text := "add-tasks\nwa-group: G\nlist: L\n\ntitle: A\nassign: 6281200000000\ndeadline: 2026-01-01 09:00\n\n\n\ntitle: B\nbad line\n"
	_, err := ParseBulkPayload(text)
	if err == nil {
		t.Fatalf("ParseBulkPayload() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "Task #2:") {
		t.Fatalf("error = %q, want task numbered #2", err.Error())
	}
}

func TestParseBulkPayloadNoTaskBlocks(t *testing.T) {
	payload, err := ParseBulkPayload("add-tasks\nwa-group: G\nlist: L")
	if err != nil {
		t.Fatalf("ParseBulkPayload() unexpected error: %v", err)
	}
	if len(payload.Tasks) != 0 {
		t.Fatalf("len(Tasks) = %d, want 0", len(payload.Tasks))
	}
}

func TestBulkCapReplyNamesCapAndUsage(t *testing.T) {
	got := bulkCapReply(51)
	for _, want := range []string{"Command Aborted", "51 task blocks found", "maximum per command is 50", "Usage:\nadd-tasks"} {
		if !strings.Contains(got, want) {
			t.Fatalf("bulkCapReply(51) missing %q; got %q", want, got)
		}
	}
}

func TestMaxBulkTasks(t *testing.T) {
	if maxBulkTasks != 50 {
		t.Fatalf("maxBulkTasks = %d, want 50", maxBulkTasks)
	}

	// Build a payload one over the cap; ParseBulkPayload must not itself reject
	// it (the cap is enforced by the handler), and must expose the full count.
	overPayload := bulkPayload(t, maxBulkTasks+1)
	if len(overPayload.Tasks) <= maxBulkTasks {
		t.Fatalf("len(Tasks) = %d, want > %d", len(overPayload.Tasks), maxBulkTasks)
	}

	// Exactly at the cap must parse and validate cleanly.
	exact, errs := validateBulkTasks("G", "L", bulkPayload(t, maxBulkTasks), nil,
		map[string]bool{"6281200000000": true}, true)
	if len(errs) != 0 {
		t.Fatalf("boundary payload unexpected errors: %v", errs)
	}
	if len(exact) != maxBulkTasks {
		t.Fatalf("len(exact) = %d, want %d", len(exact), maxBulkTasks)
	}
}

// bulkPayload builds a parsed add-tasks payload with n task blocks (headers
// included). Fails the test when the built payload does not parse.
func bulkPayload(t *testing.T, n int) *BulkPayload {
	t.Helper()
	var sb strings.Builder
	sb.WriteString("add-tasks\nwa-group: G\nlist: L")
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&sb, "\n\ntitle: Task %d\nassign: 6281200000000\ndeadline: 2026-01-01 09:00", i)
	}
	payload, err := ParseBulkPayload(sb.String())
	if err != nil {
		t.Fatalf("ParseBulkPayload(built %d blocks) unexpected error: %v", n, err)
	}
	return payload
}

func validateExample(t *testing.T) (*BulkPayload, map[string]bool, map[string]bool, bool) {
	t.Helper()
	payload, err := ParseBulkPayload(bulkExample)
	if err != nil {
		t.Fatalf("ParseBulkPayload() unexpected error: %v", err)
	}
	existing := map[string]bool{"Old task": true}
	participants := map[string]bool{"6281234567890": true, "6289876543210": true}
	return payload, existing, participants, true
}

func TestValidateBulkTasksSuccess(t *testing.T) {
	payload, existing, participants, hasPhone := validateExample(t)
	valid, errs := validateBulkTasks("Tim Marketing", "Weekly Report", payload, existing, participants, hasPhone)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(valid) != 2 {
		t.Fatalf("len(valid) = %d, want 2", len(valid))
	}

	first := valid[0]
	if first.Title != "Collect data" {
		t.Fatalf("Title = %q, want %q", first.Title, "Collect data")
	}
	wantJIDs := []string{bot.FormatJIDString("6281234567890"), bot.FormatJIDString("6289876543210")}
	if fmt.Sprint(first.AssigneeJIDs) != fmt.Sprint(wantJIDs) {
		t.Fatalf("AssigneeJIDs = %v, want %v", first.AssigneeJIDs, wantJIDs)
	}
	if y, m, d := first.Deadline.Date(); y != 2026 || m.String() != "November" || d != 6 {
		t.Errorf("Deadline = %v, want 2026-11-06", first.Deadline)
	}
	if !first.ReminderSpec.Enabled {
		t.Errorf("ReminderSpec = %+v, want enabled for 'yes'", first.ReminderSpec)
	}
	if first.ReminderSpec.Mode() != db.ModeDeadline {
		t.Errorf("Mode() = %q, want %q", first.ReminderSpec.Mode(), db.ModeDeadline)
	}
	if second := valid[1]; second.ReminderSpec.Mode() != db.ModeCron || second.ReminderSpec.Cron == "" {
		t.Errorf("second ReminderSpec = %+v, want cron", second.ReminderSpec)
	}
}

func TestValidateBulkTasksReminderDefault(t *testing.T) {
	payload, err := ParseBulkPayload("add-tasks\nwa-group: G\nlist: L\n\ntitle: No reminder field\nassign: 6281234567890\ndeadline: 2026-11-06 17:00")
	if err != nil {
		t.Fatalf("ParseBulkPayload() unexpected error: %v", err)
	}
	valid, errs := validateBulkTasks("G", "L", payload, nil, map[string]bool{"6281234567890": true}, true)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if !valid[0].ReminderSpec.Enabled {
		t.Fatalf("omitted reminder = %+v, want enabled default (parity with add-task)", valid[0].ReminderSpec)
	}
}

func TestValidateBulkTasksErrors(t *testing.T) {
	base := func(extra string) string {
		return "add-tasks\nwa-group: G\nlist: L\n\n" + extra
	}
	tests := []struct {
		name           string
		text           string
		participants   map[string]bool
		existingTitles map[string]bool
		wantSub        string
	}{
		{
			name:         "missing title",
			text:         base("assign: 6281234567890\ndeadline: 2026-11-06 17:00"),
			participants: map[string]bool{"6281234567890": true},
			wantSub:      `Task #1 (""): missing required field 'title'.`,
		},
		{
			name:         "missing assign",
			text:         base("title: A\ndeadline: 2026-11-06 17:00"),
			participants: map[string]bool{"6281234567890": true},
			wantSub:      `Task #1 ("A"): missing required field 'assign'.`,
		},
		{
			name:         "missing deadline",
			text:         base("title: A\nassign: 6281234567890"),
			participants: map[string]bool{"6281234567890": true},
			wantSub:      `Task #1 ("A"): missing required field 'deadline'.`,
		},
		{
			name:         "invalid deadline format",
			text:         base("title: A\nassign: 6281234567890\ndeadline: 06/11/2026"),
			participants: map[string]bool{"6281234567890": true},
			wantSub:      `Task #1 ("A"): Invalid deadline format. Use YYYY-MM-DD HH:MM.`,
		},
		{
			name:         "invalid reminder spec",
			text:         base("title: A\nassign: 6281234567890\ndeadline: 2026-11-06 17:00\nreminder: every never at 25:99"),
			participants: map[string]bool{"6281234567890": true},
			wantSub:      `Task #1 ("A"): Invalid reminder time: hours must be 0-23 and minutes 0-59.`,
		},
		{
			name:         "no valid assignee phones",
			text:         base("title: A\nassign: , ,\ndeadline: 2026-11-06 17:00"),
			participants: map[string]bool{"6281234567890": true},
			wantSub:      `Task #1 ("A"): No valid assignee phone numbers provided.`,
		},
		{
			name:         "assignee not in group",
			text:         base("title: A\nassign: 6281234567890, 6281111111111\ndeadline: 2026-11-06 17:00"),
			participants: map[string]bool{"6281234567890": true},
			wantSub:      `Task #1 ("A"): Assignee 6281111111111 is not a member of 'G'. Add them first.`,
		},
		{
			name:           "duplicate against existing list title",
			text:           base("title: Old task\nassign: 6281234567890\ndeadline: 2026-11-06 17:00"),
			participants:   map[string]bool{"6281234567890": true},
			existingTitles: map[string]bool{"Old task": true},
			wantSub:        `Task #1 ("Old task"): A task with title 'Old task' already exists in 'L'.`,
		},
		{
			name:         "duplicate within batch",
			text:         base("title: A\nassign: 6281234567890\ndeadline: 2026-11-06 17:00\n\ntitle: A\nassign: 6281234567890\ndeadline: 2026-11-07 17:00"),
			participants: map[string]bool{"6281234567890": true},
			wantSub:      `Task #2 ("A"): A task with title 'A' already exists in 'L'.`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := ParseBulkPayload(tt.text)
			if err != nil {
				t.Fatalf("ParseBulkPayload() unexpected error: %v", err)
			}
			_, errs := validateBulkTasks("G", "L", payload, tt.existingTitles, tt.participants, true)
			if len(errs) == 0 {
				t.Fatalf("expected error containing %q, got none", tt.wantSub)
			}
			if !strings.Contains(errs[0], tt.wantSub) {
				t.Fatalf("errs[0] = %q, want substring %q", errs[0], tt.wantSub)
			}
		})
	}
}

func TestValidateBulkTasksLIDOnlySkipsMembership(t *testing.T) {
	// Parity with bot.IsUserInGroup: when every participant JID is on the
	// LID server (canVerifyMembership=false), membership cannot be verified
	// and must be treated optimistically as passing, even for a phone that is
	// not in the participant map.
	payload, err := ParseBulkPayload("add-tasks\nwa-group: G\nlist: L\n\ntitle: A\nassign: 6281111111111\ndeadline: 2026-11-06 17:00")
	if err != nil {
		t.Fatalf("ParseBulkPayload() unexpected error: %v", err)
	}
	valid, errs := validateBulkTasks("G", "L", payload, nil, map[string]bool{}, false)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(valid) != 1 || valid[0].Title != "A" {
		t.Fatalf("valid = %+v, want task 'A'", valid)
	}
}

func TestValidateBulkTasksPartialFailureIsolatesGoodTasks(t *testing.T) {
	// One bad block must not taint the others; abort-all is enforced upstream
	// by the handler (which discards valid tasks when errs is non-empty).
	payload, err := ParseBulkPayload("add-tasks\nwa-group: G\nlist: L\n\ntitle: Good\nassign: 6281234567890\ndeadline: 2026-11-06 17:00\n\ntitle: Bad\nassign: 6289999999999\ndeadline: 2026-11-06 17:00")
	if err != nil {
		t.Fatalf("ParseBulkPayload() unexpected error: %v", err)
	}
	valid, errs := validateBulkTasks("G", "L", payload, nil, map[string]bool{"6281234567890": true}, true)
	if len(valid) != 1 {
		t.Fatalf("len(valid) = %d, want 1", len(valid))
	}
	if len(errs) != 1 || !strings.Contains(errs[0], `Task #2 ("Bad")`) {
		t.Fatalf("errs = %v, want exactly the Task #2 failure", errs)
	}
}

func TestValidateBulkTasksEmptyPayload(t *testing.T) {
	payload := &BulkPayload{WaGroup: "G", ListName: "L"}
	valid, errs := validateBulkTasks("G", "L", payload, nil, nil, true)
	if len(valid) != 0 || len(errs) != 0 {
		t.Fatalf("valid/errs = %v/%v, want empty/empty", valid, errs)
	}
}
