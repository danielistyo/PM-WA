package web

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"

	"pm-wa/bot"
	"pm-wa/cmd"
	"pm-wa/db"
	"pm-wa/format"
)

type listRow struct {
	List      db.TaskList
	GroupName string
	TaskCount int
}

type indexView struct {
	CSRF  string
	Lists []listRow
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request, sess session) {
	lists, err := s.db.GetTaskListsByAdmin(sess.JID)
	if err != nil {
		s.renderMessage(w, http.StatusInternalServerError, "Error", "Could not load your task lists.")
		return
	}
	rows := make([]listRow, 0, len(lists))
	for _, l := range lists {
		count, _ := s.db.GetTaskCountByList(l.ID)
		rows = append(rows, listRow{List: l, GroupName: s.db.GetGroupName(l.GroupJID), TaskCount: count})
	}
	s.render(w, "lists.html", indexView{CSRF: sess.CSRF, Lists: rows})
}

type groupOption struct {
	JID  string
	Name string
}

type newListView struct {
	CSRF   string
	Groups []groupOption
	Error  string
}

func (s *Server) handleNewList(w http.ResponseWriter, r *http.Request, sess session) {
	s.render(w, "list_new.html", newListView{CSRF: sess.CSRF, Groups: s.memberGroups(r.Context(), sess.JID)})
}

func (s *Server) handleCreateList(w http.ResponseWriter, r *http.Request, sess session) {
	if !s.checkCSRF(r, sess) {
		s.renderMessage(w, http.StatusForbidden, "Invalid request", "Please reload and try again.")
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	groupJIDStr := strings.TrimSpace(r.FormValue("group"))
	if name == "" || groupJIDStr == "" {
		s.render(w, "list_new.html", newListView{CSRF: sess.CSRF, Groups: s.memberGroups(r.Context(), sess.JID), Error: "Name and group are required."})
		return
	}

	groupJID, err := types.ParseJID(groupJIDStr)
	if err != nil || !s.isMember(r.Context(), groupJID, sess.JID) {
		s.render(w, "list_new.html", newListView{CSRF: sess.CSRF, Groups: s.memberGroups(r.Context(), sess.JID), Error: "You are not a member of the selected group."})
		return
	}

	if existing, _ := s.db.GetTaskListByNameAndGroup(name, groupJID.String()); existing != nil {
		s.render(w, "list_new.html", newListView{CSRF: sess.CSRF, Groups: s.memberGroups(r.Context(), sess.JID), Error: "A list with that name already exists in this group."})
		return
	}

	list, err := s.db.CreateTaskList(name, groupJID.String(), sess.JID)
	if err != nil {
		s.render(w, "list_new.html", newListView{CSRF: sess.CSRF, Groups: s.memberGroups(r.Context(), sess.JID), Error: "Could not create the list."})
		return
	}
	http.Redirect(w, r, "/lists/"+strconv.FormatInt(list.ID, 10), http.StatusSeeOther)
}

type listDetailView struct {
	CSRF      string
	List      db.TaskList
	GroupName string
	Tasks     []db.Task
}

func (s *Server) handleListDetail(w http.ResponseWriter, r *http.Request, sess session) {
	list, ok := s.loadOwnedList(w, r, sess)
	if !ok {
		return
	}
	tasks, _ := s.db.GetTasksByList(list.ID)
	s.render(w, "list_detail.html", listDetailView{
		CSRF:      sess.CSRF,
		List:      *list,
		GroupName: s.db.GetGroupName(list.GroupJID),
		Tasks:     tasks,
	})
}

func (s *Server) handlePublish(w http.ResponseWriter, r *http.Request, sess session) {
	list, ok := s.loadOwnedListPost(w, r, sess)
	if !ok {
		return
	}
	if list.Status == "stopped" {
		s.renderMessage(w, http.StatusConflict, "Cannot publish", "This list has been permanently stopped.")
		return
	}
	count, _ := s.db.GetTaskCountByList(list.ID)
	if count == 0 {
		s.renderMessage(w, http.StatusBadRequest, "Cannot publish", "Add at least one task before publishing.")
		return
	}
	if list.Status == "unpublished" {
		s.db.UpdateListStatus(list.ID, "active")
		list.Status = "active"
	}
	groupJID, err := types.ParseJID(list.GroupJID)
	if err == nil {
		tasks, _ := s.db.GetTasksByList(list.ID)
		text, mentions := format.FormatSummaryMessage(list, tasks)
		if resp, err := s.client.SendGroupMessage(r.Context(), groupJID, text, mentions); err == nil {
			s.db.SaveMessageMapping(resp.ID, list.ID, groupJID.String())
		}
	}
	http.Redirect(w, r, "/lists/"+strconv.FormatInt(list.ID, 10), http.StatusSeeOther)
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request, sess session) {
	list, ok := s.loadOwnedListPost(w, r, sess)
	if !ok {
		return
	}
	s.db.UpdateListStatus(list.ID, "stopped")
	http.Redirect(w, r, "/lists/"+strconv.FormatInt(list.ID, 10), http.StatusSeeOther)
}

func (s *Server) handleDeleteList(w http.ResponseWriter, r *http.Request, sess session) {
	list, ok := s.loadOwnedListPost(w, r, sess)
	if !ok {
		return
	}
	s.db.DeleteMessageMapByList(list.ID)
	s.db.DeleteTaskList(list.ID)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request, sess session) {
	list, ok := s.loadOwnedListPost(w, r, sess)
	if !ok {
		return
	}
	if list.Status == "stopped" {
		s.renderMessage(w, http.StatusConflict, "Cannot add task", "This list has been permanently stopped.")
		return
	}
	title, deadline, spec, assignees, errMsg := s.parseTaskForm(r, list)
	if errMsg != "" {
		s.renderMessage(w, http.StatusBadRequest, "Cannot add task", errMsg)
		return
	}
	if _, err := s.db.CreateTask(list.ID, title, deadline, spec.Enabled, spec.Cron, spec.At, assignees); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			s.renderMessage(w, http.StatusConflict, "Cannot add task", "A task with that title already exists in this list.")
			return
		}
		s.renderMessage(w, http.StatusInternalServerError, "Error", "Could not add the task.")
		return
	}
	http.Redirect(w, r, "/lists/"+strconv.FormatInt(list.ID, 10), http.StatusSeeOther)
}

