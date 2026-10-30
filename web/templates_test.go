package web

import (
	"bytes"
	"html/template"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"pm-wa/bot"
	"pm-wa/db"
)

// newTestTemplate parses the embedded templates exactly like the production
// server (same templateFuncMap) so changes to templates (including the
// reminder picker partial) are caught by the test suite.
func newTestTemplate(t *testing.T) *template.Template {
	t.Helper()
	tmpl, err := template.New("").Funcs(templateFuncMap()).ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	return tmpl
}

func TestTemplatesParseAndExecute(t *testing.T) {
	tmpl := newTestTemplate(t)

	list := db.TaskList{ID: 1, Name: "Team", GroupJID: "g", AdminJID: "6281@s.whatsapp.net"}
	task := db.Task{
		ID:           1,
		TaskListID:   1,
		Position:     1,
		Title:        "Ship it",
		Status:       "todo",
		Deadline:     time.Now().Unix(),
		ReminderMode: db.ModeCron,
		ReminderCron: "0 20 * * 1",
		Assignees:    []db.TaskAssignee{{ID: 1, TaskID: 1, AssigneeJID: "6281@s.whatsapp.net"}},
	}

	cases := []struct {
		name string
		data any
	}{
		{"lists.html", indexView{CSRF: "tok", Lists: []listRow{{List: list, GroupName: "g"}}}},
		{"list_new.html", newListView{CSRF: "tok"}},
		{"list_detail.html", listDetailView{CSRF: "tok", List: list, GroupName: "g",
			Tasks: []db.Task{task}}},
		{"task_edit.html", editTaskView{CSRF: "tok", List: list, Task: task,
			AssigneeCSV:  "6281",
			ReminderText: "0 20 * * 1",
			DeadlineText: time.Unix(task.Deadline, 0).In(gmt7).Format("2006-01-02T15:04")}},
		{"message.html", map[string]any{"Title": "t", "Body": "b", "Nav": navData{}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := tmpl.ExecuteTemplate(&buf, tc.name, tc.data); err != nil {
				t.Fatalf("execute %s: %v", tc.name, err)
			}
			if buf.Len() == 0 {
				t.Fatalf("execute %s produced empty output", tc.name)
			}
		})
	}
}

func TestListDetailReminderDisplay(t *testing.T) {
	tmpl := newTestTemplate(t)

	list := db.TaskList{ID: 1, Name: "Team", GroupJID: "g", AdminJID: "6281@s.whatsapp.net"}
	deadline := time.Now().Unix()

	cases := []struct {
		name         string
		reminderMode string
		reminderAt   int64
		cron         string
		wantText     string
		wantAbsent   string
	}{
		{
			name:         "reminder off shows nothing",
			reminderMode: db.ModeNone,
			wantText:     "",
			wantAbsent:   "reminder:",
		},
		{
			name:         "reminder at deadline shows yes",
			reminderMode: db.ModeDeadline,
			wantText:     `<span data-cron-humanize>yes</span>`,
			wantAbsent:   "",
		},
		{
			name:         "recurring reminder shows cron for client-side humanization",
			reminderMode: db.ModeCron,
			cron:         "0 20 * * 1",
			wantText:     `<span data-cron-humanize>0 20 * * 1</span>`,
			wantAbsent:   "",
		},
		{
			name:         "once-at reminder shows formatted time",
			reminderMode: db.ModeAt,
			reminderAt:   time.Date(2026, 3, 15, 9, 30, 0, 0, gmt7).Unix(),
			wantText:     `<span data-cron-humanize>2026-03-15 09:30</span>`,
			wantAbsent:   "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			task := db.Task{
				ID:           1,
				TaskListID:   list.ID,
				Position:     1,
				Title:        "Ship it",
				Status:       "todo",
				Deadline:     deadline,
				ReminderMode: tc.reminderMode,
				ReminderAt:   tc.reminderAt,
				ReminderCron: tc.cron,
			}
			var buf bytes.Buffer
			if err := tmpl.ExecuteTemplate(&buf, "list_detail.html", listDetailView{
				CSRF: "tok", List: list, GroupName: "g", Tasks: []db.Task{task},
			}); err != nil {
				t.Fatalf("execute list_detail.html: %v", err)
			}
			out := buf.String()
			if tc.wantText != "" && !strings.Contains(out, tc.wantText) {
				t.Errorf("expected %q in rendered list detail, got:\n%s", tc.wantText, out)
			}
			if tc.wantAbsent != "" && strings.Contains(out, tc.wantAbsent) {
				t.Errorf("expected %q to be absent in rendered list detail", tc.wantAbsent)
			}
		})
	}
}

