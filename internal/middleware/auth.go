package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Session holds login session data.
type Session struct {
	UserID    int64
	Username  string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// SessionStore manages login sessions in memory.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	maxAge   time.Duration
}

// NewSessionStore creates a session store with the given max age.
func NewSessionStore(maxAge time.Duration) *SessionStore {
	s := &SessionStore{
		sessions: make(map[string]*Session),
		maxAge:   maxAge,
	}
	// Cleanup expired sessions periodically
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		for range ticker.C {
			s.cleanup()
		}
	}()
	return s
}

// Create creates a new session and returns the session token.
func (s *SessionStore) Create(userID int64, username string) string {
	token := generateToken()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[token] = &Session{
		UserID:    userID,
		Username:  username,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(s.maxAge),
	}
	return token
}

// Get retrieves a session by token.
func (s *SessionStore) Get(token string) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[token]
	if !ok {
		return nil, false
	}
	if time.Now().After(sess.ExpiresAt) {
		return nil, false
	}
	return sess, true
}

// Delete removes a session.
func (s *SessionStore) Delete(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
}

func (s *SessionStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for token, sess := range s.sessions {
		if now.After(sess.ExpiresAt) {
			delete(s.sessions, token)
		}
	}
}

func generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

const CookieName = "hostman_session"

// AuthRequired returns a middleware that requires authentication.
func AuthRequired(store *SessionStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		cookie, err := c.Cookie(CookieName)
		if err != nil || cookie == "" {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		sess, ok := store.Get(cookie)
		if !ok {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		// Store session in context for handlers
		c.Set("session", sess)
		c.Set("username", sess.Username)
		c.Next()
	}
}
