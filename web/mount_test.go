package web

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pm-wa/db"
)

const testAdminJID = "628111@s.whatsapp.net"

// newMountDB opens a fresh task database for a test mount, seeds one list
// owned by testAdminJID, and issues a single-use web token for it.
func newMountDB(t *testing.T, seedList string) (*db.Database, string) {
	t.Helper()
	d, err := db.New(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("db.New(): %v", err)
	}
	t.Cleanup(func() { d.Close() })

	if _, err := d.CreateTaskList(seedList, "1203630@g.us", testAdminJID); err != nil {
		t.Fatalf("CreateTaskList(): %v", err)
	}
	token := "tok-" + seedList
	if err := d.CreateWebToken(token, testAdminJID, time.Now().Add(time.Minute).Unix()); err != nil {
		t.Fatalf("CreateWebToken(): %v", err)
	}
	return d, token
}

// newTestServer builds an httptest server from the given mounts, deriving
// the URL prefixes the same way main.go does: a single account mounts at
// the bare root, two or more mount under /a/<name>/.
func newTestServer(t *testing.T, specs []MountSpec) *httptest.Server {
	t.Helper()
	if len(specs) > 1 {
		for i := range specs {
			specs[i].Prefix = "/a/" + specs[i].Name
		}
	} else if len(specs) == 1 {
		specs[0].Prefix = "/"
	}
	s := NewServer(specs, "http://example.com")
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return srv
}

// noRedirectClient returns a client that keeps cookies but never follows
// redirects, so each test can assert the exact status it receives.
func noRedirectClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}

func get(t *testing.T, c *http.Client, srvURL, path string) (status int, body string) {
	t.Helper()
	resp, err := c.Get(srvURL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body %s: %v", path, err)
	}
	return resp.StatusCode, string(b)
}

// login authenticates against the given mount's auth endpoint and keeps the
// issued session cookie in the client's jar.
func login(t *testing.T, c *http.Client, srvURL, mountPath, token string) {
	t.Helper()
	status, body := get(t, c, srvURL, mountPath+"/auth/"+token)
	if status != http.StatusSeeOther {
		t.Fatalf("auth %s: status %d, want 303; body: %s", mountPath, status, body)
	}
}

// cookiesFor returns the cookies a client would send to srvURL+path.
// The paths are concatenated as strings on purpose: JoinPath would strip the
// trailing slash, which cookiejar's path filter cares about.
func cookiesFor(t *testing.T, c *http.Client, srvURL, path string) []*http.Cookie {
	t.Helper()
	u, err := url.Parse(srvURL + path)
	if err != nil {
		t.Fatalf("parse %s: %v", srvURL+path, err)
	}
	return c.Jar.Cookies(u)
}

