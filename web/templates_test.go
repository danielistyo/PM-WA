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

	"pm-wa/db"
)

// newTestTemplate parses the embedded templates exactly like the production
// server so changes to templates (including the reminder picker partial) are
// caught by the test suite.
func newTestTemplate(t *testing.T) *template.Template {
	t.Helper()
	fm := template.FuncMap{
		"fmtTime": func(unix int64) string {
			if unix == 0 {
				return ""
			}
			return time.Unix(unix, 0).In(gmt7).Format("2006-01-02 15:04")
		},
	}
	tmpl, err := template.New("").Funcs(fm).ParseFS(templatesFS, "templates/*.html")
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
		Reminder:     true,
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
		{"message.html", map[string]string{"Title": "t", "Body": "b"}},
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

func TestEmbeddedStaticListing(t *testing.T) {
	want := map[string]struct{}{}
	files, err := staticFS.ReadDir("static")
	if err != nil {
		t.Fatalf("read static dir: %v", err)
	}
	for _, f := range files {
		want[f.Name()] = struct{}{}
	}
	for _, name := range []string{"cronstrue.min.js", "reminder-picker.js", "reminder-picker.css"} {
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
	s := &Server{db: nil, client: nil, baseURL: "http://example.com", secure: false,
		tmpl: newTestTemplate(t)}

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
