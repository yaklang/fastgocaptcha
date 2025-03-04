package fastgocaptcha

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

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

type FastGoCaptcha struct {
	requestURIPrefix string
	slideCaptcha     slide.Captcha

	storeGoCaptchaData  func(id string, data *slide.Block)
	loadGoCaptchaData   func(id string) (*slide.Block, bool)
	deleteGoCaptchaData func(id string)
}

type FastGoCaptchaOption func(*FastGoCaptcha)

func WithRequestURIPrefix(prefix string) FastGoCaptchaOption {
	return func(f *FastGoCaptcha) {
		f.requestURIPrefix = prefix
	}
}

func WithStoreGoCaptchaData(store func(id string, data *slide.Block)) FastGoCaptchaOption {
	return func(f *FastGoCaptcha) {
		f.storeGoCaptchaData = store
	}
}

func WithLoadGoCaptchaData(load func(id string) (*slide.Block, bool)) FastGoCaptchaOption {
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

		captcha.storeGoCaptchaData = func(id string, data *slide.Block) {
			captchaStore.Store(id, data)
		}

		captcha.loadGoCaptchaData = func(id string) (*slide.Block, bool) {
			value, ok := captchaStore.Load(id)
			if !ok {
				return nil, false
			}
			data, ok := value.(*slide.Block)
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
		if next != nil {
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
		targetX := info.X
		if abs(x-targetX) <= 10 {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"message": "Verification successful",
			})
		} else {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"message": "Verification failed",
			})
		}
	case "/fastgocaptcha/captcha":
		skipped = false
		captData, err := f.slideCaptcha.Generate()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write([]byte("FastGoCaptcha:Failed to generate captcha"))
			return
		}

		id := uuid.New().String()

		dotData := captData.GetData()
		if dotData == nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write([]byte("FastGoCaptcha:Failed to generate captcha in captData.GetData()"))
			return
		}

		imageBase64, err := captData.GetMasterImage().ToBase64()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write([]byte("FastGoCaptcha:Failed to generate captcha in captData.GetData()"))
			return
		}

		thumbBase64, err := captData.GetTileImage().ToBase64()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write([]byte("FastGoCaptcha:Failed to generate captcha in captData.GetData()"))
			return
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
			w.WriteHeader(http.StatusInternalServerError)
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write([]byte("FastGoCaptcha:Failed to marshal captcha data"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		f.storeGoCaptchaData(id, dotData)
		w.Write(raw)
	default:
		skipped = true
	}
	return skipped
}

// abs 计算绝对值
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
