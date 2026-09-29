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

type Server struct {
	db      *db.Database
	client  *bot.Client
	baseURL string
	secure  bool
	tmpl    *template.Template
}

func NewServer(database *db.Database, client *bot.Client, baseURL string) *Server {
	funcMap := template.FuncMap{
		"fmtTime": func(unix int64) string {
			if unix == 0 {
				return ""
			}
			return time.Unix(unix, 0).In(gmt7).Format("2006-01-02 15:04")
		},
	}
	tmpl := template.Must(
		template.New("").Funcs(funcMap).ParseFS(templatesFS, "templates/*.html"),
	)
	return &Server{
		db:      database,
		client:  client,
		baseURL: baseURL,
		secure:  strings.HasPrefix(strings.ToLower(baseURL), "https"),
		tmpl:    tmpl,
	}
}

func subFS(fsys fs.FS, dir string) fs.FS {
	s, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return s
}

func (s *Server) Start(addr string) error {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /auth/{token}", s.handleAuth)
	mux.HandleFunc("POST /logout", s.requireSession(s.handleLogout))

	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(subFS(staticFS, "static")))))

	mux.HandleFunc("GET /{$}", s.requireSession(s.handleIndex))
	mux.HandleFunc("GET /lists/new", s.requireSession(s.handleNewList))
	mux.HandleFunc("POST /lists", s.requireSession(s.handleCreateList))
	mux.HandleFunc("GET /lists/{id}", s.requireSession(s.handleListDetail))
	mux.HandleFunc("POST /lists/{id}/publish", s.requireSession(s.handlePublish))
	mux.HandleFunc("POST /lists/{id}/stop", s.requireSession(s.handleStop))
	mux.HandleFunc("POST /lists/{id}/delete", s.requireSession(s.handleDeleteList))
	mux.HandleFunc("POST /lists/{id}/tasks", s.requireSession(s.handleCreateTask))
	mux.HandleFunc("GET /lists/{id}/tasks/{pos}/edit", s.requireSession(s.handleEditTask))
	mux.HandleFunc("POST /lists/{id}/tasks/{pos}", s.requireSession(s.handleUpdateTask))
	mux.HandleFunc("POST /lists/{id}/tasks/{pos}/delete", s.requireSession(s.handleDeleteTask))

	// API
	mux.HandleFunc("GET /api/groups/{jid}/members", s.requireSession(s.handleGroupMembers))

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
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

func (s *Server) renderMessage(w http.ResponseWriter, status int, title, body string) {
	w.WriteHeader(status)
	s.render(w, "message.html", map[string]string{"Title": title, "Body": body})
}
