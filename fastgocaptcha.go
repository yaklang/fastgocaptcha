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
	"github.com/wenlng/go-captcha-assets/resources/images"
	"github.com/wenlng/go-captcha-assets/resources/tiles"
	"github.com/wenlng/go-captcha/v2/slide"
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
	data    *slide.Block
	rawData []byte
}

type FastGoCaptchaMatcher struct {
	glob    glob.Glob
	timeout time.Duration
}

type FastGoCaptcha struct {
	requestURIPrefix string
	slideCaptcha     slide.Captcha

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

func (f *FastGoCaptcha) AddProtectMatcherWithTimeout(route string, timeout time.Duration) error {
	f.matcherMutex.Lock()
	defer f.matcherMutex.Unlock()
	if f.matchers == nil {
		f.matchers = make(map[string]*FastGoCaptchaMatcher)
	}
	glob, err := glob.Compile(route, rune('/'))
	if err != nil {
		return err
	}
	f.matchers[route] = &FastGoCaptchaMatcher{
		glob:    glob,
		timeout: timeout,
	}
	return nil
}

func (f *FastGoCaptcha) AddProtectMatcherEverytime(route string) error {
	f.matcherMutex.Lock()
	defer f.matcherMutex.Unlock()
	if f.matchers == nil {
		f.matchers = make(map[string]*FastGoCaptchaMatcher)
	}
	glob, err := glob.Compile(route, rune('/'))
	if err != nil {
		return err
	}
	f.matchers[route] = &FastGoCaptchaMatcher{
		glob:    glob,
		timeout: 0,
	}
	return nil
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

		captcha.storeGoCaptchaData = func(id string, data *SlideBlockWrapper) {
			captchaStore.Store(id, data)
		}

		captcha.loadGoCaptchaData = func(id string) (*SlideBlockWrapper, bool) {
			value, ok := captchaStore.Load(id)
			if !ok {
				return nil, false
			}
			data, ok := value.(*SlideBlockWrapper)
			return data, ok
		}

		captcha.deleteGoCaptchaData = func(id string) {
			captchaStore.Delete(id)
		}
	}

	builder := slide.NewBuilder(
		slide.WithEnableGraphVerticalRandom(true),
	)
	imgs, err := images.GetImages()
	if err != nil {
		return nil, err
	}

	graphs, err := tiles.GetTiles()
	if err != nil {
		return nil, err
	}

	var newGraphs = make([]*slide.GraphImage, 0, len(graphs))
	for i := 0; i < len(graphs); i++ {
		graph := graphs[i]
		newGraphs = append(newGraphs, &slide.GraphImage{
			OverlayImage: graph.OverlayImage,
			MaskImage:    graph.MaskImage,
			ShadowImage:  graph.ShadowImage,
		})
	}

	builder.SetResources(
		slide.WithGraphImages(newGraphs),
		slide.WithBackgrounds(imgs),
	)

	captcha.slideCaptcha = builder.Make()
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
						data:    data,
						rawData: rawData,
					})
					f.logInfof("create new captcha, store to session, redirect to captcha page")
					f.CreateSessionWithCaptchaIDAndRedirect(w, r, captchaID)
					return
				}

				var x string
				x = r.URL.Query().Get("fastgocaptcha_x")
				if x == "" {
					w.WriteHeader(http.StatusBadRequest)
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
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

				captchaData, ok := f.loadGoCaptchaData(captchaID)
				if !ok {
					w.WriteHeader(http.StatusBadRequest)
					w.Write([]byte("FastGoCaptcha:Captcha ID is invalid, no captcha data found"))
					return
				}

				if abs(captchaData.data.X-xInt) > 10 {
					w.WriteHeader(http.StatusBadRequest)
					w.Write([]byte("FastGoCaptcha:Verification failed"))
					return
				}

				// 用完即删，防止重放攻击
				f.deleteGoCaptchaData(captchaID)
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
	case "/static/fastgocaptcha/fastgocaptcha.js":
		skipped = false
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Write(fastgocaptchaJS)
	case "/static/fastgocaptcha/gocaptcha.global.css":
		skipped = false
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Write(gocaptchaGlobalCSS)
	case "/static/fastgocaptcha/gocaptcha.global.js":
		skipped = false
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Write(gocaptchaGlobalJS)
	case "/fastgocaptcha/verify":
		skipped = false
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		contentType := r.Header.Get("Content-Type")
		var id, xStr string
		var err error

		tolower := strings.ToLower(contentType)
		switch {
		case strings.HasPrefix(tolower, "application/x-www-form-urlencoded"):
			id = r.FormValue("id")
			xStr = r.FormValue("x")
		case strings.HasPrefix(tolower, "multipart/form-data"):
			if err := r.ParseMultipartForm(32 << 20); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.Write([]byte("FastGoCaptcha:Failed to parse multipart form"))
				return
			}
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

		// 获取存储的验证码信息
		info, ok := f.loadGoCaptchaData(id)
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write([]byte("FastGoCaptcha:Captcha expired or invalid"))
			return
		}

		// 用完即删，防止重放攻击
		defer f.deleteGoCaptchaData(id)

		// 验证滑动结果
		if info == nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write([]byte("FastGoCaptcha:Invalid captcha data"))
			return
		}

		// 允许一定的误差范围（10像素）
		targetX := info.data.X
		if abs(x-targetX) <= 10 {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"message": "Verification successful",
			})
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

		id, err := f.GetCaptchaIDFromSession(r)
		if err != nil || id == "" {
			id = strings.TrimSpace(r.URL.Query().Get("id"))
			if id == "" {
				f.logWarningf("captchaID not found, create new captcha, this should not happen, nonsense")
				id = uuid.New().String()
			}
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
				data:    dotData,
				rawData: raw,
			}
			f.storeGoCaptchaData(id, dotDataWrapper)
			go func() {
				time.Sleep(f.sessionTimeout)
				f.deleteGoCaptchaData(id)
			}()
		}

		f.logInfof("captchaID: %s, start to check protect matcher", id)
		w.Header().Set("Content-Type", "application/json")
		w.Write(dotDataWrapper.rawData)
	default:
		skipped = true
	}
	return skipped
}

func (f *FastGoCaptcha) createCaptchaJSON(id string) ([]byte, *slide.Block, error) {
	captData, err := f.slideCaptcha.Generate()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate captcha: %v", err)
	}
	dotData := captData.GetData()
	if dotData == nil {
		return nil, nil, fmt.Errorf("failed to generate captcha in captData.GetData()")
	}
	imageBase64, err := captData.GetMasterImage().ToBase64()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate captcha incaptData.GetMasterImage().ToBase64(): %v", err)
	}

	thumbBase64, err := captData.GetTileImage().ToBase64()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate captcha in captData.GetTileImage().ToBase64(): %v", err)
	}

	raw, err := json.Marshal(map[string]any{
		"fastgocaptcha_id":           fmt.Sprint(id),
		"fastgocaptcha_image_base64": imageBase64,
		"fastgocaptcha_thumb_base64": thumbBase64,
		"fastgocaptcha_thumb_width":  dotData.Width,
		"fastgocaptcha_thumb_height": dotData.Height,
		"fastgocaptcha_thumb_x":      dotData.TileX,
		"fastgocaptcha_thumb_y":      dotData.TileY,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal captcha data: %v", err)
	}
	return raw, dotData, nil
}

// abs 计算绝对值
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
