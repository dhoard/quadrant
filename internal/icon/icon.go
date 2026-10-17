// Package icon describes the images a macOS application icon is made of: the
// point sizes macOS draws an icon at, and the scale each image is rendered for.
//
// The names are not a design choice. An `.iconset` directory is turned into an
// `.icns` file by iconutil(1), which accepts exactly this file naming and
// silently produces an icon without the images that are named otherwise. So the
// sizes and the names live here, in one place, with a test that pins them.
package icon

import "fmt"

// Variant is one image in an .iconset: the point size macOS asks for, and the
// screen scale the image is drawn for. A 16-point icon at scale 2 is 32 pixels
// across, and is what a Retina display draws for the small Finder icon.
type Variant struct {
	Points int
	Scale  int
}

// Variants is the whole set iconutil wants for a macOS application icon: every
// point size from 16 to 512, each with its retina image. A bundle icon without
// the larger sizes is scaled up by the Finder and looks soft everywhere the
// icon is shown big, which for this application means the Accessibility pane
// and the About panel.
var Variants = []Variant{
	{Points: 16, Scale: 1},
	{Points: 16, Scale: 2},
	{Points: 32, Scale: 1},
	{Points: 32, Scale: 2},
	{Points: 128, Scale: 1},
	{Points: 128, Scale: 2},
	{Points: 256, Scale: 1},
	{Points: 256, Scale: 2},
	{Points: 512, Scale: 1},
	{Points: 512, Scale: 2},
}

// Name is the file name iconutil expects inside an .iconset directory:
// icon_16x16.png, icon_16x16@2x.png, and so on.
func (v Variant) Name() string {
	scale := ""
	if v.Scale > 1 {
		scale = fmt.Sprintf("@%dx", v.Scale)
	}
	return fmt.Sprintf("icon_%dx%d%s.png", v.Points, v.Points, scale)
}

// Pixels is the width and height the image must be drawn at.
func (v Variant) Pixels() int { return v.Points * v.Scale }
