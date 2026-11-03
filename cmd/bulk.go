package cmd

import (
	"fmt"
	"strings"
	"time"

	"pm-wa/bot"
	"pm-wa/db"
)

// maxBulkTasks is the hard cap of task blocks accepted by one add-tasks command.
const maxBulkTasks = 50

// taskFieldOrder is the canonical task-field list; unknown-field checks and
// header-phase detection both read from it so a new field is added once.
var taskFieldOrder = []string{"title", "assign", "deadline", "reminder"}

var bulkTaskFieldNames = func() map[string]bool {
	m := make(map[string]bool, len(taskFieldOrder))
	for _, k := range taskFieldOrder {
		m[k] = true
	}
	return m
}()

var bulkHeaderFieldNames = map[string]bool{
	"wa-group": true,
	"list":     true,
}

// bulkUsage is the reply sent when the payload has no task blocks or exceeds the cap.
const bulkUsage = "Usage:\nadd-tasks\nwa-group: {WA Group}\nlist: {List Name}\n\n(one block per task, separated by a blank line)\ntitle: {Title}\nassign: {Phone1, Phone2}\ndeadline: {YYYY-MM-DD HH:MM}\nreminder: {yes / no / 'every monday at 20:00' / '2029-03-29 20:00'}"

// bulkCapReply is the abort reply for payloads over the 50-task cap. Pure so
// the cap wording and the usage text are testable without a WhatsApp client.
func bulkCapReply(n int) string {
	return fmt.Sprintf("Command Aborted: %d task blocks found, but the maximum per command is %d. %s", n, maxBulkTasks, bulkUsage)
}

// BulkPayload is the parsed shape of an add-tasks message: one set of header
// fields (wa-group, list) followed by blank-line-separated task blocks.
type BulkPayload struct {
	WaGroup  string
	ListName string
	Tasks    []map[string]string // raw key/value fields per task block
}

// anyTaskField reports the first task field present in a block, in canonical
// field order so error messages are deterministic.
func anyTaskField(fields map[string]string) (string, bool) {
	for _, k := range []string{"title", "assign", "deadline", "reminder"} {
		if _, ok := fields[k]; ok {
			return k, true
		}
	}
	return "", false
}

func hasListField(fields map[string]string) bool {
	_, ok := fields["list"]
	return ok
}

// BulkTask is one fully validated task ready for storage.
type BulkTask struct {
	Title        string
	AssigneeJIDs []string
	Deadline     time.Time
	ReminderSpec ReminderSpec
}

// ParseBulkPayload splits an add-tasks message into header fields and task
// blocks. Line 1 is the command word and is ignored; the remaining lines up to
// the first blank line are header key/value pairs; each following
// blank-line-separated chunk is one task block. Tasks are numbered by their
// order among non-empty task chunks, which is the numbering used in replies.
func ParseBulkPayload(text string) (*BulkPayload, error) {
	chunks := strings.Split(strings.TrimSpace(text), "\n\n")
	if len(chunks) == 0 {
		return nil, fmt.Errorf("Missing required fields. %s", bulkUsage)
	}

	p := &BulkPayload{}
	headerPhase := true
	for i, chunk := range chunks {
		lines := strings.Split(chunk, "\n")
		fields := make(map[string]string)
		start := 0
		if i == 0 {
			start = 1 // skip the command word
		}
		badLine := ""
		hasBad := false
		for j := start; j < len(lines); j++ {
			line := strings.TrimSpace(lines[j])
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				// Deferred: the error message depends on how this chunk is
				// classified (header vs task), which is only known after the
				// remaining lines are collected.
				badLine, hasBad = line, true
				break
			}
			key := strings.TrimSpace(strings.ToLower(parts[0]))
			fields[key] = strings.TrimSpace(parts[1])
		}

		if len(fields) == 0 && !hasBad {
			continue // consecutive blank lines produce empty chunks; treat as separators
		}

		if headerPhase {
			taskKey, ok := anyTaskField(fields)
			if ok {
				// A chunk mixing header and task fields (e.g. a missing blank
				// line after the headers) is ambiguous and rejected.
				if _, hasHeader := fields["wa-group"]; hasHeader || hasListField(fields) {
					return nil, fmt.Errorf("Unexpected field '%s' in the header: separate the task blocks from the wa-group/list header with a blank line. %s", taskKey, bulkUsage)
				}
				// First task chunk ends the header phase.
				headerPhase = false
			} else {
				for k := range fields {
					if !bulkHeaderFieldNames[k] {
						return nil, fmt.Errorf("Unexpected field '%s' in the header. %s", k, bulkUsage)
					}
				}
				// Merge only the keys this chunk actually carries, so header
				// fields split across several chunks survive.
				if v, ok := fields["wa-group"]; ok {
					p.WaGroup = v
				}
				if v, ok := fields["list"]; ok {
					p.ListName = v
				}
				if hasBad {
					return nil, fmt.Errorf("Unexpected format near '%s'. %s", badLine, bulkUsage)
				}
				continue
			}
		}

		// Task chunk: numbering counts tasks so far, empty chunks don't shift it.
		taskNum := len(p.Tasks) + 1
		if hasBad {
			return nil, fmt.Errorf("Task #%d: line '%s' is not in 'key: value' format.", taskNum, badLine)
		}
		for k := range fields {
			if !bulkTaskFieldNames[k] {
				return nil, fmt.Errorf("Task #%d: unexpected field '%s'. Use title, assign, deadline, reminder.", taskNum, k)
			}
		}
		p.Tasks = append(p.Tasks, fields)
	}
	return p, nil
}