func TestReminderFieldDefaultAndEditRendering(t *testing.T) {
	tmpl := newTestTemplate(t)

	// The "Add task" form (empty initial value) must render the picker with
	// an empty hidden value and no cron.
	var buf bytes.Buffer
	err := tmpl.ExecuteTemplate(&buf, "reminder_field", "")
	if err != nil {
		t.Fatalf("render reminder_field with empty initial: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte(`name="reminder"`)) {
		t.Errorf("expected hidden reminder input inside reminder_field")
	}

	// The edit form passes the stored cron so the picker can pre-populate.
	buf.Reset()
	err = tmpl.ExecuteTemplate(&buf, "reminder_field", "0 20 * * 1")
	if err != nil {
		t.Fatalf("render reminder_field with cron initial: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte(`data-initial="0 20 * * 1"`)) {
		t.Errorf("expected data-initial to carry the cron value")
	}
}

func TestAccountNavBadgeAndSwitcher(t *testing.T) {
	tmpl := newTestTemplate(t)

	cases := []struct {
		name    string
		nav     navData
		want    string
		wantNot string
	}{
		{
			name: "connected badge renders green state",
			nav: navData{Pfx: "/a/sales", Self: "sales", Status: bot.StatusConnected,
				Accounts: []accountLink{{Name: "sales", Href: "/a/sales/", Current: true}, {Name: "service", Href: "/a/service/"}}},
			want: `status-connected`,
		},
		{
			name: "pairing-required badge renders its state",
			nav: navData{Status: bot.StatusPairingRequired,
				Accounts: []accountLink{{Name: "sales", Href: "/a/sales/", Current: true}}},
			want: `status-pairing-required`,
		},
		{
			name: "error badge renders its state",
			nav: navData{Status: bot.StatusError,
				Accounts: []accountLink{{Name: "sales", Href: "/a/sales/", Current: true}}},
			want: `status-error`,
		},
		{
			name: "unknown badge renders degraded state",
			nav: navData{Status: bot.StatusUnknown,
				Accounts: []accountLink{{Name: "sales", Href: "/a/sales/", Current: true}}},
			want: `status-unknown`,
		},
		{
			name:    "single-account mode renders no switcher or badge",
			nav:     navData{Pfx: "", Self: "default", Status: bot.StatusConnected},
			wantNot: "status-",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := tmpl.ExecuteTemplate(&buf, "accountnav", tc.nav); err != nil {
				t.Fatalf("execute accountnav: %v", err)
			}
			out := buf.String()
			if tc.want != "" && !strings.Contains(out, tc.want) {
				t.Errorf("expected %q in accountnav output, got:\n%s", tc.want, out)
			}
			if tc.wantNot != "" && strings.Contains(out, tc.wantNot) {
				t.Errorf("expected %q to be absent in accountnav output", tc.wantNot)
			}
		})
	}
}

func TestAssigneePickerCarriesMountPrefix(t *testing.T) {
	tmpl := newTestTemplate(t)

	// The assignee picker's fetch URL is built from the mount prefix baked
	// into the page: in multi-account mode the members endpoint lives under
	// /a/<name>/api/..., and a hardcoded bare-root fetch 404s (dispatch has
	// no bare-root mount in that mode).
	list := db.TaskList{ID: 1, Name: "Team", GroupJID: "120363408491084577@g.us", AdminJID: testAdminJID}
	task := db.Task{ID: 1, TaskListID: 1, Position: 1, Title: "Ship it", Status: "todo"}

	cases := []struct {
		name    string
		page    string
		pfx     string
		want    string
		wantNot string
	}{
		{"list detail on a prefixed mount stamps the account prefix",
			"list_detail.html", "/a/sales", `data-pfx="/a/sales"`, ""},
		{"task edit on a prefixed mount stamps the account prefix",
			"task_edit.html", "/a/sales", `data-pfx="/a/sales"`, ""},
		{"list detail at the bare root renders an empty prefix",
			"list_detail.html", "", `data-pfx=""`, `data-pfx="/`},
		{"task edit at the bare root renders an empty prefix",
			"task_edit.html", "", `data-pfx=""`, `data-pfx="/`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nav := navData{Pfx: tc.pfx}
			var buf bytes.Buffer
			var err error
			if tc.page == "list_detail.html" {
				err = tmpl.ExecuteTemplate(&buf, tc.page, listDetailView{Nav: nav, CSRF: "tok", List: list})
			} else {
				err = tmpl.ExecuteTemplate(&buf, tc.page, editTaskView{Nav: nav, CSRF: "tok", List: list,
					Task: task, AssigneeCSV: "6281"})
			}
			if err != nil {
				t.Fatalf("execute %s: %v", tc.page, err)
			}
			out := buf.String()
			if !strings.Contains(out, tc.want) {
				t.Errorf("expected %q in rendered %s, got:\n%s", tc.want, tc.page, out)
			}
			if tc.wantNot != "" && strings.Contains(out, tc.wantNot) {
				t.Errorf("expected %q to be absent in rendered %s", tc.wantNot, tc.page)
			}
		})
	}
}

