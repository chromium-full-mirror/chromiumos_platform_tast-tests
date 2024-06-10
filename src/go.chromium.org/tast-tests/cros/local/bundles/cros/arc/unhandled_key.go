// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UnhandledKey,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Unhandled key in Android is sent back to Chrome",
		Contacts:     []string{"arc-framework+tast@google.com", "hirokisato@chromium.org"},
		// ChromeOS > Software > ARC++ > Framework > Input
		BugComponent: "b:536706",
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "arcBooted",
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
	})
}

func UnhandledKey(ctx context.Context, s *testing.State) {
	const (
		apk = "ArcKeyboardTest.apk"
		pkg = "org.chromium.arc.testapp.keyboard"
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
