// Package icon generates per-identity tray icons: a filled colored circle with
// the identity's initial, encoded as a Windows .ico. The color is derived
// deterministically from the identity label so each identity keeps a stable hue.
package icon

import (
	"bytes"
	"encoding/binary"
	"hash/fnv"
	"image"
	"image/color"
	"image/png"
	"math"
	"strings"
	"unicode"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// ColorForLabel maps a label to a stable, opaque, reasonably vivid color.
func ColorForLabel(label string) color.RGBA {
	h := fnv.New32a()
	_, _ = h.Write([]byte(label))
	hue := float64(h.Sum32() % 360)
	r, g, b := hsvToRGB(hue, 0.60, 0.80)
	return color.RGBA{R: r, G: g, B: b, A: 0xff}
}

// Initial returns the uppercased first letter/digit of label, or "?" if none.
func Initial(label string) string {
	for _, r := range label {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return strings.ToUpper(string(r))
		}
	}
	return "?"
}

// RenderPNG draws a size×size icon (colored disc + white initial) as PNG bytes.
func RenderPNG(label string, size int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	col := ColorForLabel(label)

	cx, cy := float64(size)/2, float64(size)/2
	rad := float64(size)/2 - 1
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
			if dx*dx+dy*dy <= rad*rad {
				img.Set(x, y, col)
			}
		}
	}

	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(color.White),
		Face: basicfont.Face7x13,
	}
	s := Initial(label)
	adv := d.MeasureString(s)
	d.Dot = fixed.Point26_6{
		X: fixed.I(size)/2 - adv/2,
		Y: fixed.I(size)/2 + fixed.I(4),
	}
	d.DrawString(s)

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// PNGToICO wraps a PNG image in a single-image .ico container.
func PNGToICO(pngData []byte, size int) []byte {
	var buf bytes.Buffer
	// ICONDIR
	_ = binary.Write(&buf, binary.LittleEndian, uint16(0)) // reserved
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1)) // type: icon
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1)) // image count

	// ICONDIRENTRY
	dim := byte(size)
	if size >= 256 {
		dim = 0 // 0 means 256 in the ICO format
	}
	buf.WriteByte(dim) // width
	buf.WriteByte(dim) // height
	buf.WriteByte(0)   // palette color count
	buf.WriteByte(0)   // reserved
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))              // color planes
	_ = binary.Write(&buf, binary.LittleEndian, uint16(32))             // bits per pixel
	_ = binary.Write(&buf, binary.LittleEndian, uint32(len(pngData)))   // image data size
	_ = binary.Write(&buf, binary.LittleEndian, uint32(6+16))           // offset to image data

	buf.Write(pngData)
	return buf.Bytes()
}

// RenderICO renders a label's icon directly as .ico bytes for systray.
func RenderICO(label string, size int) []byte {
	return PNGToICO(RenderPNG(label, size), size)
}

func hsvToRGB(h, s, v float64) (uint8, uint8, uint8) {
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := v - c
	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return uint8((r + m) * 255), uint8((g + m) * 255), uint8((b + m) * 255)
}
