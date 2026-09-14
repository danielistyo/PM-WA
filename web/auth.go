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

type authedHandler func(http.ResponseWriter, *http.Request, session)

func (s *Server) requireSession(next authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil {
			s.renderMessage(w, http.StatusUnauthorized, "Not signed in",
				"Request a private link by sending 'tasklist-web' to the bot on WhatsApp.")
			return
		}
		jid, csrf, ok := s.db.GetWebSession(c.Value)
		if !ok {
			s.clearCookie(w)
			s.renderMessage(w, http.StatusUnauthorized, "Session expired",
				"Your session has expired. Send 'tasklist-web' to the bot to get a new link.")
			return
		}
		next(w, r, session{JID: jid, CSRF: csrf})
	}
}

func (s *Server) handleAuth(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	jid, ok := s.db.ConsumeWebToken(token)
	if !ok {
		s.renderMessage(w, http.StatusUnauthorized, "Invalid or expired link",
			"This link is invalid, already used, or expired. Send 'tasklist-web' to the bot for a new one.")
		return
	}

	sessionID, err := randomID()
	if err != nil {
		s.renderMessage(w, http.StatusInternalServerError, "Error", "Please try again.")
		return
	}
	csrf, err := randomID()
	if err != nil {
		s.renderMessage(w, http.StatusInternalServerError, "Error", "Please try again.")
		return
	}

	expiresAt := time.Now().Add(sessionTTL)
	if err := s.db.CreateWebSession(sessionID, jid, csrf, expiresAt.Unix()); err != nil {
		s.renderMessage(w, http.StatusInternalServerError, "Error", "Please try again.")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    sessionID,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request, sess session) {
	if !s.checkCSRF(r, sess) {
		s.renderMessage(w, http.StatusForbidden, "Invalid request", "Please reload and try again.")
		return
	}
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.db.DeleteWebSession(c.Value)
	}
	s.clearCookie(w)
	s.renderMessage(w, http.StatusOK, "Signed out", "You have been signed out.")
}

func (s *Server) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
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
