package fastgocaptcha

import (
	"bytes"
	"encoding/json"
	"image/png"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestImages(t *testing.T) {
	for _, makeImage := range []func() (*Data, error){
		func() (*Data, error) { return New(150, 50) },
		func() (*Data, error) { return NewText(150, 50, TextOptions{Length: 4, Alphabet: "0123456789"}) },
		func() (*Data, error) { return NewArithmetic(150, 50) },
	} {
		d, err := makeImage()
		if err != nil {
			t.Fatal(err)
		}
		var b bytes.Buffer
		if err = d.WriteImage(&b); err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(&b)
		if err != nil || img.Bounds().Dx() != 150 || img.Bounds().Dy() != 50 {
			t.Fatal("invalid PNG", err)
		}
		if !d.Verify(d.Text, false) || d.Verify("incorrect", true) {
			t.Fatal("answer comparison")
		}
		raw, _ := json.Marshal(d)
		if string(raw) != "{}" {
			t.Fatal("answer leaked", string(raw))
		}
	}
	for _, size := range [][2]int{{-1, 50}, {math.MaxInt, 50}, {150, math.MaxInt}, {40, 24}} {
		if _, err := New(size[0], size[1]); err == nil {
			t.Fatal("accepted invalid size", size)
		}
	}
	if _, err := RandomText(6, "11"); err == nil {
		t.Fatal("duplicate alphabet")
	}
	if _, err := RenderText(150, 50, "中文"); err == nil {
		t.Fatal("unsupported glyph")
	}
}
func TestSlide(t *testing.T) {
	s, err := NewSlide(300, 220)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Verify(s.X, 10) || s.Verify(-1, 10) || s.Verify(math.MinInt, 10) || s.Verify(math.MaxInt, 10) || s.Verify(s.X+11, 10) {
		t.Fatal("coordinate validation")
	}
	data, err := s.ClientData("id")
	if err != nil {
		t.Fatal(err)
	}
	if data["fastgocaptcha_thumb_x"] != 0 || data["fastgocaptcha_thumb_y"] != s.Y {
		t.Fatal("client position")
	}
	raw, _ := json.Marshal(s)
	if string(raw) != "{}" {
		t.Fatal("answer leaked")
	}
}
func TestAnswerStore(t *testing.T) {
	s, err := NewAnswerStore(time.Minute, 1)
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.Put("ABcd", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Put("second", false); err == nil {
		t.Fatal("unbounded store")
	}
	var passed atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.Verify(id, "abcd") {
				passed.Add(1)
			}
		}()
	}
	wg.Wait()
	if passed.Load() != 1 {
		t.Fatal("replay", passed.Load())
	}
	id, _ = s.Put("answer", false)
	if s.Verify(id, "wrong") || s.Verify(id, "answer") {
		t.Fatal("wrong attempt did not consume")
	}
	id, _ = s.Put("answer", false)
	v := s.entries[id]
	v.expires = time.Now().Add(-time.Second)
	s.entries[id] = v
	if s.Verify(id, "answer") {
		t.Fatal("expired answer")
	}
	id, _ = s.Put("answer", false)
	s.Delete(id)
	if s.Verify(id, "answer") {
		t.Fatal("deleted answer")
	}
}