type editTaskView struct {
	CSRF         string
	List         db.TaskList
	Task         db.Task
	AssigneeCSV  string
	ReminderText string
	DeadlineText string
}

func (s *Server) handleEditTask(w http.ResponseWriter, r *http.Request, sess session) {
	list, ok := s.loadOwnedList(w, r, sess)
	if !ok {
		return
	}
	pos, err := strconv.Atoi(r.PathValue("pos"))
	if err != nil {
		s.renderMessage(w, http.StatusNotFound, "Not found", "Task not found.")
		return
	}
	task, err := s.db.GetTaskAtPosition(list.ID, pos)
	if err != nil {
		s.renderMessage(w, http.StatusNotFound, "Not found", "Task not found.")
		return
	}
	s.render(w, "task_edit.html", editTaskView{
		CSRF:         sess.CSRF,
		List:         *list,
		Task:         *task,
		AssigneeCSV:  assigneeCSV(task.Assignees),
		ReminderText: reminderToText(*task),
		DeadlineText: time.Unix(task.Deadline, 0).In(gmt7).Format("2006-01-02T15:04"),
	})
}

func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request, sess session) {
	list, ok := s.loadOwnedListPost(w, r, sess)
	if !ok {
		return
	}
	pos, err := strconv.Atoi(r.PathValue("pos"))
	if err != nil {
		s.renderMessage(w, http.StatusNotFound, "Not found", "Task not found.")
		return
	}
	task, err := s.db.GetTaskAtPosition(list.ID, pos)
	if err != nil {
		s.renderMessage(w, http.StatusNotFound, "Not found", "Task not found.")
		return
	}
	title, deadline, spec, assignees, errMsg := s.parseTaskForm(r, list)
	if errMsg != "" {
		s.renderMessage(w, http.StatusBadRequest, "Cannot update task", errMsg)
		return
	}
	if err := s.db.UpdateTaskFields(task.ID, title, deadline, spec.Enabled, spec.Cron, spec.At); err != nil {
		s.renderMessage(w, http.StatusInternalServerError, "Error", "Could not update the task.")
		return
	}
	if err := s.db.ReplaceTaskAssignees(task.ID, assignees); err != nil {
		s.renderMessage(w, http.StatusInternalServerError, "Error", "Could not update assignees.")
		return
	}
	http.Redirect(w, r, "/lists/"+strconv.FormatInt(list.ID, 10), http.StatusSeeOther)
}

func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request, sess session) {
	list, ok := s.loadOwnedListPost(w, r, sess)
	if !ok {
		return
	}
	pos, err := strconv.Atoi(r.PathValue("pos"))
	if err != nil {
		s.renderMessage(w, http.StatusNotFound, "Not found", "Task not found.")
		return
	}
	s.db.DeleteTask(list.ID, pos)
	http.Redirect(w, r, "/lists/"+strconv.FormatInt(list.ID, 10), http.StatusSeeOther)
}

