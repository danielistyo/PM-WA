package scheduler

import (
	"context"
	"fmt"

	"go.mau.fi/whatsmeow/types"

	"pm-wa/db"
)

func (s *Scheduler) refreshAssigneePresence(ctx context.Context, list *db.TaskList) {
	groupJID, err := types.ParseJID(list.GroupJID)
	if err != nil {
		return
	}

	participantSet, err := s.client.GetGroupParticipants(ctx, groupJID)
	if err != nil {
		return
	}

	assignees, _ := s.db.GetAssigneesByGroup(list.GroupJID, list.ID)
	for _, a := range assignees {
		aJID, err := types.ParseJID(a.AssigneeJID)
		if err != nil {
			continue
		}
		inGroup := participantSet[aJID.User]
		if !inGroup && !a.LeftGroup {
			s.db.SetAssigneeLeft(a.ID, true)
			adminJID, err := types.ParseJID(list.AdminJID)
			if err != nil {
				continue
			}
			groupName := s.db.GetGroupName(list.GroupJID)
			msg := fmt.Sprintf("Alert: Assignee %s has left '%s'. Their tasks will show '(1 assignee left)' in reminders. Use 'delete-task' and re-add with a valid assignee to fix.",
				aJID.User, groupName)
			s.client.SendPM(ctx, adminJID, msg)
		}
	}
}
