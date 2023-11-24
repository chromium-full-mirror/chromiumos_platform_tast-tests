// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package guestos

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/errors"
)

const gtkSettingsIni = `[Settings]
gtk-cursor-blink = false
`

const imageFilename = "bg.png"

// 2x1 PNG, #1F1F1F
var solidColorImage = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x02, 0x00, 0x00, 0x00, 0x7b, 0x40, 0xe8, 0xdd, 0x00, 0x00, 0x00,
	0x0f, 0x49, 0x44, 0x41, 0x54, 0x08, 0xd7, 0x63, 0x94, 0x97, 0x97, 0x67,
	0x60, 0x60, 0x00, 0x00, 0x01, 0xdf, 0x00, 0x5f, 0x99, 0x8b, 0x56, 0x1a,
	0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

// DisableCursorBlinking disables cursor blinking for GTK applications, which
// may interfere with uidetection and screenshot testing.
func DisableCursorBlinking(ctx context.Context, guest vm.Guest) error {
	content, err := guest.ReadFile(ctx, ".config/gtk-3.0/settings.ini")
	if err == nil && content != gtkSettingsIni {
		return errors.Errorf("failed to check GTK3 X11 settings do not exist: got %q", content)
	}

	if err := guest.WriteFile(ctx, ".config/gtk-3.0/settings.ini", gtkSettingsIni); err != nil {
		return errors.Wrap(err, "failed to set GTK3 X11 settings")
	}

	content, err = guest.ReadFile(ctx, ".config/gtk-4.0/settings.ini")
	if err == nil && content != gtkSettingsIni {
		return errors.Errorf("failed to check GTK4 X11 settings do not exist: got %q", content)
	}

	if err := guest.WriteFile(ctx, ".config/gtk-4.0/settings.ini", gtkSettingsIni); err != nil {
		return errors.Wrap(err, "failed to set GTK4 X11 settings")
	}

	if err := guest.Command(ctx, "gsettings", "set", "org.gnome.desktop.interface", "cursor-blink", "false").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to set GTK Wayland settings")
	}

	return nil
}

// SetSolidColorWallpaper changes the wallpaper to a solid color.
func SetSolidColorWallpaper(ctx context.Context, tconn *chrome.TestConn) error {
	tempdir, err := os.MkdirTemp("", "bg")
	if err != nil {
		return errors.Wrap(err, "failed to create temp dir")
	}

	defer os.RemoveAll(tempdir)

	if err := os.WriteFile(filepath.Join(tempdir, imageFilename), solidColorImage, 0644); err != nil {
		return errors.Wrap(err, "failed to write image file")
	}

	server := httptest.NewServer(http.FileServer(http.Dir(tempdir)))
	defer server.Close()

	if err := tconn.Call(ctx, nil, `(url) => tast.promisify(chrome.wallpaper.setWallpaper)({
		url: url,
		layout: 'STRETCH',
		filename: 'test_wallpaper'
	})`, server.URL+"/"+imageFilename); err != nil {
		return err
	}
	return nil
}
