package db

import "time"

type TaskList struct {
	ID               int64
	Name             string
	GroupJID         string
	AdminJID         string
	Status           string
	CreatedAt        int64
	LastRemindedDate string // "YYYY-MM-DD" in WIB (GMT+7), empty if never reminded
}

func (d *Database) CreateTaskList(name, groupJID, adminJID string) (*TaskList, error) {
	now := time.Now().Unix()
	res, err := d.db.Exec(
		`INSERT INTO task_lists (name, group_jid, admin_jid, status, created_at) VALUES (?, ?, ?, 'unpublished', ?)`,
		name, groupJID, adminJID, now,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &TaskList{ID: id, Name: name, GroupJID: groupJID, AdminJID: adminJID, Status: "unpublished", CreatedAt: now}, nil
}

func (d *Database) GetTaskList(id int64) (*TaskList, error) {
	row := d.db.QueryRow(`SELECT id, name, group_jid, admin_jid, status, created_at, COALESCE(last_reminded_date, '') FROM task_lists WHERE id = ?`, id)
	tl := &TaskList{}
	err := row.Scan(&tl.ID, &tl.Name, &tl.GroupJID, &tl.AdminJID, &tl.Status, &tl.CreatedAt, &tl.LastRemindedDate)
	if err != nil {
		return nil, err
	}
	return tl, nil
}

func (d *Database) GetTaskListByNameAndGroup(name, groupJID string) (*TaskList, error) {
	row := d.db.QueryRow(`SELECT id, name, group_jid, admin_jid, status, created_at, COALESCE(last_reminded_date, '') FROM task_lists WHERE name = ? AND group_jid = ?`, name, groupJID)
	tl := &TaskList{}
	err := row.Scan(&tl.ID, &tl.Name, &tl.GroupJID, &tl.AdminJID, &tl.Status, &tl.CreatedAt, &tl.LastRemindedDate)
	if err != nil {
		return nil, err
	}
	return tl, nil
}

func (d *Database) UpdateListStatus(id int64, status string) error {
	_, err := d.db.Exec(`UPDATE task_lists SET status = ? WHERE id = ?`, status, id)
	return err
}

func (d *Database) DeleteTaskList(id int64) error {
	_, err := d.db.Exec(`DELETE FROM task_lists WHERE id = ?`, id)
	return err
}

func (d *Database) GetListsByGroupAndAdmin(groupJID, adminJID string) ([]TaskList, error) {
	rows, err := d.db.Query(
		`SELECT id, name, group_jid, admin_jid, status, created_at, COALESCE(last_reminded_date, '') FROM task_lists WHERE group_jid = ? AND admin_jid = ?`,
		groupJID, adminJID,
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

func (d *Database) GetActiveListsByGroup(groupJID string) ([]TaskList, error) {
	rows, err := d.db.Query(
		`SELECT id, name, group_jid, admin_jid, status, created_at, COALESCE(last_reminded_date, '') FROM task_lists WHERE group_jid = ? AND status IN ('unpublished', 'active')`,
		groupJID,
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

func (d *Database) GetActiveListsByGroupAndAdmin(groupJID, adminJID string) ([]TaskList, error) {
	rows, err := d.db.Query(
		`SELECT id, name, group_jid, admin_jid, status, created_at, COALESCE(last_reminded_date, '') FROM task_lists WHERE group_jid = ? AND admin_jid = ? AND status IN ('unpublished', 'active')`,
		groupJID, adminJID,
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

func (d *Database) GetAllActiveLists() ([]TaskList, error) {
	rows, err := d.db.Query(
		`SELECT id, name, group_jid, admin_jid, status, created_at, COALESCE(last_reminded_date, '') FROM task_lists WHERE status = 'active'`,
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

func (d *Database) UpdateLastRemindedDate(listID int64, date string) error {
	_, err := d.db.Exec(`UPDATE task_lists SET last_reminded_date = ? WHERE id = ?`, date, listID)
	return err
}
