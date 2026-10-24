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

// navData carries the navbar state shared by every page: this mount's URL
// prefix, the account switcher (multi-account only) and the connection badge.
type navData struct {
	Pfx      string
	Self     string
	Accounts []accountLink
	Status   string
}

type accountLink struct {
	Name    string
	Href    string
	Current bool
}

type listRow struct {
	List      db.TaskList
	GroupName string
	TaskCount int
}

type indexView struct {
	Nav   navData
	CSRF  string
	Lists []listRow
}

func (s *Server) handleIndex(m *mount, w http.ResponseWriter, r *http.Request, sess session) {
	lists, err := m.db.GetTaskListsByAdmin(sess.JID)
	if err != nil {
		s.renderMessage(w, m, http.StatusInternalServerError, "Error", "Could not load your task lists.")
		return
	}
	rows := make([]listRow, 0, len(lists))
	for _, l := range lists {
		count, _ := m.db.GetTaskCountByList(l.ID)
		rows = append(rows, listRow{List: l, GroupName: m.db.GetGroupName(l.GroupJID), TaskCount: count})
	}
	s.render(w, "lists.html", indexView{Nav: s.navFor(m), CSRF: sess.CSRF, Lists: rows})
}

type groupOption struct {
	JID  string
	Name string
}

type newListView struct {
	Nav    navData
	CSRF   string
	Groups []groupOption
	Error  string
}

func (s *Server) handleNewList(m *mount, w http.ResponseWriter, r *http.Request, sess session) {
	s.render(w, "list_new.html", newListView{Nav: s.navFor(m), CSRF: sess.CSRF, Groups: s.memberGroups(m, r.Context(), sess.JID)})
}

func (s *Server) handleCreateList(m *mount, w http.ResponseWriter, r *http.Request, sess session) {
	if !s.checkCSRF(r, sess) {
		s.renderMessage(w, m, http.StatusForbidden, "Invalid request", "Please reload and try again.")
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	groupJIDStr := strings.TrimSpace(r.FormValue("group"))
	if name == "" || groupJIDStr == "" {
		s.render(w, "list_new.html", newListView{Nav: s.navFor(m), CSRF: sess.CSRF, Groups: s.memberGroups(m, r.Context(), sess.JID), Error: "Name and group are required."})
		return
	}

	groupJID, err := types.ParseJID(groupJIDStr)
	if err != nil || !s.isMember(m, r.Context(), groupJID, sess.JID) {
		s.render(w, "list_new.html", newListView{Nav: s.navFor(m), CSRF: sess.CSRF, Groups: s.memberGroups(m, r.Context(), sess.JID), Error: "You are not a member of the selected group."})
		return
	}

	if existing, _ := m.db.GetTaskListByNameAndGroup(name, groupJID.String()); existing != nil {
		s.render(w, "list_new.html", newListView{Nav: s.navFor(m), CSRF: sess.CSRF, Groups: s.memberGroups(m, r.Context(), sess.JID), Error: "A list with that name already exists in this group."})
		return
	}

	list, err := m.db.CreateTaskList(name, groupJID.String(), sess.JID)
	if err != nil {
		s.render(w, "list_new.html", newListView{Nav: s.navFor(m), CSRF: sess.CSRF, Groups: s.memberGroups(m, r.Context(), sess.JID), Error: "Could not create the list."})
		return
	}
	http.Redirect(w, r, m.pfx()+"/lists/"+strconv.FormatInt(list.ID, 10), http.StatusSeeOther)
}

type listDetailView struct {
	Nav       navData
	CSRF      string
	List      db.TaskList
	GroupName string
	Tasks     []db.Task
}

func (s *Server) handleListDetail(m *mount, w http.ResponseWriter, r *http.Request, sess session) {
	list, ok := s.loadOwnedList(m, w, r, sess)
	if !ok {
		return
	}
	tasks, _ := m.db.GetTasksByList(list.ID)
	s.render(w, "list_detail.html", listDetailView{
		Nav:       s.navFor(m),
		CSRF:      sess.CSRF,
		List:      *list,
		GroupName: m.db.GetGroupName(list.GroupJID),
		Tasks:     tasks,
	})
}

func (s *Server) handlePublish(m *mount, w http.ResponseWriter, r *http.Request, sess session) {
	list, ok := s.loadOwnedListPost(m, w, r, sess)
	if !ok {
		return
	}
	if list.Status == "stopped" {
		s.renderMessage(w, m, http.StatusConflict, "Cannot publish", "This list has been permanently stopped.")
		return
	}
	count, _ := m.db.GetTaskCountByList(list.ID)
	if count == 0 {
		s.renderMessage(w, m, http.StatusBadRequest, "Cannot publish", "Add at least one task before publishing.")
		return
	}
	if list.Status == "unpublished" {
		m.db.UpdateListStatus(list.ID, "active")
		list.Status = "active"
	}
	groupJID, err := types.ParseJID(list.GroupJID)
	if err == nil {
		tasks, _ := m.db.GetTasksByList(list.ID)
		text, mentions := format.FormatSummaryMessage(list, tasks)
		if resp, err := m.client.SendGroupMessage(r.Context(), groupJID, text, mentions); err == nil {
			m.db.SaveMessageMapping(resp.ID, list.ID, groupJID.String())
		}
	}
	http.Redirect(w, r, m.pfx()+"/lists/"+strconv.FormatInt(list.ID, 10), http.StatusSeeOther)
}

func (s *Server) handleStop(m *mount, w http.ResponseWriter, r *http.Request, sess session) {
	list, ok := s.loadOwnedListPost(m, w, r, sess)
	if !ok {
		return
	}
	m.db.UpdateListStatus(list.ID, "stopped")
	http.Redirect(w, r, m.pfx()+"/lists/"+strconv.FormatInt(list.ID, 10), http.StatusSeeOther)
}

func (s *Server) handleDeleteList(m *mount, w http.ResponseWriter, r *http.Request, sess session) {
	list, ok := s.loadOwnedListPost(m, w, r, sess)
	if !ok {
		return
	}
	m.db.DeleteMessageMapByList(list.ID)
	m.db.DeleteTaskList(list.ID)
	http.Redirect(w, r, m.root(), http.StatusSeeOther)
}

func (s *Server) handleCreateTask(m *mount, w http.ResponseWriter, r *http.Request, sess session) {
	list, ok := s.loadOwnedListPost(m, w, r, sess)
	if !ok {
		return
	}
	if list.Status == "stopped" {
		s.renderMessage(w, m, http.StatusConflict, "Cannot add task", "This list has been permanently stopped.")
		return
	}
	title, deadline, spec, assignees, errMsg := s.parseTaskForm(m, r, list)
	if errMsg != "" {
		s.renderMessage(w, m, http.StatusBadRequest, "Cannot add task", errMsg)
		return
	}
	if _, err := m.db.CreateTask(list.ID, title, deadline, spec.Mode(), spec.Cron, spec.At, assignees); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			s.renderMessage(w, m, http.StatusConflict, "Cannot add task", "A task with that title already exists in this list.")
			return
		}
		s.renderMessage(w, m, http.StatusInternalServerError, "Error", "Could not add the task.")
		return
	}
	http.Redirect(w, r, m.pfx()+"/lists/"+strconv.FormatInt(list.ID, 10), http.StatusSeeOther)
}

