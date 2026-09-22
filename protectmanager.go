package fastgocaptcha

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type PathedSession struct {
	mu        sync.Mutex
	id        string
	path      string
	captchaID string

	captchaAllowedTimes int
	captchaExpiredAt    time.Time
}

type FastGoCaptchaSession struct {
	id        string
	pathed    *sync.Map
	expiresAt time.Time
}

func (f *FastGoCaptcha) GetCaptchaRequiredPath(r *http.Request) (string, error) {
	protected, _ := f.CheckProtectMatcher(r.URL.Path)
	if protected {
		return r.URL.Path, nil
	}

	f.logInfof("GetCaptchaRequiredPath: %s", r.URL.Path)
	requireQueryPath := false
	switch strings.TrimPrefix(r.URL.Path, f.requestURIPrefix) {
	case "/fastgocaptcha/session/captcha", "/fastgocaptcha/session/captcha/":
		requireQueryPath = true
	case "/fastgocaptcha/captcha", "/fastgocaptcha/captcha/":
		requireQueryPath = true
	case "/fastgocaptcha/verify", "/fastgocaptcha/verify/":
		requireQueryPath = true
	}
	if requireQueryPath {
		queryPath := r.URL.Query().Get("fastgocaptcha_path")
		f.logInfof("GetCaptchaRequiredPath: requireQueryPath, queryPath: %s", queryPath)
		if queryPath == "" {
			return "", errors.New("fastgocaptcha_path is not found")
		}
		return queryPath, nil
	}
	return "", errors.New("captcha is not required")
}

func (f *FastGoCaptcha) GetOrCreateSession(r *http.Request) *FastGoCaptchaSession {
	f.sessionMutex.Lock()
	defer f.sessionMutex.Unlock()
	var id string
	cookie, err := r.Cookie("fastgocaptcha_session")
	if err != nil {
		id = uuid.New().String()
	} else {
		if stored, ok := f.sessionManager.Load(cookie.Value); ok && time.Now().Before(stored.(*FastGoCaptchaSession).expiresAt) {
			return stored.(*FastGoCaptchaSession)
		}
		id = uuid.New().String()
	}
	now := time.Now()
	count := 0
	var oldestKey any
	var oldest time.Time
	f.sessionManager.Range(func(key, value any) bool {
		if !now.Before(value.(*FastGoCaptchaSession).expiresAt) {
			f.sessionManager.Delete(key)
		} else {
			count++
			expires := value.(*FastGoCaptchaSession).expiresAt
			if oldestKey == nil || expires.Before(oldest) {
				oldestKey, oldest = key, expires
			}
		}
		return true
	})
	if count >= 4096 {
		f.sessionManager.Delete(oldestKey)
	}
	sessionraw, ok := f.sessionManager.Load(id)
	if !ok {
		session := &FastGoCaptchaSession{
			id:        id,
			pathed:    new(sync.Map),
			expiresAt: time.Now().Add(f.sessionTimeout),
		}
		f.sessionManager.Store(id, session)
		return session
	}
	session := sessionraw.(*FastGoCaptchaSession)
	return session
}

func (f *FastGoCaptcha) GetCaptchaIDFromSession(r *http.Request) (string, error) {
	pathedSession, err := f.GetCaptchaSession(r)
	if err != nil {
		return "", err
	}
	pathedSession.mu.Lock()
	defer pathedSession.mu.Unlock()
	return pathedSession.captchaID, nil
}

func (f *FastGoCaptcha) GetCaptchaSession(r *http.Request) (*PathedSession, error) {
	cookie, err := r.Cookie("fastgocaptcha_session")
	if err != nil {
		return nil, errors.New("session is not found")
	}
	sessionID := cookie.Value
	sessionraw, ok := f.sessionManager.Load(sessionID)
	if !ok {
		return nil, errors.New("session is not found")
	}
	session := sessionraw.(*FastGoCaptchaSession)
	if !time.Now().Before(session.expiresAt) {
		f.sessionManager.Delete(sessionID)
		return nil, errors.New("session expired")
	}
	newpath, err := f.GetCaptchaRequiredPath(r)
	if err != nil {
		return nil, err
	}
	load, ok := session.pathed.Load(newpath)
	if !ok {
		return nil, errors.New("captcha is not required")
	}
	pathedSession, ok := load.(*PathedSession)
	if !ok {
		return nil, errors.New("captcha is not required")
	}
	return pathedSession, nil
}

