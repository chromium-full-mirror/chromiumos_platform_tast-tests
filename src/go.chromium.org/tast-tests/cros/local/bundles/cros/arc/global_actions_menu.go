// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         GlobalActionsMenu,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks if showing and hiding global actions work on ARC",
		Contacts:     []string{"arc-framework+tast@google.com", "nergi@chromium.org"},
		// ChromeOS > Software > ARC++ > Framework > Input
		BugComponent: "b:536706",
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		Fixture:      "arcBooted",
		SoftwareDeps: []string{"chrome", "android_vm"},
		Timeout:      3 * time.Minute,
	})
}

func GlobalActionsMenu(ctx context.Context, s *testing.State) {
	a := s.FixtValue().(*arc.PreData).ARC
	cr := s.FixtValue().(*arc.PreData).Chrome

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}
	ui := uiauto.New(tconn)

	// Open global action menu
	if err := a.Command(ctx, "input", "keyevent", "--longpress", "KEYCODE_POWER").Run(); err != nil {
		s.Fatal("Failed to launch global actions menu via ADB command: ", err)
	}

	powerButtonMenu := nodewith.ClassName("PowerButtonMenuScreenView")
	if err := ui.WaitUntilExists(powerButtonMenu)(ctx); err != nil {
		s.Fatal("Failed to find PowerButtonMenuScreenView: ", err)
	}

	// Close global action menu
	if err := a.CloseSystemDialogs(ctx); err != nil {
		s.Fatal("Failed to close global actions menu via ADB command: ", err)
	}

	if err := ui.WaitUntilGone(powerButtonMenu)(ctx); err != nil {
		s.Fatal("PowerButtonMenuScreenView is not dismissed: ", err)
	}
}