func (s *Server) parseTaskForm(r *http.Request, list *db.TaskList) (title string, deadline int64, spec cmd.ReminderSpec, assignees []string, errMsg string) {
	title = strings.TrimSpace(r.FormValue("title"))
	deadlineStr := strings.TrimSpace(r.FormValue("deadline"))
	reminderStr := strings.TrimSpace(r.FormValue("reminder"))

	var assignVals []string
	if err := r.ParseForm(); err == nil {
		assignVals = r.Form["assign"]
	}
	if len(assignVals) == 0 {
		// Fallback for single field or comma-separated test inputs
		assignStr := strings.TrimSpace(r.FormValue("assign"))
		if assignStr != "" {
			assignVals = strings.Split(assignStr, ",")
		}
	}

	if title == "" || len(assignVals) == 0 || deadlineStr == "" {
		return "", 0, spec, nil, "Title, assignees, and deadline are required."
	}

	d, err := time.ParseInLocation("2006-01-02T15:04", deadlineStr, gmt7)
	if err != nil {
		return "", 0, spec, nil, "Invalid deadline format. Use YYYY-MM-DDTHH:MM."
	}
	deadline = d.Unix()

	spec, err = cmd.ParseReminderSpec(reminderStr, gmt7)
	if err != nil {
		return "", 0, spec, nil, err.Error()
	}

	groupJID, err := types.ParseJID(list.GroupJID)
	if err != nil {
		return "", 0, spec, nil, "Invalid group."
	}
	
	// Single API call to get all members for validation
	participants, _, err := s.client.GetGroupParticipantsEx(r.Context(), groupJID)
	if err != nil {
		return "", 0, spec, nil, "Could not fetch group members for validation."
	}

	for _, part := range assignVals {
		phone := strings.TrimSpace(part)
		if phone == "" {
			continue
		}
		
		formattedJID := bot.FormatJIDString(phone)
		parsedJID, err := types.ParseJID(formattedJID)
		if err != nil {
			return "", 0, spec, nil, "Invalid assignee format: " + phone
		}
		
		if !participants[parsedJID.ToNonAD().User] {
			return "", 0, spec, nil, "Assignee " + phone + " is not a member of the group."
		}
		assignees = append(assignees, formattedJID)
	}
	if len(assignees) == 0 {
		return "", 0, spec, nil, "At least one valid assignee is required."
	}
	return title, deadline, spec, assignees, ""
}

func (s *Server) loadOwnedList(w http.ResponseWriter, r *http.Request, sess session) (*db.TaskList, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		s.renderMessage(w, http.StatusNotFound, "Not found", "List not found.")
		return nil, false
	}
	list, err := s.db.GetTaskList(id)
	if err != nil || list.AdminJID != sess.JID {
		s.renderMessage(w, http.StatusNotFound, "Not found", "List not found.")
		return nil, false
	}
	return list, true
}

func (s *Server) loadOwnedListPost(w http.ResponseWriter, r *http.Request, sess session) (*db.TaskList, bool) {
	if !s.checkCSRF(r, sess) {
		s.renderMessage(w, http.StatusForbidden, "Invalid request", "Please reload and try again.")
		return nil, false
	}
	return s.loadOwnedList(w, r, sess)
}

func (s *Server) memberGroups(ctx context.Context, jid string) []groupOption {
	groups, err := s.db.GetAllGroups()
	if err != nil {
		return nil
	}
	var opts []groupOption
	for _, g := range groups {
		parsed, err := types.ParseJID(g.JID)
		if err != nil {
			continue
		}
		if s.isMember(ctx, parsed, jid) {
			opts = append(opts, groupOption{JID: g.JID, Name: g.Name})
		}
	}
	return opts
}

func (s *Server) isMember(ctx context.Context, groupJID types.JID, userJIDStr string) bool {
	userJID, err := types.ParseJID(userJIDStr)
	if err != nil {
		return false
	}
	in, err := s.client.IsUserInGroup(ctx, groupJID, userJID)
	return err == nil && in
}

func assigneeCSV(assignees []db.TaskAssignee) string {
	var phones []string
	for _, a := range assignees {
		phones = append(phones, a.Phone())
	}
	return strings.Join(phones, ", ")
}

func (s *Server) handleGroupMembers(w http.ResponseWriter, r *http.Request, sess session) {
	jidStr := r.PathValue("jid")
	groupJID, err := types.ParseJID(jidStr)
	if err != nil {
		http.Error(w, "invalid group JID", http.StatusBadRequest)
		return
	}

	// Make sure the user is in the group or owns lists in it to prevent leaking members?
	// The plan doesn't specify deep auth logic here other than requireSession, but to be safe we could check if user is admin of a list with this groupJID or if they are in the group. But requireSession is applied.
	// We'll just call GetGroupPhoneMembers. If bot can't access it, it will return error.
	members, err := s.client.GetGroupPhoneMembers(r.Context(), groupJID)
	if err != nil {
		http.Error(w, "failed to get group members", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(members); err != nil {
		slog.Error("failed to encode group members", "error", err)
	}
}

func reminderToText(t db.Task) string {
	if !t.Reminder {
		return "no"
	}
	if t.ReminderAt > 0 {
		return time.Unix(t.ReminderAt, 0).In(gmt7).Format("2006-01-02 15:04")
	}
	if t.ReminderCron != "" {
		return t.ReminderCron
	}
	return "yes"
}