func TestEmbeddedStaticListing(t *testing.T) {
	want := map[string]struct{}{}
	files, err := staticFS.ReadDir("static")
	if err != nil {
		t.Fatalf("read static dir: %v", err)
	}
	for _, f := range files {
		want[f.Name()] = struct{}{}
	}
	for _, name := range []string{"cronstrue.min.js", "reminder-picker.js", "reminder-picker.css", "assignee-picker.js", "assignee-picker.css"} {
		if _, ok := want[name]; !ok {
			t.Errorf("expected %s to be embedded in staticFS", name)
		} else {
			f, err := staticFS.Open("static/" + name)
			if err != nil {
				t.Errorf("embed open static/%s: %v", name, err)
				continue
			}
			st, _ := f.Stat()
			f.Close()
			if st.Size() == 0 {
				t.Errorf("embedded static/%s is empty", name)
			}
		}
	}
}

func TestServerServesAssets(t *testing.T) {
	// No mounts: only the shared static assets are exercised here.
	s := NewServer(nil, "http://example.com")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	go func() { _ = s.Start(addr) }()
	defer func() {
		// The test server keeps running until the process exits; force-cleanup
		// is not strictly required since Start is only invoked here.
		t.Logf("test server started on %s", addr)
	}()

	waitFor := func(path string, status int) string {
		t.Helper()
		var body string
		for i := 0; i < 50; i++ {
			resp, err := http.Get("http://" + addr + path)
			if err == nil {
				defer resp.Body.Close()
				b, err := io.ReadAll(resp.Body)
				if err == nil && resp.StatusCode == status {
					return string(b)
				}
				if resp.StatusCode != status {
					t.Errorf("GET %s: status %d, want %d", path, resp.StatusCode, status)
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
		return body
	}

	js := waitFor("/static/reminder-picker.js", http.StatusOK)
	if js == "" || !strings.Contains(js, "reminder-field") {
		t.Errorf("reminder-picker.js not served correctly: %d bytes", len(js))
	}
	css := waitFor("/static/reminder-picker.css", http.StatusOK)
	if css == "" || !strings.Contains(css, "rj-multi") {
		t.Errorf("reminder-picker.css not served correctly: %d bytes", len(css))
	}
	cronstrue := waitFor("/static/cronstrue.min.js", http.StatusOK)
	if cronstrue == "" {
		t.Errorf("cronstrue.min.js not served")
	}

	// Asset paths referenced by the shared head template must exist.
	head := waitFor("/static/nope.js", http.StatusNotFound)
	_ = head
}
