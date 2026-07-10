package icon

import (
	"bytes"
	"image/png"
	"testing"
)

func TestColorForLabelIsDeterministic(t *testing.T) {
	a := ColorForLabel("work-account")
	b := ColorForLabel("work-account")
	if a != b {
		t.Errorf("ColorForLabel not deterministic: %v != %v", a, b)
	}
}

func TestColorForLabelDiffersByLabel(t *testing.T) {
	if ColorForLabel("personal-account") == ColorForLabel("side-project") {
		t.Error("distinct labels produced identical colors")
	}
}

func TestColorForLabelIsOpaque(t *testing.T) {
	c := ColorForLabel("anything")
	if c.A != 0xff {
		t.Errorf("color alpha = %d, want 255 (opaque)", c.A)
	}
}

func TestInitial(t *testing.T) {
	cases := map[string]string{
		"work-account":     "W",
		"personal-account": "P",
		"github":           "G",
		"":                 "?",
	}
	for in, want := range cases {
		if got := Initial(in); got != want {
			t.Errorf("Initial(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRenderPNGDecodesToRequestedSize(t *testing.T) {
	size := 32
	data := RenderPNG("personal-account", size)
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("RenderPNG did not produce a valid PNG: %v", err)
	}
	b := img.Bounds()
	if b.Dx() != size || b.Dy() != size {
		t.Errorf("rendered size = %dx%d, want %dx%d", b.Dx(), b.Dy(), size, size)
	}
}

func TestPNGToICOHasIconHeader(t *testing.T) {
	png := RenderPNG("x", 32)
	ico := PNGToICO(png, 32)

	// ICO header: reserved=0, type=1 (icon), count=1  -> 00 00 01 00 01 00
	want := []byte{0x00, 0x00, 0x01, 0x00, 0x01, 0x00}
	if len(ico) < len(want) || !bytes.Equal(ico[:len(want)], want) {
		t.Fatalf("ICO header wrong: got % x", ico[:min(len(ico), 6)])
	}
	// The embedded PNG must be present after the 6+16 byte header/dir entry.
	const headerLen = 22
	if len(ico) != headerLen+len(png) {
		t.Errorf("ICO length = %d, want %d", len(ico), headerLen+len(png))
	}
	if !bytes.Equal(ico[headerLen:], png) {
		t.Error("embedded PNG bytes do not match")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
