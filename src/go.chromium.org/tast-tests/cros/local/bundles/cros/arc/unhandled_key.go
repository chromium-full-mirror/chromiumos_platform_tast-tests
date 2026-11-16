// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	androidui "go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     UnhandledKey,
		Desc:     "Unhandled key in Android is sent back to Chrome",
		Contacts: []string{"arc-framework+tast@google.com", "hirokisato@chromium.org"},
		// ChromeOS > Software > ARC++ > Framework > Input
		BugComponent: "b:536706",
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "arcBooted",
		Params: []testing.Param{
			{
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraHardwareDeps: arc.ArcAppHwDeps,
			}},
	})
}

func UnhandledKey(ctx context.Context, s *testing.State) {
	const (
		apk     = "ArcKeyboardTest.apk"
		pkg     = "org.chromium.arc.testapp.keyboard"
		fieldID = pkg + ":id/text"
	)

	cr := s.FixtValue().(*arc.PreData).Chrome

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	a := s.FixtValue().(*arc.PreData).ARC

	s.Log("Starting app")

	if err := a.Install(ctx, arc.APKPath(apk)); err != nil {
		s.Fatal("Failed installing app: ", err)
	}

	act, err := arc.NewActivity(a, pkg, ".MainActivity")
	if err != nil {
		s.Fatal("Failed to create new activity: ", err)
	}
	defer act.Close(ctx)

	if err := act.StartWithDefaultOptions(ctx, tconn); err != nil {
		s.Fatal("Failed to start app: ", err)
	}

	// StartWithDefaultOptions returns once the Ash task window is visible, while
	// Android may still be showing the splash screen and rendering MainActivity's
	// initial frame. Wait for the activity's text field to appear, ensure it
	// holds Android focus (clicking if Android started in touch mode), and wait
	// for UI events to settle so key acks are not delayed past Exo's
	// pending-key-ack timeout.
	d := s.FixtValue().(*arc.PreData).UIDevice
	field := d.Object(androidui.ID(fieldID))
	if err := field.WaitForExists(ctx, 15*time.Second); err != nil {
		s.Fatal("Failed to wait for the text field to appear: ", err)
	}
	focusedField := d.Object(androidui.ID(fieldID), androidui.Focused(true))
	if err := focusedField.Exists(ctx); err != nil {
		if err := field.Click(ctx); err != nil {
			s.Fatal("Failed to click the text field: ", err)
		}
		if err := focusedField.WaitForExists(ctx, 10*time.Second); err != nil {
			s.Fatal("Failed to wait for the text field to be focused: ", err)
		}
	}
	if err := d.WaitForIdle(ctx, 10*time.Second); err != nil {
		s.Fatal("Failed to wait for Android UI to become idle: ", err)
	}

	// Verify the Ash task window holds focus right before sending Ctrl+N.
	if err := ash.WaitForCondition(ctx, tconn, func(window *ash.Window) bool {
		return window.ARCPackageName == pkg &&
			window.IsVisible && window.HasFocus && window.IsActive
	}, &testing.PollOptions{Timeout: 15 * time.Second}); err != nil {
		s.Fatal("Failed to wait for the app to be ready: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)

	if err := kb.Accel(ctx, "Ctrl+N"); err != nil {
		s.Fatal("Failed to inject Ctrl + N: ", err)
	}

	ui := uiauto.New(tconn)
	if err := ui.WaitUntilExists(
		nodewith.Role(role.Window).NameContaining("Chrome - New Tab").Visible(),
	)(ctx); err != nil {
		s.Fatal("New Chrome window didn't appear: ", err)
	}
}
