package db

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func newTestDB(t *testing.T) *Database {
	t.Helper()
	d, err := New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func newTestList(t *testing.T, d *Database) *TaskList {
	t.Helper()
	tl, err := d.CreateTaskList("List A", "1203630@g.us", "admin@s.whatsapp.net")
	if err != nil {
		t.Fatalf("CreateTaskList() error: %v", err)
	}
	return tl
}

// TestUpdateTaskFieldsResetsLastRemindedAt verifies that re-saving a task
// re-arms its one-shot reminder (last_reminded_at -> 0) so an edited deadline
// or "once at" reminder fires again.
func TestUpdateTaskFieldsResetsLastRemindedAt(t *testing.T) {
	tests := []struct {
		name            string
		lastRemindedNow int64 // last_reminded_at before the update
	}{
		{name: "previously reminded task re-arms", lastRemindedNow: 1700000000},
		{name: "never-reminded task stays armed", lastRemindedNow: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newTestDB(t)
			tl := newTestList(t, d)

			task, err := d.CreateTask(tl.ID, "Task A", 1700000100, ModeDeadline, "", 0, []string{"assignee@s.whatsapp.net"})
			if err != nil {
				t.Fatalf("CreateTask() error: %v", err)
			}

			// Simulate a fired reminder, then edit fields.
			if err := d.UpdateTaskLastReminded(task.ID, tt.lastRemindedNow); err != nil {
				t.Fatalf("UpdateTaskLastReminded() error: %v", err)
			}

			err = d.UpdateTaskFields(task.ID, "Task A edited", 1700003600, ModeDeadline, "", 0)
			if err != nil {
				t.Fatalf("UpdateTaskFields() error: %v", err)
			}

			tasks, err := d.GetTasksByList(tl.ID)
			if err != nil {
				t.Fatalf("GetTasksByList() error: %v", err)
			}
			if len(tasks) != 1 {
				t.Fatalf("GetTasksByList() returned %d tasks, want 1", len(tasks))
			}

			got := tasks[0]
			if got.Title != "Task A edited" {
				t.Errorf("Title = %q, want %q", got.Title, "Task A edited")
			}
			if got.Deadline != 1700003600 {
				t.Errorf("Deadline = %d, want %d", got.Deadline, 1700003600)
			}
			if got.LastRemindedAt != 0 {
				t.Errorf("LastRemindedAt = %d, want 0 (re-armed after edit)", got.LastRemindedAt)
			}
		})
	}
}

// TestTaskReminderModeRoundTrip writes every reminder mode and reads it back
// through the positional scans, also checking that normalizeReminder zeroes
// the value columns that don't belong to the mode.
func TestTaskReminderModeRoundTrip(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		cron       string
		at         int64
		wantCron   string
		wantAt     int64
		edits      bool // exercise UpdateTaskFields as well
		wantEdCron string
		wantEdAt   int64
		wantEdMode string
	}{
		{
			name:       "deadline mode drops stray values",
			mode:       ModeDeadline,
			cron:       "0 9 * * *", // stray
			at:         1700000000,  // stray
			wantCron:   "",
			wantAt:     0,
			edits:      true,
			wantEdMode: ModeAt,
			wantEdCron: "",
			wantEdAt:   1700003600,
		},
		{
			name:       "none mode drops stray values",
			mode:       ModeNone,
			cron:       "0 9 * * *", // stray
			at:         1700000000,  // stray
			wantCron:   "",
			wantAt:     0,
			edits:      true,
			wantEdMode: ModeCron,
			wantEdCron: "30 8 * * 2",
			wantEdAt:   0,
		},
		{
			name:       "cron mode keeps cron and zeroes at",
			mode:       ModeCron,
			cron:       "0 20 * * 1",
			at:         1700000000, // stray
			wantCron:   "0 20 * * 1",
			wantAt:     0,
			edits:      true,
			wantEdMode: ModeNone,
			wantEdCron: "",
			wantEdAt:   0,
		},
		{
			name:       "at mode keeps at and zeroes cron",
			mode:       ModeAt,
			cron:       "0 20 * * 1", // stray
			at:         1700000000,
			wantCron:   "",
			wantAt:     1700000000,
			edits:      true,
			wantEdMode: ModeDeadline,
			wantEdCron: "",
			wantEdAt:   0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newTestDB(t)
			tl := newTestList(t, d)

			task, err := d.CreateTask(tl.ID, "Task "+tt.name, 1700000100, tt.mode, tt.cron, tt.at, []string{"assignee@s.whatsapp.net"})
			if err != nil {
				t.Fatalf("CreateTask() error: %v", err)
			}
			if task.ReminderMode != tt.mode {
				t.Errorf("CreateTask() mode = %q, want %q", task.ReminderMode, tt.mode)
			}
			if task.ReminderCron != tt.wantCron {
				t.Errorf("CreateTask() cron = %q, want %q", task.ReminderCron, tt.wantCron)
			}
			if task.ReminderAt != tt.wantAt {
				t.Errorf("CreateTask() at = %d, want %d", task.ReminderAt, tt.wantAt)
			}

			tasks, err := d.GetTasksByList(tl.ID)
			if err != nil {
				t.Fatalf("GetTasksByList() error: %v", err)
			}
			got := tasks[0]
			if got.ReminderMode != tt.mode || got.ReminderCron != tt.wantCron || got.ReminderAt != tt.wantAt {
				t.Errorf("GetTasksByList() = mode %q cron %q at %d, want mode %q cron %q at %d",
					got.ReminderMode, got.ReminderCron, got.ReminderAt, tt.mode, tt.wantCron, tt.wantAt)
			}

			single, err := d.GetTaskAtPosition(tl.ID, got.Position)
			if err != nil {
				t.Fatalf("GetTaskAtPosition() error: %v", err)
			}
			if single.ReminderMode != tt.mode || single.ReminderCron != tt.wantCron || single.ReminderAt != tt.wantAt {
				t.Errorf("GetTaskAtPosition() = mode %q cron %q at %d, want mode %q cron %q at %d",
					single.ReminderMode, single.ReminderCron, single.ReminderAt, tt.mode, tt.wantCron, tt.wantAt)
			}

			if !tt.edits {
				return
			}
			if err := d.UpdateTaskFields(task.ID, task.Title, 1700003600, tt.wantEdMode, tt.wantEdCron, tt.wantEdAt); err != nil {
				t.Fatalf("UpdateTaskFields() error: %v", err)
			}
			edited, err := d.GetTaskAtPosition(tl.ID, got.Position)
			if err != nil {
				t.Fatalf("GetTaskAtPosition() after edit: %v", err)
			}
			if edited.ReminderMode != tt.wantEdMode || edited.ReminderCron != tt.wantEdCron || edited.ReminderAt != tt.wantEdAt {
				t.Errorf("after edit = mode %q cron %q at %d, want mode %q cron %q at %d",
					edited.ReminderMode, edited.ReminderCron, edited.ReminderAt, tt.wantEdMode, tt.wantEdCron, tt.wantEdAt)
			}
		})
	}
}

