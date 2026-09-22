package fastgocaptcha

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"math/big"
	"strconv"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

const DefaultAlphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"

// Data contains a server-side answer and its rendered image. Text is never JSON encoded.
type Data struct {
	Text  string      `json:"-"`
	Image image.Image `json:"-"`
}

func (d *Data) WriteImage(w io.Writer) error { return png.Encode(w, d.Image) }
func (d *Data) WriteJPG(w io.Writer, options *jpeg.Options) error {
	return jpeg.Encode(w, d.Image, options)
}
func (d *Data) Base64() (string, error) {
	var b bytes.Buffer
	if err := d.WriteImage(&b); err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes()), nil
}

// Verify compares the answer. Stateful applications should use AnswerStore to prevent replay.
func (d *Data) Verify(answer string, ignoreCase bool) bool {
	expected := d.Text
	if ignoreCase {
		expected, answer = strings.ToUpper(expected), strings.ToUpper(answer)
	}
	return expected != "" && subtle.ConstantTimeCompare([]byte(expected), []byte(answer)) == 1
}

type TextOptions struct {
	Length   int
	Alphabet string
}

// RandomText returns cryptographically random printable ASCII text, at most 64 characters.
func RandomText(length int, alphabet string) (string, error) {
	if length < 1 || length > 64 || len(alphabet) < 2 || len(alphabet) > 95 {
		return "", errors.New("invalid text length or alphabet")
	}
	seen := make(map[byte]bool)
	for i := range alphabet {
		if alphabet[i] < 33 || alphabet[i] > 126 || seen[alphabet[i]] {
			return "", errors.New("alphabet must contain unique printable ASCII characters")
		}
		seen[alphabet[i]] = true
	}
	out := make([]byte, length)
	for i := range out {
		n, err := randomInt(len(alphabet))
		if err != nil {
			return "", err
		}
		out[i] = alphabet[n]
	}
	return string(out), nil
}

func randomInt(n int) (int, error) {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0, err
	}
	return int(v.Int64()), nil
}

func newID() (string, error) {
	var b [24]byte
	if _, err := io.ReadFull(rand.Reader, b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// New generates a six-character image, providing the New/Text/WriteImage migration API.
func New(width, height int) (*Data, error) { return NewText(width, height, TextOptions{}) }

func NewText(width, height int, options TextOptions) (*Data, error) {
	if err := validSize(width, height); err != nil {
		return nil, err
	}
	if options.Length == 0 {
		options.Length = 6
	}
	if options.Alphabet == "" {
		options.Alphabet = DefaultAlphabet
	}
	text, err := RandomText(options.Length, options.Alphabet)
	if err != nil {
		return nil, err
	}
	return RenderText(width, height, text)
}

// NewArithmetic renders addition or subtraction of two one-digit operands.
// Data.Text contains the numeric answer, never the expression.
func NewArithmetic(width, height int) (*Data, error) {
	if err := validSize(width, height); err != nil {
		return nil, err
	}
	a, err := randomInt(10)
	if err != nil {
		return nil, err
	}
	b, err := randomInt(10)
	if err != nil {
		return nil, err
	}
	op, err := randomInt(2)
	if err != nil {
		return nil, err
	}
	sign, answer := "+", a+b
	if op == 1 {
		if a < b {
			a, b = b, a
		}
		sign, answer = "-", a-b
	}
	d, err := RenderText(width, height, fmt.Sprintf("%d %s %d = ?", a, sign, b))
	if err != nil {
		return nil, err
	}
	d.Text = strconv.Itoa(answer)
	return d, nil
}

func validSize(width, height int) error {
	if width < 40 || height < 24 || width > 1024 || height > 512 {
		return errors.New("image dimensions must be within 40..1024 by 24..512")
	}
	return nil
}

// RenderText renders caller-provided printable ASCII. It does not generate entropy for the answer.
func RenderText(width, height int, text string) (*Data, error) {
	if err := validSize(width, height); err != nil {
		return nil, err
	}
	if len(text) == 0 || len(text) > 64 {
		return nil, errors.New("text length must be 1..64")
	}
	for _, r := range text {
		if r < 32 || r > 126 {
			return nil, errors.New("text must be printable ASCII")
		}
	}
	scale := min((width-12)/(7*len(text)), (height-8)/15)
	if scale < 1 {
		return nil, errors.New("text does not fit image")
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{245, 248, 251, 255}), image.Point{}, draw.Src)
	// Random texture and per-character baseline offsets, without embedded fonts.
	noise := make([]byte, width*height/12+len(text))
	if _, err := io.ReadFull(rand.Reader, noise); err != nil {
		return nil, err
	}
	for i, v := range noise[:len(noise)-len(text)] {
		x, y := (i*37+int(v))%width, (i*17+int(v))%height
		img.SetRGBA(x, y, color.RGBA{v/2 + 100, v/3 + 140, v/4 + 170, 255})
	}
	left := (width - len(text)*7*scale) / 2
	for i, c := range text {
		glyph := image.NewRGBA(image.Rect(0, 0, 7, 15))
		drawer := font.Drawer{Dst: glyph, Src: image.NewUniform(color.RGBA{25, 45, 70, 255}), Face: basicfont.Face7x13, Dot: fixed.P(0, 12)}
		drawer.DrawString(string(c))
		top := (height-15*scale)/2 + int(noise[len(noise)-len(text)+i]%3) - 1
		for y := 0; y < 15*scale; y++ {
			for x := 0; x < 7*scale; x++ {
				col := glyph.RGBAAt(x/scale, y/scale)
				if col.A != 0 {
					img.SetRGBA(left+i*7*scale+x, top+y, col)
				}
			}
		}
	}
	return &Data{Text: text, Image: img}, nil
}
