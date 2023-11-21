// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package guestos

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/errors"
)

const gtkSettingsIni = `[Settings]
gtk-cursor-blink = false
`

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
