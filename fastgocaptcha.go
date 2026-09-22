package fastgocaptcha

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gobwas/glob"

	"github.com/google/uuid"
)

//go:embed resources/v1.0.9/fastgocaptcha.js
var fastgocaptchaJS []byte

//go:embed resources/v1.0.9/gocaptcha.global.css
var gocaptchaGlobalCSS []byte

//go:embed resources/v1.0.9/gocaptcha.global.js
var gocaptchaGlobalJS []byte

//go:embed resources/index.html
var testPage []byte

type SlideBlockWrapper struct {
	data      *SlideData
	rawData   []byte
	expiresAt time.Time
}

type FastGoCaptchaMatcher struct {
	glob    glob.Glob
	timeout time.Duration
}

type FastGoCaptcha struct {
	verifyMutex      sync.Mutex
	sessionMutex     sync.Mutex
	requestURIPrefix string

	matcherMutex sync.RWMutex
	matchers     map[string]*FastGoCaptchaMatcher

	sessionTimeout time.Duration
	sessionManager *sync.Map

	infof    func(format string, v ...any)
	warningf func(format string, v ...any)
	errorf   func(format string, v ...any)

	storeGoCaptchaData  func(id string, data *SlideBlockWrapper)
	loadGoCaptchaData   func(id string) (*SlideBlockWrapper, bool)
	deleteGoCaptchaData func(id string)
}

func testRoute(glob glob.Glob) (ok bool) {
	ok = true
	for _, route := range []string{
		"/fastgocaptcha/",
		"/fastgocaptcha/captcha",
		"/fastgocaptcha/verify",
		"/fastgocaptcha/resources/fastgocaptcha.js",
		"/fastgocaptcha/resources/gocaptcha.global.css",
		"/fastgocaptcha/resources/gocaptcha.global.js",
		"/fastgocaptcha/session/captcha",
	} {
		if glob.Match(route) {
			return false
		}
	}
	return true
}

func (f *FastGoCaptcha) addProtectMatcher(rawRoute string, timeout time.Duration) error {
	f.matcherMutex.Lock()
	defer f.matcherMutex.Unlock()

	var routes []string = make([]string, 0, 2)
	routes = append(routes, rawRoute)
	if !strings.HasSuffix(rawRoute, "/") {
		routes = append(routes, rawRoute+"/")
	}

	if f.matchers == nil {
		f.matchers = make(map[string]*FastGoCaptchaMatcher)
	}

	for _, route := range routes {

		glob, err := glob.Compile(route, rune('/'))
		if err != nil {
			f.logErrorf("failed to compile glob: %v", err)
			continue
		}

		if !testRoute(glob) {
			f.logErrorf("route %s is not allowed", route)
			continue
		}

		f.matchers[route] = &FastGoCaptchaMatcher{
			glob:    glob,
			timeout: timeout,
		}
	}
	return nil
}

func (f *FastGoCaptcha) AddProtectMatcherWithTimeout(route string, timeout time.Duration) error {
	return f.addProtectMatcher(route, timeout)
}

func (f *FastGoCaptcha) AddProtectMatcherEverytime(route string) error {
	return f.addProtectMatcher(route, 0)
}

func (f *FastGoCaptcha) CheckProtectMatcher(path string) (protected bool, matcher *FastGoCaptchaMatcher) {
	f.matcherMutex.RLock()
	defer f.matcherMutex.RUnlock()
	for _, matcher := range f.matchers {
		if matcher.glob.Match(path) {
			return true, matcher
		}
	}
	return false, nil
}

func (f *FastGoCaptcha) RemoveProtectMatcher(route string) {
	f.matcherMutex.Lock()
	defer f.matcherMutex.Unlock()
	delete(f.matchers, route)
}

type FastGoCaptchaOption func(*FastGoCaptcha)

func WithRequestURIPrefix(prefix string) FastGoCaptchaOption {
	return func(f *FastGoCaptcha) {
		f.requestURIPrefix = prefix
	}
}

func WithStoreGoCaptchaData(store func(id string, data *SlideBlockWrapper)) FastGoCaptchaOption {
	return func(f *FastGoCaptcha) {
		f.storeGoCaptchaData = store
	}
}

func WithLoadGoCaptchaData(load func(id string) (*SlideBlockWrapper, bool)) FastGoCaptchaOption {
	return func(f *FastGoCaptcha) {
		f.loadGoCaptchaData = load
	}
}

func WithDeleteGoCaptchaData(delete func(id string)) FastGoCaptchaOption {
	return func(f *FastGoCaptcha) {
		f.deleteGoCaptchaData = delete
	}
}

