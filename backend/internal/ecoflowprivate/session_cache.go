package ecoflowprivate

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	defaultSharedSessionTTL = 50 * time.Minute
	defaultBusyLoginBackoff = 10 * time.Minute
)

var defaultSessionCache = newSessionCache(defaultSharedSessionTTL, defaultBusyLoginBackoff, time.Now)

type sessionCacheEntry struct {
	session    Session
	hasSession bool
	expiresAt  time.Time
	retryAfter time.Time
	lastError  string
}

type SessionCache struct {
	mu          sync.Mutex
	entries     map[string]sessionCacheEntry
	sessionTTL  time.Duration
	busyBackoff time.Duration
	now         func() time.Time
}

func newSessionCache(sessionTTL time.Duration, busyBackoff time.Duration, now func() time.Time) *SessionCache {
	if sessionTTL <= 0 {
		sessionTTL = defaultSharedSessionTTL
	}
	if busyBackoff <= 0 {
		busyBackoff = defaultBusyLoginBackoff
	}
	if now == nil {
		now = time.Now
	}
	return &SessionCache{
		entries:     make(map[string]sessionCacheEntry),
		sessionTTL:  sessionTTL,
		busyBackoff: busyBackoff,
		now:         now,
	}
}

func (c *SessionCache) getOrLogin(ctx context.Context, key string, auth sessionAuthenticator) (Session, bool, error) {
	if c == nil || key == "" {
		session, err := auth.Login(ctx)
		return session, false, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	entry := c.entries[key]
	if entry.hasSession && now.Before(entry.expiresAt) {
		return entry.session, true, nil
	}
	if !entry.retryAfter.IsZero() && now.Before(entry.retryAfter) {
		return Session{}, false, fmt.Errorf("EcoFlow private login suppressed until %s after previous busy response: %s", entry.retryAfter.Format(time.RFC3339), entry.lastError)
	}

	session, err := auth.Login(ctx)
	if err != nil {
		if isBusyLoginError(err) {
			entry.hasSession = false
			entry.session = Session{}
			entry.expiresAt = time.Time{}
			entry.retryAfter = now.Add(c.busyBackoff)
			entry.lastError = sanitizedAuthError(err)
			c.entries[key] = entry
		}
		return Session{}, false, err
	}

	c.entries[key] = sessionCacheEntry{
		session:    session,
		hasSession: true,
		expiresAt:  now.Add(c.sessionTTL),
	}
	return session, false, nil
}

func (c *SessionCache) invalidate(key string) {
	if c == nil || key == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

func privateSessionCacheKey(cfg Config) string {
	cfg = cfg.normalized()
	if cfg.PrivateAPIHost == "" || cfg.Email == "" {
		return ""
	}
	return strings.Join([]string{cfg.PrivateAPIHost, strings.ToLower(cfg.Email), cfg.MQTTClientID}, "\x00")
}

func isBusyLoginError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "server is too busy") ||
		strings.Contains(message, "code=1001")
}

func sanitizedAuthError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if message == "" {
		return "unknown error"
	}
	return message
}