// locationOf performs a GET without following redirects and returns the
// Location response header (empty when absent).
func locationOf(t *testing.T, c *http.Client, srvURL, path string) string {
	t.Helper()
	resp, err := c.Get(srvURL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.Header.Get("Location")
}

func TestSingleMountServesAtRoot(t *testing.T) {
	d, token := newMountDB(t, "rootlist")
	srv := newTestServer(t, []MountSpec{{Name: "default", DB: d}})
	c := noRedirectClient()

	login(t, c, srv.URL, "", token)

	// The account root serves the lists page from the bare root path.
	status, body := get(t, c, srv.URL, "/")
	if status != http.StatusOK {
		t.Fatalf("GET /: status %d, want 200; body: %s", status, body)
	}
	if !strings.Contains(body, "rootlist") {
		t.Errorf("GET / should list this account's task list; got:\n%s", body)
	}

	// The session cookie keeps the legacy single-account name and path.
	found := false
	for _, ck := range cookiesFor(t, c, srv.URL, "/") {
		if ck.Name == "pmwa_sess" {
			found = true
		}
	}
	if !found {
		t.Errorf("single-account mode should use the legacy pmwa_sess cookie")
	}
}

func TestAccountRootRedirectsToListsPage(t *testing.T) {
	d, _ := newMountDB(t, "saleslist")
	srv := newTestServer(t, []MountSpec{
		{Name: "sales", DB: d},
		{Name: "service", DB: d},
	})
	c := noRedirectClient()

	status, _ := get(t, c, srv.URL, "/a/sales")
	if status != http.StatusMovedPermanently {
		t.Fatalf("GET /a/sales: status %d, want 301", status)
	}
	if loc := locationOf(t, c, srv.URL, "/a/sales"); loc != "/a/sales/" {
		t.Errorf("GET /a/sales Location = %q, want %q", loc, "/a/sales/")
	}
}

func TestPerAccountAuthScoping(t *testing.T) {
	salesDB, salesTok := newMountDB(t, "saleslist")
	serviceDB, _ := newMountDB(t, "servicelist")
	srv := newTestServer(t, []MountSpec{
		{Name: "sales", DB: salesDB},
		{Name: "service", DB: serviceDB},
	})
	c := noRedirectClient()

	login(t, c, srv.URL, "/a/sales", salesTok)

	// The sales session sees sales' lists on its own prefix...
	status, body := get(t, c, srv.URL, "/a/sales/")
	if status != http.StatusOK || !strings.Contains(body, "saleslist") {
		t.Fatalf("GET /a/sales/: status %d; body:\n%s", status, body)
	}
	// ...and never the other account's data.
	if strings.Contains(body, "servicelist") {
		t.Errorf("sales session must not see service account lists; got:\n%s", body)
	}

	// The session cookie is scoped to the sales mount...
	found := false
	for _, ck := range cookiesFor(t, c, srv.URL, "/a/sales/") {
		if ck.Name == "pmwa_sess_sales" {
			found = true
		}
	}
	if !found {
		t.Errorf("multi-account mounts must use per-account cookie names")
	}

	// ...and does not authorize the other account: its DB has no session.
	status, _ = get(t, c, srv.URL, "/a/service/")
	if status != http.StatusUnauthorized {
		t.Errorf("GET /a/service/ with sales cookie: status %d, want 401", status)
	}
}

func TestSwitcherAndDegradedBadge(t *testing.T) {
	salesDB, salesTok := newMountDB(t, "saleslist")
	serviceDB, _ := newMountDB(t, "servicelist")
	// nil clients: rendering must degrade (unknown badge) instead of failing.
	srv := newTestServer(t, []MountSpec{
		{Name: "sales", DB: salesDB},
		{Name: "service", DB: serviceDB},
	})
	c := noRedirectClient()
	login(t, c, srv.URL, "/a/sales", salesTok)

	status, body := get(t, c, srv.URL, "/a/sales/")
	if status != http.StatusOK {
		t.Fatalf("GET /a/sales/: status %d, want 200", status)
	}

	if !strings.Contains(body, `href="/a/service/"`) {
		t.Errorf("switcher should link the other account's dashboard; got:\n%s", body)
	}
	if !strings.Contains(body, `class="current" href="/a/sales/"`) {
		t.Errorf("switcher should mark the current account; got:\n%s", body)
	}
	if !strings.Contains(body, "status-unknown") || !strings.Contains(body, "unknown") {
		t.Errorf("badge should render unknown gracefully when status is unavailable; got:\n%s", body)
	}
}

func TestUnknownAccountPrefixIs404(t *testing.T) {
	d, _ := newMountDB(t, "saleslist")
	srv := newTestServer(t, []MountSpec{
		{Name: "sales", DB: d},
		{Name: "service", DB: d},
	})
	c := noRedirectClient()
	status, _ := get(t, c, srv.URL, "/a/ghost/")
	if status != http.StatusNotFound {
		t.Errorf("GET /a/ghost/: status %d, want 404", status)
	}
}

func TestStaticSharedAcrossAccounts(t *testing.T) {
	d, _ := newMountDB(t, "saleslist")
	srv := newTestServer(t, []MountSpec{
		{Name: "sales", DB: d},
		{Name: "service", DB: d},
	})
	status, body := get(t, &http.Client{}, srv.URL, "/static/cronstrue.min.js")
	if status != http.StatusOK || body == "" {
		t.Errorf("GET /static/cronstrue.min.js: status %d", status)
	}
}