func NewFastGoCaptcha(options ...FastGoCaptchaOption) (*FastGoCaptcha, error) {
	captcha := &FastGoCaptcha{}
	for _, option := range options {
		option(captcha)
	}

	// 检查存储相关函数是否都具备
	if (captcha.storeGoCaptchaData != nil || captcha.loadGoCaptchaData != nil || captcha.deleteGoCaptchaData != nil) &&
		(captcha.storeGoCaptchaData == nil || captcha.loadGoCaptchaData == nil || captcha.deleteGoCaptchaData == nil) {
		return nil, fmt.Errorf("store, load, and delete functions must all be provided together")
	}

	// 如果都不具备，使用 sync.Map 作为默认存储
	if captcha.storeGoCaptchaData == nil && captcha.loadGoCaptchaData == nil && captcha.deleteGoCaptchaData == nil {
		var captchaStore sync.Map
		var storeMutex sync.Mutex
		var count int

		captcha.storeGoCaptchaData = func(id string, data *SlideBlockWrapper) {
			storeMutex.Lock()
			defer storeMutex.Unlock()
			now := time.Now()
			captchaStore.Range(func(key, value any) bool {
				if !now.Before(value.(*SlideBlockWrapper).expiresAt) {
					captchaStore.Delete(key)
					count--
				}
				return true
			})
			if _, exists := captchaStore.Load(id); !exists {
				if count >= 4096 {
					var oldestKey any
					var oldest time.Time
					captchaStore.Range(func(key, value any) bool {
						expires := value.(*SlideBlockWrapper).expiresAt
						if oldestKey == nil || expires.Before(oldest) {
							oldestKey, oldest = key, expires
						}
						return true
					})
					captchaStore.Delete(oldestKey)
					count--
				}
				count++
			}
			captchaStore.Store(id, data)
		}

		captcha.loadGoCaptchaData = func(id string) (*SlideBlockWrapper, bool) {
			value, ok := captchaStore.Load(id)
			if !ok {
				return nil, false
			}
			data, ok := value.(*SlideBlockWrapper)
			return data, ok && time.Now().Before(data.expiresAt)
		}

		captcha.deleteGoCaptchaData = func(id string) {
			storeMutex.Lock()
			if _, ok := captchaStore.LoadAndDelete(id); ok {
				count--
			}
			storeMutex.Unlock()
		}
	}

	captcha.sessionManager = new(sync.Map)
	if captcha.sessionTimeout <= 0 {
		captcha.sessionTimeout = 30 * time.Minute
	}
	return captcha, nil
}

func (f *FastGoCaptcha) GetRequestURI() string {
	return f.requestURIPrefix
}

func (f *FastGoCaptcha) GetTestPageHTTPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(testPage)
	})
}

