package macos

import (
	"errors"
	"fmt"
	"unsafe"

	"github.com/go-macos/objc"
)

// AppIconPNG draws the application icon at pixels by pixels and returns it as
// PNG data.
//
// The art is appIconArt, the same function the About panel draws: the menu-bar
// mark in white on the rounded square macOS gives an application. That is what
// ties the icon in the Finder, in the Accessibility pane and in the About panel
// to the icon in the menu bar -- they are one drawing, not four files to keep
// in step.
//
// build.sh runs cmd/icongen over this for each size in an .iconset and hands
// the directory to iconutil, so the bundle's .icns is generated at build time
// and the project still ships no image assets.
func AppIconPNG(pixels int) ([]byte, error) {
	if pixels < 16 || pixels > 2048 {
		return nil, fmt.Errorf("icon size %d is outside the range AppKit draws well (16-2048)", pixels)
	}
	// One pixel per point: the art is vector, and what the icon set is measured
	// in is the bitmap's pixel count.
	rep, err := bitmapRep(float64(pixels), 1, appIconArt)
	if err != nil {
		return nil, err
	}
	// NSBitmapImageFileTypePNG. The properties dictionary is deliberately nil:
	// its defaults are a plain PNG, which is what an icon is.
	data := rep.Send(objc.Sel("representationUsingType:properties:"), uint64(4), objc.ID(0))
	if data == 0 {
		return nil, errors.New("AppKit would not encode the icon as PNG")
	}
	length := objc.Send[uint64](data, objc.Sel("length"))
	if length == 0 {
		return nil, errors.New("the encoded icon is empty")
	}
	png := make([]byte, length)
	data.Send(objc.Sel("getBytes:length:"), unsafe.Pointer(&png[0]), uintptr(length))
	return png, nil
}
