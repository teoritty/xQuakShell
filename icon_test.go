package main

import (
	"bytes"
	"image"
	_ "image/png"
	"testing"
)

// pngSignature is the 8-byte header every PNG file starts with.
var pngSignature = []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}

// The embedded icon is handed straight to GTK via linux.Options.Icon, and Wails silently skips
// setting the window icon when the slice is empty (see SetWindowIcon in the Wails Linux frontend).
// A missing or corrupted icon therefore produces no error anywhere — just an app with no icon,
// which is exactly the defect this file guards against.
func TestAppIconIsAUsablePNG(t *testing.T) {
	if len(appIcon) == 0 {
		t.Fatal("appIcon is empty; the Linux window icon would be silently skipped")
	}
	if !bytes.HasPrefix(appIcon, pngSignature) {
		t.Fatal("appIcon is not a PNG; gdk-pixbuf would fail to load it")
	}

	cfg, format, err := image.DecodeConfig(bytes.NewReader(appIcon))
	if err != nil {
		t.Fatalf("decoding appIcon: %v", err)
	}
	if format != "png" {
		t.Errorf("appIcon format = %q, want png", format)
	}
	// Small enough and the icon looks blurry when the desktop scales it up for the taskbar.
	if cfg.Width < 64 || cfg.Height < 64 {
		t.Errorf("appIcon is %dx%d, want at least 64x64", cfg.Width, cfg.Height)
	}
	if cfg.Width != cfg.Height {
		t.Errorf("appIcon is %dx%d, want a square icon", cfg.Width, cfg.Height)
	}
}