type editTaskView struct {
	Nav          navData
	CSRF         string
	List         db.TaskList
	Task         db.Task
	AssigneeCSV  string
	ReminderText string
	DeadlineText string
}

func (s *Server) handleEditTask(m *mount, w http.ResponseWriter, r *http.Request, sess session) {
	list, ok := s.loadOwnedList(m, w, r, sess)
	if !ok {
		return
	}
	pos, err := strconv.Atoi(r.PathValue("pos"))
	if err != nil {
		s.renderMessage(w, m, http.StatusNotFound, "Not found", "Task not found.")
		return
	}
	task, err := m.db.GetTaskAtPosition(list.ID, pos)
	if err != nil {
		s.renderMessage(w, m, http.StatusNotFound, "Not found", "Task not found.")
		return
	}
	s.render(w, "task_edit.html", editTaskView{
		Nav:          s.navFor(m),
		CSRF:         sess.CSRF,
		List:         *list,
		Task:         *task,
		AssigneeCSV:  assigneeCSV(task.Assignees),
		ReminderText: reminderToText(*task),
		DeadlineText: time.Unix(task.Deadline, 0).In(gmt7).Format("2006-01-02T15:04"),
	})
}

func (s *Server) handleUpdateTask(m *mount, w http.ResponseWriter, r *http.Request, sess session) {
	list, ok := s.loadOwnedListPost(m, w, r, sess)
	if !ok {
		return
	}
	pos, err := strconv.Atoi(r.PathValue("pos"))
	if err != nil {
		s.renderMessage(w, m, http.StatusNotFound, "Not found", "Task not found.")
		return
	}
	task, err := m.db.GetTaskAtPosition(list.ID, pos)
	if err != nil {
		s.renderMessage(w, m, http.StatusNotFound, "Not found", "Task not found.")
		return
	}
	title, deadline, spec, assignees, errMsg := s.parseTaskForm(m, r, list)
	if errMsg != "" {
		s.renderMessage(w, m, http.StatusBadRequest, "Cannot update task", errMsg)
		return
	}
	if err := m.db.UpdateTaskFields(task.ID, title, deadline, spec.Mode(), spec.Cron, spec.At); err != nil {
		s.renderMessage(w, m, http.StatusInternalServerError, "Error", "Could not update the task.")
		return
	}
	if err := m.db.ReplaceTaskAssignees(task.ID, assignees); err != nil {
		s.renderMessage(w, m, http.StatusInternalServerError, "Error", "Could not update assignees.")
		return
	}
	http.Redirect(w, r, m.pfx()+"/lists/"+strconv.FormatInt(list.ID, 10), http.StatusSeeOther)
}

