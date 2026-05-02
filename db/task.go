package db

import (
	"database/sql"
	"time"
)

type Task struct {
	ID         int64
	TaskListID int64
	Position   int
	Title      string
	Status     string
	Deadline   int64
	Reminder   bool
	CreatedAt  int64
	Assignees  []TaskAssignee
}

type TaskAssignee struct {
	ID          int64
	TaskID      int64
	AssigneeJID string
	LeftGroup   bool
}

func (a TaskAssignee) Phone() string {
	jid := a.AssigneeJID
	for i, c := range jid {
		if c == '@' {
			return jid[:i]
		}
	}
	return jid
}

func (d *Database) CreateTask(taskListID int64, title string, deadline int64, reminder bool, assigneeJIDs []string) (*Task, error) {
	tx, err := d.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var maxPos int
	row := tx.QueryRow(`SELECT COALESCE(MAX(position), 0) FROM tasks WHERE task_list_id = ?`, taskListID)
	row.Scan(&maxPos)
	position := maxPos + 1

	now := time.Now().Unix()
	reminderInt := 0
	if reminder {
		reminderInt = 1
	}

	res, err := tx.Exec(
		`INSERT INTO tasks (task_list_id, position, title, status, deadline, reminder, created_at) VALUES (?, ?, ?, 'todo', ?, ?, ?)`,
		taskListID, position, title, deadline, reminderInt, now,
	)
	if err != nil {
		return nil, err
	}
	taskID, _ := res.LastInsertId()

	for _, jid := range assigneeJIDs {
		_, err = tx.Exec(`INSERT INTO task_assignees (task_id, assignee_jid, left_group) VALUES (?, ?, 0)`, taskID, jid)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &Task{
		ID:         taskID,
		TaskListID: taskListID,
		Position:   position,
		Title:      title,
		Status:     "todo",
		Deadline:   deadline,
		Reminder:   reminder,
		CreatedAt:  now,
	}, nil
}

func (d *Database) GetTasksByList(taskListID int64) ([]Task, error) {
	rows, err := d.db.Query(
		`SELECT id, task_list_id, position, title, status, deadline, reminder, created_at FROM tasks WHERE task_list_id = ? ORDER BY position`,
		taskListID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		var rem int
		rows.Scan(&t.ID, &t.TaskListID, &t.Position, &t.Title, &t.Status, &t.Deadline, &rem, &t.CreatedAt)
		t.Reminder = rem == 1
		tasks = append(tasks, t)
	}

	for i := range tasks {
		tasks[i].Assignees, _ = d.getAssigneesForTask(tasks[i].ID)
	}
	return tasks, nil
}

func (d *Database) getAssigneesForTask(taskID int64) ([]TaskAssignee, error) {
	rows, err := d.db.Query(
		`SELECT id, task_id, assignee_jid, left_group FROM task_assignees WHERE task_id = ?`, taskID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assignees []TaskAssignee
	for rows.Next() {
		var a TaskAssignee
		var left int
		rows.Scan(&a.ID, &a.TaskID, &a.AssigneeJID, &left)
		a.LeftGroup = left == 1
		assignees = append(assignees, a)
	}
	return assignees, nil
}

func (d *Database) GetTaskCountByList(taskListID int64) (int, error) {
	var count int
	err := d.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE task_list_id = ?`, taskListID).Scan(&count)
	return count, err
}

func (d *Database) UpdateTaskStatus(taskListID int64, position int, status string) error {
	_, err := d.db.Exec(
		`UPDATE tasks SET status = ? WHERE task_list_id = ? AND position = ?`,
		status, taskListID, position,
	)
	return err
}

func (d *Database) UpdateTaskStatusTx(tx *sql.Tx, taskListID int64, position int, status string) error {
	_, err := tx.Exec(
		`UPDATE tasks SET status = ? WHERE task_list_id = ? AND position = ?`,
		status, taskListID, position,
	)
	return err
}

func (d *Database) DeleteTask(taskListID int64, position int) (string, error) {
	var title string
	err := d.db.QueryRow(`SELECT title FROM tasks WHERE task_list_id = ? AND position = ?`, taskListID, position).Scan(&title)
	if err != nil {
		return "", err
	}

	tx, err := d.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`DELETE FROM tasks WHERE task_list_id = ? AND position = ?`, taskListID, position)
	if err != nil {
		return "", err
	}

	rows, err := tx.Query(`SELECT id FROM tasks WHERE task_list_id = ? ORDER BY position`, taskListID)
	if err != nil {
		return "", err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()

	for i, id := range ids {
		_, err = tx.Exec(`UPDATE tasks SET position = ? WHERE id = ?`, i+1, id)
		if err != nil {
			return "", err
		}
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}
	return title, nil
}

func (d *Database) TaskExistsAtPosition(taskListID int64, position int) bool {
	var count int
	d.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE task_list_id = ? AND position = ?`, taskListID, position).Scan(&count)
	return count > 0
}

func (d *Database) GetTaskAtPosition(taskListID int64, position int) (*Task, error) {
	row := d.db.QueryRow(
		`SELECT id, task_list_id, position, title, status, deadline, reminder, created_at FROM tasks WHERE task_list_id = ? AND position = ?`,
		taskListID, position,
	)
	var t Task
	var rem int
	err := row.Scan(&t.ID, &t.TaskListID, &t.Position, &t.Title, &t.Status, &t.Deadline, &rem, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	t.Reminder = rem == 1
	t.Assignees, _ = d.getAssigneesForTask(t.ID)
	return &t, nil
}

func (d *Database) MarkAssigneeLeft(groupJID, assigneeJID string) ([]Task, error) {
	rows, err := d.db.Query(`
		SELECT t.id, t.task_list_id, t.position, t.title, t.status, t.deadline, t.reminder, t.created_at
		FROM tasks t
		JOIN task_lists tl ON t.task_list_id = tl.id
		JOIN task_assignees ta ON ta.task_id = t.id
		WHERE tl.group_jid = ? AND ta.assignee_jid = ? AND ta.left_group = 0
	`, groupJID, assigneeJID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		var rem int
		rows.Scan(&t.ID, &t.TaskListID, &t.Position, &t.Title, &t.Status, &t.Deadline, &rem, &t.CreatedAt)
		t.Reminder = rem == 1
		tasks = append(tasks, t)
	}

	_, err = d.db.Exec(`
		UPDATE task_assignees SET left_group = 1
		WHERE assignee_jid = ? AND task_id IN (
			SELECT t.id FROM tasks t JOIN task_lists tl ON t.task_list_id = tl.id WHERE tl.group_jid = ?
		)
	`, assigneeJID, groupJID)

	return tasks, err
}

func (d *Database) SetAssigneeLeft(assigneeID int64, left bool) error {
	val := 0
	if left {
		val = 1
	}
	_, err := d.db.Exec(`UPDATE task_assignees SET left_group = ? WHERE id = ?`, val, assigneeID)
	return err
}

func (d *Database) GetAssigneesByGroup(groupJID string, taskListID int64) ([]TaskAssignee, error) {
	rows, err := d.db.Query(`
		SELECT ta.id, ta.task_id, ta.assignee_jid, ta.left_group
		FROM task_assignees ta
		JOIN tasks t ON ta.task_id = t.id
		WHERE t.task_list_id = ?
	`, taskListID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assignees []TaskAssignee
	for rows.Next() {
		var a TaskAssignee
		var left int
		rows.Scan(&a.ID, &a.TaskID, &a.AssigneeJID, &left)
		a.LeftGroup = left == 1
		assignees = append(assignees, a)
	}
	return assignees, nil
}
