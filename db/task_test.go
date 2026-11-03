package db

import (
	"strings"
	"testing"
)

func TestCreateTasksSuccess(t *testing.T) {
	d := newTestDB(t)
	tl := newTestList(t, d)

	rows := []BulkTaskRow{
		{
			Title:        "Collect data",
			Deadline:     1700000100,
			ReminderMode: ModeDeadline,
			AssigneeJIDs: []string{"assignee1@s.whatsapp.net", "assignee2@s.whatsapp.net"},
		},
		{
			Title:        "Compile slides",
			Deadline:     1700000200,
			ReminderMode: ModeCron,
			ReminderCron: "0 10 * * 5",
			AssigneeJIDs: []string{"assignee1@s.whatsapp.net"},
		},
		{
			Title:        "No reminder",
			Deadline:     1700000300,
			ReminderMode: ModeAt,
			ReminderAt:   1700000250,
			AssigneeJIDs: nil,
		},
	}

	created, err := d.CreateTasks(tl.ID, rows)
	if err != nil {
		t.Fatalf("CreateTasks() error: %v", err)
	}
	if len(created) != len(rows) {
		t.Fatalf("len(created) = %d, want %d", len(created), len(rows))
	}

	// Positions are sequential starting at 1 in block order.
	for i, task := range created {
		if task.Position != i+1 {
			t.Errorf("created[%d].Position = %d, want %d", i, task.Position, i+1)
		}
		if task.TaskListID != tl.ID {
			t.Errorf("created[%d].TaskListID = %d, want %d", i, task.TaskListID, tl.ID)
		}
		if task.Status != "todo" {
			t.Errorf("created[%d].Status = %q, want todo", i, task.Status)
		}
	}

	stored, err := d.GetTasksByList(tl.ID)
	if err != nil {
		t.Fatalf("GetTasksByList() error: %v", err)
	}
	if len(stored) != 3 {
		t.Fatalf("len(stored) = %d, want 3", len(stored))
	}
	wantDeadlines := []int64{1700000100, 1700000200, 1700000300}
	for i, ts := range stored {
		if ts.Deadline != wantDeadlines[i] {
			t.Errorf("stored[%d].Deadline = %d, want %d", i, ts.Deadline, wantDeadlines[i])
		}
	}
	if stored[0].ReminderMode != ModeDeadline || stored[1].ReminderMode != ModeCron || stored[2].ReminderMode != ModeAt {
		t.Fatalf("reminder modes = %q/%q/%q, want deadline/cron/at", stored[0].ReminderMode, stored[1].ReminderMode, stored[2].ReminderMode)
	}
	if stored[0].ReminderCron != "" || stored[0].ReminderAt != 0 {
		t.Errorf("normalizeReminder not applied to deadline task: cron=%q at=%d", stored[0].ReminderCron, stored[0].ReminderAt)
	}
	if stored[2].ReminderCron != "" || stored[2].ReminderAt != 1700000250 {
		t.Errorf("at-mode task lost ReminderAt: cron=%q at=%d", stored[2].ReminderCron, stored[2].ReminderAt)
	}
	if len(stored[0].Assignees) != 2 {
		t.Fatalf("stored[0] assignees = %d, want 2", len(stored[0].Assignees))
	}
	if len(stored[2].Assignees) != 0 {
		t.Fatalf("stored[2] assignees = %d, want 0", len(stored[2].Assignees))
	}
}

func TestCreateTasksContinuesPositions(t *testing.T) {
	d := newTestDB(t)
	tl := newTestList(t, d)

	if _, err := d.CreateTask(tl.ID, "Single", 1700000000, ModeDeadline, "", 0, nil); err != nil {
		t.Fatalf("CreateTask() error: %v", err)
	}

	created, err := d.CreateTasks(tl.ID, []BulkTaskRow{
		{Title: "Bulk one", Deadline: 1700000100, ReminderMode: ModeDeadline},
		{Title: "Bulk two", Deadline: 1700000200, ReminderMode: ModeDeadline},
	})
	if err != nil {
		t.Fatalf("CreateTasks() error: %v", err)
	}
	if created[0].Position != 2 || created[1].Position != 3 {
		t.Fatalf("positions = %d/%d, want 2/3", created[0].Position, created[1].Position)
	}
}

