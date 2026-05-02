package format

import (
	"fmt"
	"strings"

	"pm-wa/db"
)

func BuildProgressBar(completed, total int) (string, int) {
	if total == 0 {
		return "[----------]", 0
	}
	pct := (completed * 100) / total
	filled := (completed * 10) / total
	bar := "[" + strings.Repeat("=", filled) + strings.Repeat("-", 10-filled) + "]"
	return bar, pct
}

func FormatAssignees(assignees []db.TaskAssignee) (string, []string) {
	var parts []string
	var mentions []string
	leftCount := 0

	for _, a := range assignees {
		if a.LeftGroup {
			leftCount++
		} else {
			parts = append(parts, fmt.Sprintf("@%s", a.Phone()))
			mentions = append(mentions, a.AssigneeJID)
		}
	}

	result := strings.Join(parts, " ")
	if leftCount > 0 {
		suffix := fmt.Sprintf("(%d assignee left)", leftCount)
		if leftCount > 1 {
			suffix = fmt.Sprintf("(%d assignees left)", leftCount)
		}
		if len(parts) > 0 {
			result += " " + suffix
		} else {
			result = suffix
		}
	}
	return result, mentions
}
