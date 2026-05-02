package db

import "database/sql"

func (d *Database) migrate() error {
	_, err := d.db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER PRIMARY KEY)`)
	if err != nil {
		return err
	}

	var version int
	row := d.db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`)
	row.Scan(&version)

	migrations := []string{
		migrationV1,
	}

	for i, m := range migrations {
		v := i + 1
		if v <= version {
			continue
		}
		tx, err := d.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(m); err != nil {
			tx.Rollback()
			return err
		}
		if _, err := tx.Exec(`INSERT INTO schema_version (version) VALUES (?)`, v); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

var migrationV1 = `
CREATE TABLE wa_groups (
    jid TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE task_lists (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    group_jid TEXT NOT NULL,
    admin_jid TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'unpublished',
    created_at INTEGER NOT NULL,
    UNIQUE(name, group_jid)
);

CREATE TABLE tasks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_list_id INTEGER NOT NULL,
    position INTEGER NOT NULL,
    title TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'todo',
    deadline INTEGER NOT NULL,
    reminder INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    FOREIGN KEY (task_list_id) REFERENCES task_lists(id) ON DELETE CASCADE,
    UNIQUE(task_list_id, title),
    UNIQUE(task_list_id, position)
);

CREATE TABLE task_assignees (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id INTEGER NOT NULL,
    assignee_jid TEXT NOT NULL,
    left_group INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
    UNIQUE(task_id, assignee_jid)
);

CREATE TABLE message_map (
    message_id TEXT PRIMARY KEY,
    task_list_id INTEGER NOT NULL,
    group_jid TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    FOREIGN KEY (task_list_id) REFERENCES task_lists(id) ON DELETE CASCADE
);

CREATE INDEX idx_task_lists_group ON task_lists(group_jid);
CREATE INDEX idx_task_lists_admin ON task_lists(admin_jid);
CREATE INDEX idx_task_lists_status ON task_lists(status);
CREATE INDEX idx_tasks_list ON tasks(task_list_id, position);
CREATE INDEX idx_task_assignees_jid ON task_assignees(assignee_jid);
CREATE INDEX idx_message_map_list ON message_map(task_list_id);
`

func (d *Database) BeginTx() (*sql.Tx, error) {
	return d.db.Begin()
}