// TestCreateTasksRollsBackOnViolation exercises the abort-all guarantee at the
// storage layer: any constraint failure (here a duplicate title inside the
// batch) leaves the list untouched.
func TestCreateTasksRollsBackOnViolation(t *testing.T) {
	d := newTestDB(t)
	tl := newTestList(t, d)

	if _, err := d.CreateTask(tl.ID, "Existing", 1700000000, ModeDeadline, "", 0, nil); err != nil {
		t.Fatalf("CreateTask() error: %v", err)
	}

	_, err := d.CreateTasks(tl.ID, []BulkTaskRow{
		{Title: "Good", Deadline: 1700000100, ReminderMode: ModeDeadline},
		{Title: "Existing", Deadline: 1700000200, ReminderMode: ModeDeadline}, // UNIQUE violation
	})
	if err == nil {
		t.Fatalf("CreateTasks() expected UNIQUE violation error, got nil")
	}
	if !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("err = %v, want UNIQUE constraint error", err)
	}

	var count int
	if err := d.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE task_list_id = ?`, tl.ID).Scan(&count); err != nil {
		t.Fatalf("COUNT query error: %v", err)
	}
	if count != 1 {
		t.Fatalf("task count after failed batch = %d, want 1 (rollback)", count)
	}
}

// TestCreateTasksRollsBackOnAssigneeViolation covers the same guarantee when a
// duplicate assignee row fails inside the transaction.
func TestCreateTasksRollsBackOnAssigneeViolation(t *testing.T) {
	d := newTestDB(t)
	tl := newTestList(t, d)

	_, err := d.CreateTasks(tl.ID, []BulkTaskRow{
		{
			Title:        "Dup assignee",
			Deadline:     1700000100,
			ReminderMode: ModeDeadline,
			AssigneeJIDs: []string{"a@s.whatsapp.net", "a@s.whatsapp.net"}, // UNIQUE(task_id, assignee_jid)
		},
	})
	if err == nil {
		t.Fatalf("CreateTasks() expected assignee UNIQUE violation error, got nil")
	}

	var count int
	if err := d.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE task_list_id = ?`, tl.ID).Scan(&count); err != nil {
		t.Fatalf("COUNT query error: %v", err)
	}
	if count != 0 {
		t.Fatalf("task count after failed batch = %d, want 0 (rollback)", count)
	}
}

func TestCreateTasksEmptyInput(t *testing.T) {
	d := newTestDB(t)
	tl := newTestList(t, d)

	created, err := d.CreateTasks(tl.ID, nil)
	if err == nil {
		t.Fatalf("CreateTasks(nil) expected error, got nil")
	}
	if created != nil {
		t.Fatalf("CreateTasks(nil) created = %v, want nil", created)
	}
	if !strings.Contains(err.Error(), "no tasks") {
		t.Fatalf("err = %v, want 'no tasks' hint", err)
	}
}

func TestGetTaskTitlesByList(t *testing.T) {
	d := newTestDB(t)
	tl := newTestList(t, d)

	if _, err := d.CreateTask(tl.ID, "Alpha", 1700000000, ModeDeadline, "", 0, nil); err != nil {
		t.Fatalf("CreateTask() error: %v", err)
	}
	if _, err := d.CreateTasks(tl.ID, []BulkTaskRow{
		{Title: "Beta", Deadline: 1700000100, ReminderMode: ModeDeadline},
	}); err != nil {
		t.Fatalf("CreateTasks() error: %v", err)
	}

	titles, err := d.GetTaskTitlesByList(tl.ID)
	if err != nil {
		t.Fatalf("GetTaskTitlesByList() error: %v", err)
	}
	if len(titles) != 2 || !titles["Alpha"] || !titles["Beta"] {
		t.Fatalf("titles = %v, want Alpha and Beta", titles)
	}

	// Empty list returns an empty (non-nil) map without error.
	other, err := d.CreateTaskList("List B", "1203631@g.us", "admin@s.whatsapp.net")
	if err != nil {
		t.Fatalf("CreateTaskList() error: %v", err)
	}
	titles, err = d.GetTaskTitlesByList(other.ID)
	if err != nil {
		t.Fatalf("GetTaskTitlesByList(empty) error: %v", err)
	}
	if len(titles) != 0 {
		t.Fatalf("titles = %v, want empty", titles)
	}
}