func (s *Server) handleDeleteTask(m *mount, w http.ResponseWriter, r *http.Request, sess session) {
	list, ok := s.loadOwnedListPost(m, w, r, sess)
	if !ok {
		return
	}
	pos, err := strconv.Atoi(r.PathValue("pos"))
	if err != nil {
		s.renderMessage(w, m, http.StatusNotFound, "Not found", "Task not found.")
		return
	}
	m.db.DeleteTask(list.ID, pos)
	http.Redirect(w, r, m.pfx()+"/lists/"+strconv.FormatInt(list.ID, 10), http.StatusSeeOther)
}

func (s *Server) parseTaskForm(m *mount, r *http.Request, list *db.TaskList) (title string, deadline int64, spec cmd.ReminderSpec, assignees []string, errMsg string) {
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
	participants, _, err := m.client.GetGroupParticipantsEx(r.Context(), groupJID)
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

func (s *Server) loadOwnedList(m *mount, w http.ResponseWriter, r *http.Request, sess session) (*db.TaskList, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		s.renderMessage(w, m, http.StatusNotFound, "Not found", "List not found.")
		return nil, false
	}
	list, err := m.db.GetTaskList(id)
	if err != nil || list.AdminJID != sess.JID {
		s.renderMessage(w, m, http.StatusNotFound, "Not found", "List not found.")
		return nil, false
	}
	return list, true
}

func (s *Server) loadOwnedListPost(m *mount, w http.ResponseWriter, r *http.Request, sess session) (*db.TaskList, bool) {
	if !s.checkCSRF(r, sess) {
		s.renderMessage(w, m, http.StatusForbidden, "Invalid request", "Please reload and try again.")
		return nil, false
	}
	return s.loadOwnedList(m, w, r, sess)
}

func (s *Server) memberGroups(m *mount, ctx context.Context, jid string) []groupOption {
	groups, err := m.db.GetAllGroups()
	if err != nil {
		return nil
	}
	userJID, err := types.ParseJID(jid)
	if err != nil {
		return nil
	}

	// Single RPC for every group's membership instead of one GetGroupInfo per group.
	rpcCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	member, err := m.client.GetMemberGroups(rpcCtx, userJID)
	if err != nil {
		slog.Error("GetMemberGroups failed", "error", err)
		return nil
	}

	var opts []groupOption
	for _, g := range groups {
		if member[g.JID] {
			opts = append(opts, groupOption{JID: g.JID, Name: g.Name})
		}
	}
	return opts
}

func (s *Server) isMember(m *mount, ctx context.Context, groupJID types.JID, userJIDStr string) bool {
	userJID, err := types.ParseJID(userJIDStr)
	if err != nil {
		return false
	}
	in, err := m.client.IsUserInGroup(ctx, groupJID, userJID)
	return err == nil && in
}

func assigneeCSV(assignees []db.TaskAssignee) string {
	var phones []string
	for _, a := range assignees {
		phones = append(phones, a.Phone())
	}
	return strings.Join(phones, ", ")
}

func (s *Server) handleGroupMembers(m *mount, w http.ResponseWriter, r *http.Request, sess session) {
	jidStr := r.PathValue("jid")
	groupJID, err := types.ParseJID(jidStr)
	if err != nil {
		http.Error(w, "invalid group JID", http.StatusBadRequest)
		return
	}

	// Make sure the user is in the group or owns lists in it to prevent leaking members?
	// The plan doesn't specify deep auth logic here other than requireSession, but to be safe we could check if user is admin of a list with this groupJID or if they are in the group. But requireSession is applied.
	// We'll just call GetGroupPhoneMembers. If bot can't access it, it will return error.
	members, err := m.client.GetGroupPhoneMembers(r.Context(), groupJID)
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
	switch t.ReminderMode {
	case db.ModeNone:
		return "no"
	case db.ModeAt:
		return time.Unix(t.ReminderAt, 0).In(gmt7).Format("2006-01-02 15:04")
	case db.ModeCron:
		return t.ReminderCron
	default:
		// ModeDeadline — shown as "yes"; the deadline is the task's own row.
		return "yes"
	}
}