func (f *FastGoCaptcha) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		skipped := f.HandleFastGoCaptcha(w, r)
		if !skipped {
			return
		}

		f.logInfof("checking protected for: %s", r.URL.Path)
		if next != nil {
			// match route and check
			protected, matcher := f.CheckProtectMatcher(r.URL.Path)
			if protected {
				f.logInfof("protected: %s, matcher: %v", r.URL.Path, matcher.glob)
				if id, ok, updatedExpiresAt := f.NoNeedCaptcha(r); ok {
					f.logInfof("no need captcha temporarily, skip, session: %v, path: %v", id, r.URL.Path)
					if updatedExpiresAt {

					}
					next.ServeHTTP(w, r)
					return
				}
				// check captcha
				f.logInfof("captcha need, start to check session's captcha")
				captchaID, err := f.GetCaptchaIDFromSession(r)
				if err != nil || captchaID == "" {
					f.logInfof("captchaID not found, create new captcha")
					captchaID := uuid.New().String()
					rawData, data, err := f.createCaptchaJSON(captchaID)
					if err != nil {
						w.WriteHeader(http.StatusInternalServerError)
						w.Write([]byte("FastGoCaptcha:Failed to create captcha data"))
						return
					}
					f.storeGoCaptchaData(captchaID, &SlideBlockWrapper{
						data:      data,
						rawData:   rawData,
						expiresAt: time.Now().Add(5 * time.Minute),
					})
					f.logInfof("create new captcha, store to session, redirect to captcha page")
					f.CreateSessionWithCaptchaIDAndRedirect(w, r, captchaID)
					return
				}

				x := r.URL.Query().Get("fastgocaptcha_x")
				if x == "" {
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
					authPath := `/fastgocaptcha/session/captcha?fastgocaptcha_path=` + url.QueryEscape(r.URL.Path)
					w.Header().Set("X-FastGoCaptcha-Auth", authPath)
					w.WriteHeader(http.StatusBadRequest)
					w.Write([]byte(
						"<html><body>" +
							"This route requires a x value(fastgocaptcha_x), view <a href='/fastgocaptcha/session/captcha?fastgocaptcha_path=" + url.QueryEscape(r.URL.Path) + "'>here</a>" +
							" for auth it! or with query param fastgocaptcha_x" +
							"</body></html>"))
					return
				}

				xInt, err := strconv.Atoi(x)
				if err != nil {
					w.WriteHeader(http.StatusBadRequest)
					w.Write([]byte("FastGoCaptcha:Invalid x value"))
					return
				}

				if !f.VerifySlide(captchaID, xInt) {
					http.Error(w, "FastGoCaptcha:Verification failed or expired", http.StatusBadRequest)
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

// return value is skipped
func (f *FastGoCaptcha) HandleFastGoCaptcha(w http.ResponseWriter, r *http.Request) (skipped bool) {
	if r.URL.Path == f.requestURIPrefix {
		return true
	}

	// gocaptcha
	removePrefix := strings.TrimPrefix(r.URL.Path, f.requestURIPrefix)
	if removePrefix == "" {
		return true
	}

	if !strings.HasPrefix(removePrefix, "/") {
		removePrefix = "/" + removePrefix
	}

	skipped = true
	switch removePrefix {
	case "/fastgocaptcha/resources/fastgocaptcha.js":
		skipped = false
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Write(fastgocaptchaJS)
	case "/fastgocaptcha/resources/gocaptcha.global.css":
		skipped = false
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Write(gocaptchaGlobalCSS)
	case "/fastgocaptcha/resources/gocaptcha.global.js":
		skipped = false
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Write(gocaptchaGlobalJS)
	case "/fastgocaptcha/verify":
		skipped = false
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		defer r.Body.Close()
		contentType := r.Header.Get("Content-Type")
		var id, xStr string
		var err error

		tolower := strings.ToLower(contentType)
		switch {
		case strings.HasPrefix(tolower, "application/x-www-form-urlencoded"):
			id = r.FormValue("id")
			xStr = r.FormValue("x")
		case strings.HasPrefix(tolower, "multipart/form-data"):
			if err := r.ParseMultipartForm(64 << 10); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.Write([]byte("FastGoCaptcha:Failed to parse multipart form"))
				return
			}
			defer r.MultipartForm.RemoveAll()
			id = r.FormValue("id")
			xStr = r.FormValue("x")
		case strings.HasPrefix(tolower, "application/json"), strings.HasPrefix(tolower, "text/json"), strings.HasPrefix(tolower, "application/x-json"):
			var data struct {
				ID string `json:"id"`
				X  string `json:"x"`
			}
			if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.Write([]byte("FastGoCaptcha:Failed to parse json body"))
				return
			}
			id = data.ID
			xStr = data.X
		default:
			w.WriteHeader(http.StatusUnsupportedMediaType)
			return
		}

		x, err := strconv.Atoi(xStr)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write([]byte("FastGoCaptcha:Invalid x value"))
			return
		}

		// A session may only be authorized by its own challenge.
		sessionID, _ := f.GetCaptchaIDFromSession(r)
		if sessionID != "" && sessionID != id {
			http.Error(w, "FastGoCaptcha:Captcha ID does not match session", http.StatusBadRequest)
			return
		}
		if f.VerifySlide(id, x) {

			f.logInfof("verification successful, update session's captcha times to 1")
			f.UpdateSessionCaptchaTimes(r, 1)
			newPath, _ := f.GetCaptchaRequiredPath(r)
			if newPath != "" {
				protected, matcher := f.CheckProtectMatcher(newPath)
				if protected {
					f.logInfof("verification successful, update session's captcha expires at to %v", matcher.timeout)
					f.UpdateSessionCaptchaExpiresAt(r, matcher.timeout)
				}
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"success": true, "message": "Verification successful"})
		} else {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"message": "Verification failed",
			})
		}
		return
	case "/fastgocaptcha/session/captcha":
		skipped = false

		id, _ := f.GetCaptchaIDFromSession(r)
		if id == "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write([]byte("FastGoCaptcha:Captcha ID is invalid, session is not created"))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(testPage)
		return
	case "/fastgocaptcha/captcha":
		skipped = false
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		id, err := f.GetCaptchaIDFromSession(r)
		if err != nil || id == "" {
			id = uuid.New().String()
		}

		f.logInfof("captchaID: %s, start to load captcha data", id)
		dotDataWrapper, ok := f.loadGoCaptchaData(id)
		if !ok || dotDataWrapper == nil {
			f.logInfof("captchaID: %s, captcha data not found, create new captcha", id)
			raw, dotData, err := f.createCaptchaJSON(id)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.Write([]byte("FastGoCaptcha:Failed to create captcha data"))
				return
			}
			dotDataWrapper = &SlideBlockWrapper{
				data:      dotData,
				rawData:   raw,
				expiresAt: time.Now().Add(5 * time.Minute),
			}
			f.storeGoCaptchaData(id, dotDataWrapper)
		}

		f.logInfof("captchaID: %s, start to check protect matcher", id)
		w.Header().Set("Content-Type", "application/json")
		w.Write(dotDataWrapper.rawData)
	default:
		skipped = true
	}
	return skipped
}

// VerifySlide atomically consumes a stored slide challenge on every attempt.
// Custom storage callbacks must be concurrency safe; shared distributed stores must
// additionally serialize consumption across manager instances.
func (f *FastGoCaptcha) VerifySlide(id string, x int) bool {
	f.verifyMutex.Lock()
	defer f.verifyMutex.Unlock()
	data, ok := f.loadGoCaptchaData(id)
	f.deleteGoCaptchaData(id)
	return ok && data != nil && data.data != nil && time.Now().Before(data.expiresAt) && data.data.Verify(x, 10)
}