func (f *FastGoCaptcha) NoNeedCaptcha(r *http.Request) (sessionId string, noNeedCaptcha bool, updateExpiresAt bool) {
	pathedSession, err := f.GetCaptchaSession(r)
	if err != nil {
		f.logInfof("NoNeedCaptcha check pathedSession error: %v", err)
		return "", false, false
	}
	pathedSession.mu.Lock()
	defer pathedSession.mu.Unlock()
	if pathedSession.captchaAllowedTimes <= 0 {
		return pathedSession.id, pathedSession.captchaExpiredAt.After(time.Now()), false
	}
	pathedSession.captchaAllowedTimes--
	return pathedSession.id, true, true
}

func (f *FastGoCaptcha) UpdateSessionCaptchaID(r *http.Request, captchaID string) error {
	pathedSession, err := f.GetCaptchaSession(r)
	if err != nil {
		return err
	}
	pathedSession.mu.Lock()
	defer pathedSession.mu.Unlock()
	pathedSession.captchaID = captchaID
	return nil
}

func (f *FastGoCaptcha) UpdateSessionCaptchaExpiresAt(r *http.Request, timeout time.Duration) error {
	pathedSession, err := f.GetCaptchaSession(r)
	if err != nil {
		return err
	}
	pathedSession.mu.Lock()
	defer pathedSession.mu.Unlock()
	pathedSession.captchaExpiredAt = time.Now().Add(timeout)
	f.logInfof("UpdateSessionCaptchaExpiresAt: %s, %v", pathedSession.id, pathedSession.captchaExpiredAt)
	return nil
}

func (f *FastGoCaptcha) UpdateSessionCaptchaTimes(r *http.Request, times int) error {
	pathedSession, err := f.GetCaptchaSession(r)
	if err != nil {
		return err
	}
	pathedSession.mu.Lock()
	defer pathedSession.mu.Unlock()
	pathedSession.captchaAllowedTimes = times
	return nil
}

func (f *FastGoCaptcha) CreateSessionWithCaptchaIDAndRedirect(w http.ResponseWriter, r *http.Request, captchaID string) error {

	session := f.GetOrCreateSession(r)
	newPath, err := f.GetCaptchaRequiredPath(r)
	if err != nil {
		f.logErrorf("GetCaptchaRequiredPath error: %v", err)
		return err
	}

	var pathedSession *PathedSession
	pathedSessionRaw, ok := session.pathed.Load(newPath)
	if !ok {
		f.logInfof("CreateSessionWithCaptchaIDAndRedirect: newPath not found, create new pathedSession")
		pathedSession = &PathedSession{
			id:        session.id,
			path:      newPath,
			captchaID: captchaID,
		}
		session.pathed.Store(newPath, pathedSession)
	} else {
		f.logInfof("CreateSessionWithCaptchaIDAndRedirect: newPath found, update pathedSession")
		pathedSession, ok := pathedSessionRaw.(*PathedSession)
		if !ok {
			return errors.New("captcha is not required")
		}
		pathedSession.mu.Lock()
		defer pathedSession.mu.Unlock()
		pathedSession.captchaID = captchaID
	}

	cookie, err := r.Cookie("fastgocaptcha_session")
	if err != nil {
		f.logInfof("CreateSessionWithCaptchaIDAndRedirect: session is not found, create new session")
		cookie = &http.Cookie{
			Name:     "fastgocaptcha_session",
			Value:    session.id,
			Path:     "/",
			HttpOnly: true,
			MaxAge:   int(f.sessionTimeout.Seconds()),
			SameSite: http.SameSiteLaxMode,
			Secure:   r.TLS != nil,
		}
	}
	oldId := cookie.Value
	if oldId != session.id {
		cookie = &http.Cookie{
			Name:     "fastgocaptcha_session",
			Value:    session.id,
			Path:     "/",
			HttpOnly: true,
			MaxAge:   int(f.sessionTimeout.Seconds()),
			SameSite: http.SameSiteLaxMode,
			Secure:   r.TLS != nil,
		}
	}
	http.SetCookie(w, cookie)
	http.Redirect(w, r, r.URL.String(), http.StatusFound)
	return nil
}
