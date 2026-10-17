package icon

import (
	"testing"
)

// iconutil ignores an image whose name it does not recognise, and the icon it
// builds is then missing a size with nothing in the build output to say so.
// These are the names, spelled out.
func TestVariantNames(t *testing.T) {
	want := []string{
		"icon_16x16.png", "icon_16x16@2x.png",
		"icon_32x32.png", "icon_32x32@2x.png",
		"icon_128x128.png", "icon_128x128@2x.png",
		"icon_256x256.png", "icon_256x256@2x.png",
		"icon_512x512.png", "icon_512x512@2x.png",
	}
	if len(Variants) != len(want) {
		t.Fatalf("Variants has %d entries, want %d", len(Variants), len(want))
	}
	for i, variant := range Variants {
		if got := variant.Name(); got != want[i] {
			t.Errorf("Variants[%d].Name() = %q, want %q", i, got, want[i])
		}
	}
}

// The pixel sizes are what the art is rendered at, and the macOS icon sizes are
// 16/32/128/256/512 points; anything else is a scaling artefact.
func TestVariantPixels(t *testing.T) {
	want := []int{16, 32, 32, 64, 128, 256, 256, 512, 512, 1024}
	for i, variant := range Variants {
		if got := variant.Pixels(); got != want[i] {
			t.Errorf("%s: Pixels() = %d, want %d", variant.Name(), got, want[i])
		}
		if got := variant.Points * variant.Scale; got != variant.Pixels() {
			t.Errorf("%s: %d points at scale %d is %d, not %d", variant.Name(), variant.Points, variant.Scale, got, variant.Pixels())
		}
	}
}

// Two variants that share a name overwrite each other in the icon set, and the
// icon loses a size without any error.
func TestVariantsAreDistinct(t *testing.T) {
	seen := map[string]bool{}
	for _, variant := range Variants {
		name := variant.Name()
		if seen[name] {
			t.Errorf("%s is listed twice", name)
		}
		seen[name] = true
	}
}

// Every size macOS asks for needs both of its images: one for a standard
// display and one for a Retina display, and 1024 pixels is the largest one
// macOS uses.
func TestVariantsCoverTheSizesMacOSUses(t *testing.T) {
	scales := map[int]map[int]bool{}
	largest := 0
	for _, variant := range Variants {
		if scales[variant.Points] == nil {
			scales[variant.Points] = map[int]bool{}
		}
		scales[variant.Points][variant.Scale] = true
		if variant.Pixels() > largest {
			largest = variant.Pixels()
		}
	}
	for _, size := range []int{16, 32, 128, 256, 512} {
		if !scales[size][1] || !scales[size][2] {
			t.Errorf("%d points needs a 1x and a 2x image, has %v scales", size, scales[size])
		}
	}
	if largest != 1024 {
		t.Errorf("largest image is %d pixels, want 1024", largest)
	}
}
