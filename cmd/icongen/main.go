// Command icongen writes the application icon as an .iconset directory: one PNG
// per size and scale, named the way iconutil(1) requires, which then turns the
// directory into the .icns the bundle carries.
//
// It exists because the icon is drawn, not shipped: the art lives in the
// program (internal/macos), so the icon in the Finder, the icon in the
// Accessibility pane and the one in the About panel are the same function.
// build.sh runs this on macOS only -- the drawing needs AppKit -- and a bundle
// built elsewhere simply has no icon.
//
//	icongen -out /path/to/AppIcon.iconset
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"quadrant/internal/icon"
	"quadrant/internal/macos"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("icongen: ")
	out := flag.String("out", "", "directory to write the .iconset into (created if missing)")
	flag.Parse()
	if *out == "" {
		log.Fatal("-out is required")
	}

	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	for _, variant := range icon.Variants {
		png, err := macos.AppIconPNG(variant.Pixels())
		if err != nil {
			log.Fatalf("%s: %v", variant.Name(), err)
		}
		path := filepath.Join(*out, variant.Name())
		if err := os.WriteFile(path, png, 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s (%d pixels)\n", variant.Name(), variant.Pixels())
	}
}
