package format

import (
	"fmt"
	"strings"
	"time"

	"pm-wa/db"
)

func FormatDailyPulse(list *db.TaskList, pulseTasks []db.Task, now time.Time) (string, []string) {
	var b strings.Builder
	var mentions []string

	b.WriteString("⏰ Daily Reminder\n")
	b.WriteString(fmt.Sprintf("ID: %d\n", list.ID))
	b.WriteString(fmt.Sprintf("Title: %s\n\n", list.Name))

	for _, t := range pulseTasks {
		b.WriteString(fmt.Sprintf("%d. %s", t.Position, t.Title))
		assigneeStr, assigneeMentions := FormatAssignees(t.Assignees)
		b.WriteString(" " + assigneeStr)
		mentions = append(mentions, assigneeMentions...)
		b.WriteString(fmt.Sprintf(" — Due: %s", FormatDeadline(t.Deadline)))
		deadline := time.Unix(t.Deadline, 0)
		if deadline.Before(now) {
			b.WriteString(" ⚠️ OVERDUE")
		}
		b.WriteString("\n")
	}

	b.WriteString("\nTip: Reply 'done: [number]' to complete or 'todo: [number]' to undo.")

	return b.String(), mentions
}
