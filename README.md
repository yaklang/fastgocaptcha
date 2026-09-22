# FastGoCaptcha

Small, offline CAPTCHA generators and HTTP middleware for Go 1.22.6+.
文字、算术、滑块验证码统一入口，无需字体包、图片包、CDN 或运行时下载。

## Dependencies and size

The only direct modules are `github.com/gobwas/glob v0.2.3`,
`github.com/google/uuid v1.6.0` and `golang.org/x/image v0.18.0`.
All three already exist in yaklang/yaklang, at these versions. There is no
 dependency on yaklang/yaklang itself, steambap/captcha, wenlng's Go libraries,
 go-captcha-assets or freetype. Text uses the small basic bitmap font; slide
 backgrounds and puzzle pieces are generated in code. The existing ~65 KB
 browser UI remains embedded. CI caps tracked source and assets at 250 KB
 and the module graph at the root plus these three dependencies and x/image’s
unused transitive golang.org/x/text requirement (also already in yaklang).

## Text images / 文字验证码

```go
data, err := fastgocaptcha.New(150, 50) // six unambiguous uppercase letters/digits
if err != nil { return err }
// Store data.Text on the server; never return the answer in a client response.
err = data.WriteImage(w) // PNG; set Content-Type: image/png in an HTTP handler
```

`New`, `Data.Text` and `Data.WriteImage` cover yaklang's former steambap usage.
This is not an implementation of every steambap font/option API.

```go
data, err := fastgocaptcha.NewText(180, 60, fastgocaptcha.TextOptions{
    Length: 4, Alphabet: "0123456789",
})
data, err = fastgocaptcha.NewArithmetic(180, 60) // Data.Text is the answer
// RenderText renders caller-provided printable ASCII; it does not randomize the answer.
data, err = fastgocaptcha.RenderText(180, 60, "A7B9")
uri, err := data.Base64() // data:image/png;base64,...
err = data.WriteJPG(w, nil)
matched := data.Verify(answer, true) // ignore case; stateless comparison
```

Dimensions are bounded to 40..1024 × 24..512; text must fit. Alphabet entries
must be unique printable non-space ASCII. Length is 1..64. RandomText provides
cryptographically random codes without rendering, e.g. for an application-managed
email verification flow. Delivery, OCR and TOTP are outside this library's scope.
No generation API silently falls back to predictable randomness.

## One-time verification / 一次性校验

```go
store, err := fastgocaptcha.NewAnswerStore(5*time.Minute, 1024)
id, err := store.Put(data.Text, true)
// Return only id and image to the client; bind id to the application's session.
ok := store.Verify(id, submittedAnswer)
store.Delete(id)
```

AnswerStore stores answer hashes, is concurrency safe, has a fixed capacity and
TTL, and consumes an answer on **every** attempt (including wrong answers).
Expired entries are reclaimed during Put; no background goroutines are started.
Put returns an error when capacity is exhausted. Application stores can instead
use Data and implement their own atomic consume operation. For deliberate
vulnerable training scenarios, Data.Verify allows caller-owned lifecycle policy.

Run `go run ./example/text` and open http://localhost:8127 to try text and
arithmetic issuance and verification. This example uses a standalone challenge
ID; real form handlers should bind it to their own session and enforce rate limits.

## Slides / 滑块验证码

```go
slide, err := fastgocaptcha.NewSlide(300, 220)
response, err := slide.ClientData(id) // existing fastgocaptcha_* JSON fields
ok := slide.Verify(submittedX, 10)
```

SlideData.X is the **server-only** target. ClientData sends only the initial
piece position, PNG data URIs and dimensions. SlideData and Data exclude answers
and images from automatic JSON serialization. Slide dimensions must be at least
160 × 80. Verify bounds the coordinate and tolerance, including extreme integers.
Standalone slide verification is stateless; store/consume it in your application
or use the built-in middleware. Procedural CAPTCHAs are lightweight friction,
not protection against sophisticated automated image recognition.

## HTTP middleware / 兼容现有 httpserver

```go
captcha, err := fastgocaptcha.NewFastGoCaptcha()
if err != nil { log.Fatal(err) }
captcha.AddProtectMatcherWithTimeout("/admin/*", 10*time.Minute)
captcha.AddProtectMatcherEverytime("/sensitive")
http.Handle("/", captcha.Middleware(application))
```

The existing constructor, logger setters, storage callbacks, matcher/session
methods and browser resource paths remain available:

- `GET /fastgocaptcha/captcha`: generate/load slide JSON; responses are not cached.
- `POST /fastgocaptcha/verify`: `{ "id": "...", "x": "123" }`, form-urlencoded
  or multipart; response contains `success` and `message`.
- `GET /fastgocaptcha/session/captcha?fastgocaptcha_path=/admin/page`: session UI.
- `/fastgocaptcha/resources/fastgocaptcha.js`, `gocaptcha.global.js`, `gocaptcha.global.css`.

Use the `fastgocaptcha_path` query on issuance and verification for a protected
route. The middleware binds session authorization to that session's challenge,
atomically consumes each attempt and expires challenges after five minutes.
The in-memory slide and session caches are capped at 4096 entries, evicting the
oldest when full. Sessions last 30 minutes. HTTP cookies use HttpOnly/SameSite=Lax
and Secure on TLS connections; applications terminating TLS at a proxy should
apply their own secure-cookie policy. Cleanup is lazy, with no per-request timers.

`VerifySlide(id, x)` exposes the middleware's atomic one-shot check without HTTP.
The three custom storage options (WithStoreGoCaptchaData, WithLoadGoCaptchaData,
WithDeleteGoCaptchaData) must be supplied together and be concurrency safe.
Callbacks sharing a distributed store across managers additionally need an atomic
consume operation across those managers. The wrapper type remains opaque, as in
previous versions. Configure logger callbacks before serving requests.

Browser usage:

```html
<script src="/fastgocaptcha/resources/fastgocaptcha.js"></script>
<script>
showSlideCaptcha({
  captchaUrl: '/fastgocaptcha/captcha',
  verifyUrl: '/fastgocaptcha/verify',
  onSuccess() { /* submit protected operation */ }
});
</script>
```

## Validation

```sh
go test -race ./...
go vet ./...
```

Tests cover PNG decoding, answer secrecy, input bounds, case handling, one-shot
concurrent verification, expiration, session binding, direct protected-route
verification, all three request encodings and embedded UI resources.

## Acknowledgments

The embedded GoCaptcha browser UI is retained from earlier releases and the
wenlng/go-captcha ecosystem. The Go generation engine is now implemented here.
See LICENSE for this project's MIT license.
