package fastgocaptcha

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"io"
)

// SlideData is server-side challenge data. X must never be sent to the client.
type SlideData struct {
	X      int         `json:"-"`
	Y      int         `json:"-"`
	Width  int         `json:"-"`
	Height int         `json:"-"`
	TileX  int         `json:"-"`
	TileY  int         `json:"-"`
	Image  image.Image `json:"-"`
	Tile   image.Image `json:"-"`
}

func (s *SlideData) Verify(x, tolerance int) bool {
	return tolerance >= 0 && tolerance <= 10 && x >= 0 && x <= s.Image.Bounds().Dx()-s.Width && x >= s.X-tolerance && x <= s.X+tolerance
}

// NewSlide generates a jigsaw using procedural artwork; no asset downloads or image packs.
func NewSlide(width, height int) (*SlideData, error) {
	if err := validSize(width, height); err != nil {
		return nil, err
	}
	if width < 160 || height < 80 {
		return nil, errors.New("slide dimensions must be at least 160 by 80")
	}
	const size = 40
	x, err := randomInt(width - 2*size - 10)
	if err != nil {
		return nil, err
	}
	x += size + 5
	y, err := randomInt(height - size - 10)
	if err != nil {
		return nil, err
	}
	y += 5
	seed := make([]byte, 64)
	if _, err = io.ReadFull(rand.Reader, seed); err != nil {
		return nil, err
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for py := 0; py < height; py++ {
		for px := 0; px < width; px++ {
			v := int(seed[(px/23+py/19)%len(seed)])
			img.SetRGBA(px, py, color.RGBA{uint8(70 + (px+v)%140), uint8(80 + (py+v)%130), uint8(100 + (px+py+v)%110), 255})
		}
	}
	// Overlay random colored discs to create continuous features across the cutout.
	for i := 0; i < 16; i++ {
		cx, cy := int(seed[i])*width/256, int(seed[i+16])*height/256
		radius := 8 + int(seed[i+32]%30)
		col := color.RGBA{seed[i]/2 + 80, seed[i+16]/2 + 80, seed[i+32]/2 + 80, 255}
		for dy := -radius; dy <= radius; dy++ {
			for dx := -radius; dx <= radius; dx++ {
				if dx*dx+dy*dy <= radius*radius {
					img.SetRGBA(cx+dx, cy+dy, col)
				}
			}
		}
	}
	tile := image.NewRGBA(image.Rect(0, 0, size, size))
	for ty := 0; ty < size; ty++ {
		for tx := 0; tx < size; tx++ {
			// Body, top tab and right tab, with a left circular notch.
			inside := tx >= 5 && tx < 35 && ty >= 8 && ty < 38
			inside = inside || (tx-20)*(tx-20)+(ty-8)*(ty-8) < 49 || (tx-34)*(tx-34)+(ty-23)*(ty-23) < 36
			inside = inside && !((tx-5)*(tx-5)+(ty-23)*(ty-23) < 25)
			if inside {
				col := img.RGBAAt(x+tx, y+ty)
				tile.SetRGBA(tx, ty, col)
				img.SetRGBA(x+tx, y+ty, color.RGBA{col.R / 3, col.G / 3, col.B / 3, 255})
			}
		}
	}
	// Preserve an immutable copy in the result.
	master := image.NewRGBA(img.Bounds())
	draw.Draw(master, master.Bounds(), img, image.Point{}, draw.Src)
	return &SlideData{X: x, Y: y, Width: size, Height: size, TileX: 0, TileY: y, Image: master, Tile: tile}, nil
}

func (s *SlideData) ClientData(id string) (map[string]any, error) {
	bg, err := (&Data{Image: s.Image}).Base64()
	if err != nil {
		return nil, err
	}
	tile, err := (&Data{Image: s.Tile}).Base64()
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"fastgocaptcha_id": id, "fastgocaptcha_image_base64": bg, "fastgocaptcha_thumb_base64": tile,
		"fastgocaptcha_thumb_width": s.Width, "fastgocaptcha_thumb_height": s.Height,
		"fastgocaptcha_thumb_x": s.TileX, "fastgocaptcha_thumb_y": s.TileY,
	}, nil
}
func (f *FastGoCaptcha) createCaptchaJSON(id string) ([]byte, *SlideData, error) {
	slide, err := NewSlide(300, 220)
	if err != nil {
		return nil, nil, err
	}
	data, err := slide.ClientData(id)
	if err != nil {
		return nil, nil, err
	}
	raw, err := json.Marshal(data)
	return raw, slide, err
}
