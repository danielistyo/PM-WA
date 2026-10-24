package web

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"pm-wa/bot"
	"pm-wa/db"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed static
var staticFS embed.FS

var gmt7 = time.FixedZone("GMT+7", 7*60*60)

const sessionCookie = "pmwa_sess"

// MountSpec describes one account's web UI: its URL prefix, database and
// client. The caller decides the mounting scheme (root for a single
// account, /a/<name> for multi-account); the server only mounts what it is
// told.
type MountSpec struct {
	Name   string
	Prefix string // "/" or "/a/<name>" (no trailing slash)
	DB     *db.Database
	Client *bot.Client
}

// mount is one account's mounted web subserver. In single-account mode the
// mount sits at the bare root exactly like the pre-multi-account UI; in
// multi-account mode every mount sits under /a/<name>/.
type mount struct {
	name       string
	prefix     string // "/" or "/a/<name>" (no trailing slash)
	cookieName string // "pmwa_sess" or "pmwa_sess_<name>"
	db         *db.Database
	client     *bot.Client
	inner      http.Handler
}

// pfx is the URL prefix for links: "" for the root mount, "/a/<name>" for a
// prefixed mount.
func (m *mount) pfx() string {
	if m.prefix == "/" {
		return ""
	}
	return m.prefix
}

// root is the mount's lists-page URL ("/" or "/a/<name>/").
func (m *mount) root() string {
	if m.prefix == "/" {
		return "/"
	}
	return m.prefix + "/"
}

// cookiePath scopes the session cookie to the mount: "/" for the root mount
// (unchanged legacy behavior), "/a/<name>/" for prefixed mounts.
func (m *mount) cookiePath() string {
	if m.prefix == "/" {
		return "/"
	}
	return m.prefix + "/"
}

type Server struct {
	baseURL string
	secure  bool
	tmpl    *template.Template
	mounts  []*mount
}

// templateFuncMap is the single source of truth for template functions, shared
// by the production server and the test template parser.
func templateFuncMap() template.FuncMap {
	return template.FuncMap{
		"fmtTime": func(unix int64) string {
			if unix == 0 {
				return ""
			}
			return time.Unix(unix, 0).In(gmt7).Format("2006-01-02 15:04")
		},
		// Same human-readable text as the edit-task page.
		"reminderText": reminderToText,
	}
}

// NewServer builds the web UI for the given account mounts. A mount with
// prefix "/" is served from the bare root (URL paths, cookie name and
// auth behavior identical to the pre-multi-account UI); prefixed mounts
// sit under /a/<name>/.
func NewServer(mounts []MountSpec, baseURL string) *Server {
	tmpl := template.Must(
		template.New("").Funcs(templateFuncMap()).ParseFS(templatesFS, "templates/*.html"),
	)
	s := &Server{
		baseURL: baseURL,
		secure:  strings.HasPrefix(strings.ToLower(baseURL), "https"),
		tmpl:    tmpl,
	}
	for _, spec := range mounts {
		m := &mount{
			name:   spec.Name,
			prefix: spec.Prefix,
			db:     spec.DB,
			client: spec.Client,
		}
		if m.prefix == "/" {
			m.cookieName = sessionCookie
		} else {
			m.cookieName = sessionCookie + "_" + m.name
		}
		m.inner = s.innerHandler(m)
		s.mounts = append(s.mounts, m)
	}
	return s
}

func subFS(fsys fs.FS, dir string) fs.FS {
	s, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return s
}

// Handler returns the root handler served on the single listener. Static
// assets stay at the shared absolute /static/ path for every account.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(subFS(staticFS, "static")))))
	mux.HandleFunc("/", s.dispatch)
	return mux
}

// dispatch routes a request to the mount whose prefix matches, redirecting
// /a/<name> to /a/<name>/ so the account root always lands on its lists page.
func (s *Server) dispatch(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	for _, m := range s.mounts {
		if m.prefix == "/" {
			continue
		}
		if path == m.prefix {
			http.Redirect(w, r, m.root(), http.StatusMovedPermanently)
			return
		}
		if path == m.prefix+"/" || strings.HasPrefix(path, m.prefix+"/") {
			http.StripPrefix(m.prefix, m.inner).ServeHTTP(w, r)
			return
		}
	}
	if len(s.mounts) == 1 && s.mounts[0].prefix == "/" {
		s.mounts[0].inner.ServeHTTP(w, r)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) innerHandler(m *mount) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /auth/{token}", s.handleAuth(m))
	mux.HandleFunc("POST /logout", s.requireSession(m, s.handleLogout))

	mux.HandleFunc("GET /{$}", s.requireSession(m, s.handleIndex))
	mux.HandleFunc("GET /lists/new", s.requireSession(m, s.handleNewList))
	mux.HandleFunc("POST /lists", s.requireSession(m, s.handleCreateList))
	mux.HandleFunc("GET /lists/{id}", s.requireSession(m, s.handleListDetail))
	mux.HandleFunc("POST /lists/{id}/publish", s.requireSession(m, s.handlePublish))
	mux.HandleFunc("POST /lists/{id}/stop", s.requireSession(m, s.handleStop))
	mux.HandleFunc("POST /lists/{id}/delete", s.requireSession(m, s.handleDeleteList))
	mux.HandleFunc("POST /lists/{id}/tasks", s.requireSession(m, s.handleCreateTask))
	mux.HandleFunc("GET /lists/{id}/tasks/{pos}/edit", s.requireSession(m, s.handleEditTask))
	mux.HandleFunc("POST /lists/{id}/tasks/{pos}", s.requireSession(m, s.handleUpdateTask))
	mux.HandleFunc("POST /lists/{id}/tasks/{pos}/delete", s.requireSession(m, s.handleDeleteTask))

	// API
	mux.HandleFunc("GET /api/groups/{jid}/members", s.requireSession(m, s.handleGroupMembers))

	return mux
}

func (s *Server) Start(addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return srv.ListenAndServe()
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

func (s *Server) renderMessage(w http.ResponseWriter, m *mount, status int, title, body string) {
	w.WriteHeader(status)
	s.render(w, "message.html", map[string]any{"Title": title, "Body": body, "Nav": s.navFor(m)})
}

// navFor builds the navbar data for one mount: URL prefix, the account
// switcher (multi-account only) and this account's connection badge.
func (s *Server) navFor(m *mount) navData {
	d := navData{
		Pfx:    m.pfx(),
		Self:   m.name,
		Status: m.client.Status(),
	}
	if len(s.mounts) > 1 {
		for _, other := range s.mounts {
			d.Accounts = append(d.Accounts, accountLink{
				Name:    other.name,
				Href:    other.root(),
				Current: other == m,
			})
		}
	}
	return d
}
