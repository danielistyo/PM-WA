package db

import "time"

func (d *Database) CreateWebToken(token, jid string, expiresAt int64) error {
	_, err := d.db.Exec(
		`INSERT INTO web_tokens (token, jid, expires_at, used) VALUES (?, ?, ?, 0)`,
		token, jid, expiresAt,
	)
	return err
}

func (d *Database) ConsumeWebToken(token string) (string, bool) {
	tx, err := d.db.Begin()
	if err != nil {
		return "", false
	}
	defer tx.Rollback()

	var jid string
	var expiresAt int64
	var used int
	err = tx.QueryRow(`SELECT jid, expires_at, used FROM web_tokens WHERE token = ?`, token).Scan(&jid, &expiresAt, &used)
	if err != nil {
		return "", false
	}
	if used != 0 || time.Now().Unix() > expiresAt {
		return "", false
	}
	if _, err := tx.Exec(`UPDATE web_tokens SET used = 1 WHERE token = ?`, token); err != nil {
		return "", false
	}
	if err := tx.Commit(); err != nil {
		return "", false
	}
	return jid, true
}

func (d *Database) CreateWebSession(id, jid, csrf string, expiresAt int64) error {
	_, err := d.db.Exec(
		`INSERT INTO web_sessions (id, jid, csrf, expires_at) VALUES (?, ?, ?, ?)`,
		id, jid, csrf, expiresAt,
	)
	return err
}

func (d *Database) GetWebSession(id string) (jid string, csrf string, ok bool) {
	var expiresAt int64
	err := d.db.QueryRow(`SELECT jid, csrf, expires_at FROM web_sessions WHERE id = ?`, id).Scan(&jid, &csrf, &expiresAt)
	if err != nil {
		return "", "", false
	}
	if time.Now().Unix() > expiresAt {
		return "", "", false
	}
	return jid, csrf, true
}

func (d *Database) DeleteWebSession(id string) error {
	_, err := d.db.Exec(`DELETE FROM web_sessions WHERE id = ?`, id)
	return err
}

func (d *Database) DeleteExpiredWebAuth() error {
	now := time.Now().Unix()
	if _, err := d.db.Exec(`DELETE FROM web_tokens WHERE expires_at < ?`, now); err != nil {
		return err
	}
	_, err := d.db.Exec(`DELETE FROM web_sessions WHERE expires_at < ?`, now)
	return err
}

func (d *Database) GetTaskListsByAdmin(adminJID string) ([]TaskList, error) {
	rows, err := d.db.Query(
		`SELECT id, name, group_jid, admin_jid, status, created_at, COALESCE(last_reminded_date, '') FROM task_lists WHERE admin_jid = ? ORDER BY created_at DESC`,
		adminJID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var lists []TaskList
	for rows.Next() {
		var tl TaskList
		rows.Scan(&tl.ID, &tl.Name, &tl.GroupJID, &tl.AdminJID, &tl.Status, &tl.CreatedAt, &tl.LastRemindedDate)
		lists = append(lists, tl)
	}
	return lists, nil
}

func (d *Database) GetAllGroups() ([]WAGroup, error) {
	rows, err := d.db.Query(`SELECT jid, name, updated_at FROM wa_groups ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []WAGroup
	for rows.Next() {
		var g WAGroup
		rows.Scan(&g.JID, &g.Name, &g.UpdatedAt)
		groups = append(groups, g)
	}
	return groups, nil
}

func (d *Database) UpdateTaskFields(taskID int64, title string, deadline int64, reminder bool, reminderCron string, reminderAt int64) error {
	reminderInt := 0
	if reminder {
		reminderInt = 1
	}
	_, err := d.db.Exec(
		`UPDATE tasks SET title = ?, deadline = ?, reminder = ?, reminder_cron = ?, reminder_at = ? WHERE id = ?`,
		title, deadline, reminderInt, reminderCron, reminderAt, taskID,
	)
	return err
}

func (d *Database) ReplaceTaskAssignees(taskID int64, jids []string) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM task_assignees WHERE task_id = ?`, taskID); err != nil {
		return err
	}
	for _, jid := range jids {
		if _, err := tx.Exec(`INSERT INTO task_assignees (task_id, assignee_jid, left_group) VALUES (?, ?, 0)`, taskID, jid); err != nil {
			return err
		}
	}
	return tx.Commit()
}