// validateBulkTasks validates every task block without touching the database.
// Assignee membership is checked against a pre-fetched participant set; groups
// whose participants are LID-primary (canVerifyMembership=false, the rule
// bot.IsUserInGroup applies) skip the check optimistically. The participant
// set also includes phone-number match keys, so bulk may accept an assignee
// that add-task's JID-only match would reject — that is intentional: a real
// member is never falsely aborted.
// It returns the valid tasks plus one human-readable error per rejected task;
// when errs is non-empty the caller must add nothing.
func validateBulkTasks(waGroup, listName string, payload *BulkPayload, existingTitles map[string]bool, participants map[string]bool, canVerifyMembership bool) ([]BulkTask, []string) {
	var errs []string
	taskErr := func(idx int, title, format string, args ...any) {
		errs = append(errs, fmt.Sprintf("Task #%d (%q): %s", idx, title, fmt.Sprintf(format, args...)))
	}

	var valid []BulkTask
	seen := make(map[string]bool, len(payload.Tasks))
	for i, fields := range payload.Tasks {
		idx := i + 1

		title := fields["title"]
		if title == "" {
			taskErr(idx, "", "missing required field 'title'.")
			continue
		}

		assignStr := fields["assign"]
		if assignStr == "" {
			taskErr(idx, title, "missing required field 'assign'.")
			continue
		}

		deadlineStr := fields["deadline"]
		if deadlineStr == "" {
			taskErr(idx, title, "missing required field 'deadline'.")
			continue
		}

		deadline, err := time.ParseInLocation("2006-01-02 15:04", deadlineStr, gmt7)
		if err != nil {
			taskErr(idx, title, "Invalid deadline format. Use YYYY-MM-DD HH:MM.")
			continue
		}

		reminderSpec, err := ParseReminderSpec(fields["reminder"], gmt7)
		if err != nil {
			taskErr(idx, title, "%s", err.Error())
			continue
		}

		phones := parsePhoneList(assignStr)
		if len(phones) == 0 {
			taskErr(idx, title, "No valid assignee phone numbers provided.")
			continue
		}

		var assigneeJIDs []string
		membersOK := true
		for _, phone := range phones {
			if canVerifyMembership && !participants[phone] {
				taskErr(idx, title, "Assignee %s is not a member of '%s'. Add them first.", phone, waGroup)
				membersOK = false
				break
			}
			assigneeJIDs = append(assigneeJIDs, bot.FormatJIDString(phone))
		}
		if !membersOK {
			continue
		}

		if existingTitles[title] || seen[title] {
			taskErr(idx, title, "A task with title '%s' already exists in '%s'.", title, listName)
			continue
		}
		seen[title] = true

		valid = append(valid, BulkTask{
			Title:        title,
			AssigneeJIDs: assigneeJIDs,
			Deadline:     deadline,
			ReminderSpec: reminderSpec,
		})
	}
	return valid, errs
}

// bulkRows converts validated tasks into storage rows.
func bulkRows(tasks []BulkTask) []db.BulkTaskRow {
	if len(tasks) == 0 {
		return nil
	}
	rows := make([]db.BulkTaskRow, 0, len(tasks))
	for _, t := range tasks {
		rows = append(rows, db.BulkTaskRow{
			Title:        t.Title,
			Deadline:     t.Deadline.Unix(),
			ReminderMode: t.ReminderSpec.Mode(),
			ReminderCron: t.ReminderSpec.Cron,
			ReminderAt:   t.ReminderSpec.At,
			AssigneeJIDs: t.AssigneeJIDs,
		})
	}
	return rows
}
