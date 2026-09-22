package fastgocaptcha

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func issue(t *testing.T, f *FastGoCaptcha, cookie *http.Cookie, path string) (string, int) {
	t.Helper()
	r := httptest.NewRequest("GET", "/fastgocaptcha/captcha?fastgocaptcha_path="+url.QueryEscape(path), nil)
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	f.Middleware(nil).ServeHTTP(w, r)
	var data map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatal(w.Code, w.Body.String(), err)
	}
	id := data["fastgocaptcha_id"].(string)
	stored, ok := f.loadGoCaptchaData(id)
	if !ok {
		t.Fatal("not stored")
	}
	return id, stored.data.X
}
func TestHTTPProtocol(t *testing.T) {
	for _, contentType := range []string{"application/json", "application/x-www-form-urlencoded", "multipart/form-data; boundary=test"} {
		t.Run(contentType, func(t *testing.T) {
			f, err := NewFastGoCaptcha()
			if err != nil {
				t.Fatal(err)
			}
			if err = f.AddProtectMatcherWithTimeout("/private", time.Minute); err != nil {
				t.Fatal(err)
			}
			h := f.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "protected content") }))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", "/private", nil))
			if w.Code != 302 || len(w.Result().Cookies()) != 1 {
				t.Fatal("missing session", w.Code)
			}
			cookie := w.Result().Cookies()[0]
			if cookie.Secure {
				t.Fatal("HTTP cookie unusable")
			}
			id, x := issue(t, f, cookie, "/private")
			body := fmt.Sprintf(`{"id":%q,"x":"%d"}`, id, x)
			if strings.HasPrefix(contentType, "application/x-www") {
				body = url.Values{"id": {id}, "x": {fmt.Sprint(x)}}.Encode()
			}
			if strings.HasPrefix(contentType, "multipart") {
				body = fmt.Sprintf("--test\r\nContent-Disposition: form-data; name=\"id\"\r\n\r\n%s\r\n--test\r\nContent-Disposition: form-data; name=\"x\"\r\n\r\n%d\r\n--test--\r\n", id, x)
			}
			r := httptest.NewRequest("POST", "/fastgocaptcha/verify?fastgocaptcha_path=/private", strings.NewReader(body))
			r.AddCookie(cookie)
			r.Header.Set("Content-Type", contentType)
			w = httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if !strings.Contains(w.Body.String(), `"success":true`) {
				t.Fatal(w.Code, w.Body.String())
			}
			if f.VerifySlide(id, x) {
				t.Fatal("replayed")
			}
			r = httptest.NewRequest("GET", "/private", nil)
			r.AddCookie(cookie)
			w = httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Body.String() != "protected content" {
				t.Fatal(w.Code, w.Body.String())
			}
			for _, resource := range []string{"fastgocaptcha.js", "gocaptcha.global.js", "gocaptcha.global.css"} {
				w = httptest.NewRecorder()
				h.ServeHTTP(w, httptest.NewRequest("GET", "/fastgocaptcha/resources/"+resource, nil))
				if w.Code != 200 || w.Body.Len() == 0 {
					t.Fatal(resource)
				}
			}
		})
	}
}
func TestSlideConsumption(t *testing.T) {
	f, _ := NewFastGoCaptcha()
	id, x := issue(t, f, nil, "")
	var success atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if f.VerifySlide(id, x) {
				success.Add(1)
			}
		}()
	}
	wg.Wait()
	if success.Load() != 1 {
		t.Fatal("concurrent replay")
	}
	id, x = issue(t, f, nil, "")
	if f.VerifySlide(id, -1) || f.VerifySlide(id, x) {
		t.Fatal("failed attempt reusable")
	}
	id, x = issue(t, f, nil, "")
	v, _ := f.loadGoCaptchaData(id)
	v.expiresAt = time.Now().Add(-time.Second)
	if f.VerifySlide(id, x) {
		t.Fatal("expired captcha")
	}
}
func TestSessionBindingAndExpiry(t *testing.T) {
	f, _ := NewFastGoCaptcha()
	f.AddProtectMatcherEverytime("/private")
	h := f.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") }))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/private", nil))
	cookie := w.Result().Cookies()[0]
	other, x := issue(t, f, nil, "")
	r := httptest.NewRequest("POST", "/fastgocaptcha/verify?fastgocaptcha_path=/private", strings.NewReader(fmt.Sprintf(`{"id":%q,"x":"%d"}`, other, x)))
	r.Header.Set("Content-Type", "application/json")
	r.AddCookie(cookie)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatal("accepted another challenge")
	}
	id, x := issue(t, f, cookie, "/private")
	r = httptest.NewRequest("GET", fmt.Sprintf("/private?fastgocaptcha_x=%d", x), nil)
	r.AddCookie(cookie)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Body.String() != "ok" || f.VerifySlide(id, x) {
		t.Fatal("direct verification")
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code == 200 {
		t.Fatal("direct replay")
	}
	session, _ := f.sessionManager.Load(cookie.Value)
	session.(*FastGoCaptchaSession).expiresAt = time.Now().Add(-time.Second)
	if _, err := f.GetCaptchaIDFromSession(r); err == nil {
		t.Fatal("expired session")
	}
}
