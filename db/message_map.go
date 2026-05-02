package db

import "time"

func (d *Database) SaveMessageMapping(messageID string, taskListID int64, groupJID string) error {
	now := time.Now().Unix()
	_, err := d.db.Exec(
		`INSERT OR REPLACE INTO message_map (message_id, task_list_id, group_jid, created_at) VALUES (?, ?, ?, ?)`,
		messageID, taskListID, groupJID, now,
	)
	return err
}

func (d *Database) GetTaskListByMessageID(messageID string) (int64, error) {
	var taskListID int64
	err := d.db.QueryRow(`SELECT task_list_id FROM message_map WHERE message_id = ?`, messageID).Scan(&taskListID)
	if err != nil {
		return 0, err
	}
	return taskListID, nil
}

func (d *Database) DeleteMessageMapByList(taskListID int64) error {
	_, err := d.db.Exec(`DELETE FROM message_map WHERE task_list_id = ?`, taskListID)
	return err
}

func (d *Database) UpsertGroup(jid, name string) error {
	now := time.Now().Unix()
	_, err := d.db.Exec(
		`INSERT OR REPLACE INTO wa_groups (jid, name, updated_at) VALUES (?, ?, ?)`,
		jid, name, now,
	)
	return err
}

func (d *Database) DeleteGroup(jid string) error {
	_, err := d.db.Exec(`DELETE FROM wa_groups WHERE jid = ?`, jid)
	return err
}

func (d *Database) GetGroupsByName(name string) ([]WAGroup, error) {
	rows, err := d.db.Query(`SELECT jid, name, updated_at FROM wa_groups WHERE name = ? COLLATE NOCASE`, name)
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

func (d *Database) GetGroupName(jid string) string {
	var name string
	d.db.QueryRow(`SELECT name FROM wa_groups WHERE jid = ?`, jid).Scan(&name)
	return name
}

type WAGroup struct {
	JID       string
	Name      string
	UpdatedAt int64
}