// TestMigrationV5Backfill builds a genuine pre-V5 schema (V1..V4), seeds rows
// with the legacy sentinel encoding (reminder bool + cron/at), then applies V5
// and asserts every row decodes to the same behavior taskDue used to infer.
func TestMigrationV5Backfill(t *testing.T) {
	sqlDB, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	defer sqlDB.Close()

	for _, m := range []string{migrationV1, migrationV2, migrationV3, migrationV4} {
		if _, err := sqlDB.Exec(m); err != nil {
			t.Fatalf("pre-V5 migration failed: %v", err)
		}
	}

	if _, err := sqlDB.Exec(
		`INSERT INTO task_lists (name, group_jid, admin_jid, status, created_at) VALUES ('List A', '1203630@g.us', 'admin@s.whatsapp.net', 'active', 1700000000)`,
	); err != nil {
		t.Fatalf("CreateTaskList insert failed: %v", err)
	}

	rows := []struct {
		name       string
		legacyRem  int // dropped `reminder` column
		legacyCron string
		legacyAt   int64
		wantMode   string
	}{
		{name: "yes -> deadline", legacyRem: 1, wantMode: ModeDeadline},
		{name: "no -> none", legacyRem: 0, wantMode: ModeNone},
		{name: "cron row -> cron", legacyRem: 1, legacyCron: "0 20 * * 1", wantMode: ModeCron},
		{name: "at row -> at", legacyRem: 1, legacyAt: 1700000000, wantMode: ModeAt},
		{name: "none with stray values stays none", legacyRem: 0, legacyCron: "0 20 * * 1", legacyAt: 1700000000, wantMode: ModeNone},
		{name: "reminder=1 with cron and at -> at (taskDue precedence)", legacyRem: 1, legacyCron: "0 20 * * 1", legacyAt: 1700000000, wantMode: ModeAt},
	}
	for i, r := range rows {
		if _, err := sqlDB.Exec(
			`INSERT INTO tasks (task_list_id, position, title, status, deadline, reminder, reminder_cron, reminder_at, last_reminded_at, created_at)
			 VALUES (1, ?, ?, 'todo', 1700000100, ?, ?, ?, 0, 1700000000)`,
			i+1, r.name, r.legacyRem, r.legacyCron, r.legacyAt,
		); err != nil {
			t.Fatalf("legacy insert %q failed: %v", r.name, err)
		}
	}

	if _, err := sqlDB.Exec(migrationV5); err != nil {
		t.Fatalf("migrationV5 failed: %v", err)
	}

	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			var mode string
			if err := sqlDB.QueryRow(`SELECT reminder_mode FROM tasks WHERE title = ?`, r.name).Scan(&mode); err != nil {
				t.Fatalf("query mode: %v", err)
			}
			if mode != r.wantMode {
				t.Errorf("reminder_mode = %q, want %q", mode, r.wantMode)
			}
		})
	}

	t.Run("reminder column dropped", func(t *testing.T) {
		var count int
		err := sqlDB.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('tasks') WHERE name = 'reminder'`).Scan(&count)
		if err != nil {
			t.Fatalf("pragma query: %v", err)
		}
		if count != 0 {
			t.Errorf("tasks.reminder still exists after V5")
		}
	})
}
