package format

import (
	"fmt"
	"strings"
	"time"

	"pm-wa/db"
)

var gmt7 = time.FixedZone("GMT+7", 7*60*60)

func FormatSummaryMessage(list *db.TaskList, tasks []db.Task) (string, []string) {
	var b strings.Builder
	var mentions []string

	b.WriteString(fmt.Sprintf("ID: %d\n", list.ID))
	b.WriteString(fmt.Sprintf("Title: %s\n\n", list.Name))

	completedCount := 0
	for _, t := range tasks {
		if t.Status == "done" {
			completedCount++
			b.WriteString(fmt.Sprintf("~%d. [DONE] %s~", t.Position, t.Title))
		} else {
			b.WriteString(fmt.Sprintf("%d. %s", t.Position, t.Title))
			assigneeStr, assigneeMentions := FormatAssignees(t.Assignees)
			b.WriteString(" " + assigneeStr)
			mentions = append(mentions, assigneeMentions...)
		}
		b.WriteString(fmt.Sprintf(" (%s)", FormatDeadline(t.Deadline)))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	total := len(tasks)
	bar, pct := BuildProgressBar(completedCount, total)
	b.WriteString(fmt.Sprintf("Progress: %s %d%%\n", bar, pct))
	b.WriteString(fmt.Sprintf("(%d of %d tasks completed)\n", completedCount, total))
	b.WriteString("\nTip: Reply 'done: [number]' to complete or 'todo: [number]' to undo.")

	return b.String(), mentions
}

func FormatDeadline(unixTs int64) string {
	return time.Unix(unixTs, 0).In(gmt7).Format("2006-01-02 15:04")
}
