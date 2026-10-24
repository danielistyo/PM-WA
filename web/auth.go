package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"time"
)

const sessionTTL = 60 * time.Minute

type session struct {
	JID  string
	CSRF string
}

// authedHandler handles one authenticated request on a specific mount.
type authedHandler func(m *mount, w http.ResponseWriter, r *http.Request, sess session)

func (s *Server) requireSession(m *mount, next authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(m.cookieName)
		if err != nil {
			s.renderMessage(w, m, http.StatusUnauthorized, "Not signed in",
				"Request a private link by sending 'tasklist-web' to the bot on WhatsApp.")
			return
		}
		jid, csrf, ok := m.db.GetWebSession(c.Value)
		if !ok {
			s.clearCookie(m, w)
			s.renderMessage(w, m, http.StatusUnauthorized, "Session expired",
				"Your session has expired. Send 'tasklist-web' to the bot to get a new link.")
			return
		}
		next(m, w, r, session{JID: jid, CSRF: csrf})
	}
}

func (s *Server) handleAuth(m *mount) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.PathValue("token")
		jid, ok := m.db.ConsumeWebToken(token)
		if !ok {
			s.renderMessage(w, m, http.StatusUnauthorized, "Invalid or expired link",
				"This link is invalid, already used, or expired. Send 'tasklist-web' to the bot for a new one.")
			return
		}

		sessionID, err := randomID()
		if err != nil {
			s.renderMessage(w, m, http.StatusInternalServerError, "Error", "Please try again.")
			return
		}
		csrf, err := randomID()
		if err != nil {
			s.renderMessage(w, m, http.StatusInternalServerError, "Error", "Please try again.")
			return
		}

		expiresAt := time.Now().Add(sessionTTL)
		if err := m.db.CreateWebSession(sessionID, jid, csrf, expiresAt.Unix()); err != nil {
			s.renderMessage(w, m, http.StatusInternalServerError, "Error", "Please try again.")
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     m.cookieName,
			Value:    sessionID,
			Path:     m.cookiePath(),
			Expires:  expiresAt,
			HttpOnly: true,
			Secure:   s.secure,
			SameSite: http.SameSiteLaxMode,
		})
		http.Redirect(w, r, m.root(), http.StatusSeeOther)
	}
}

func (s *Server) handleLogout(m *mount, w http.ResponseWriter, r *http.Request, sess session) {
	if !s.checkCSRF(r, sess) {
		s.renderMessage(w, m, http.StatusForbidden, "Invalid request", "Please reload and try again.")
		return
	}
	if c, err := r.Cookie(m.cookieName); err == nil {
		m.db.DeleteWebSession(c.Value)
	}
	s.clearCookie(m, w)
	s.renderMessage(w, m, http.StatusOK, "Signed out", "You have been signed out.")
}

func (s *Server) clearCookie(m *mount, w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     m.cookieName,
		Value:    "",
		Path:     m.cookiePath(),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) checkCSRF(r *http.Request, sess session) bool {
	token := r.FormValue("_csrf")
	if token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(sess.CSRF)) == 1
}

func randomID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
