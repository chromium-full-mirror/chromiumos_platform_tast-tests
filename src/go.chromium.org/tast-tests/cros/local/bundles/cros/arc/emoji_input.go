// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         EmojiInput,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks whether ARC apps accept the input from ChromeOS emoji picker",
		Contacts:     []string{"arc-framework+tast@google.com", "hirokisato@chromium.org"},
		// ChromeOS > Software > ARC++ > Framework > Input
		BugComponent: "b:536706",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic", "group:input-tools"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "arcBooted",
		Timeout:      5 * time.Minute,
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_p"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
	})
}

func EmojiInput(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	p := s.FixtValue().(*arc.PreData)
	cr := p.Chrome
	a := p.ARC
	d := p.UIDevice

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}

	handler := func(msg string) {
		faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)
	}
	s.AttachErrorHandlers(handler, handler)

	// Ensure Clamshell mode because we want to verify the physical keyboard usecase
	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure clamshell mode: ", err)
	}
	defer cleanup(cleanupCtx)

	const (
		apk                 = "ArcKeyboardTest.apk"
		pkg                 = "org.chromium.arc.testapp.keyboard"
		fieldID             = pkg + ":id/text"
		initText            = "hello"
		activity            = ".MainActivity"
		nodeName            = "Emoji Picker"
		inputEmoji          = "😄"
		emojiPickerShortcut = "search+shift+space"
	)

	s.Log("Installing app")
	if err := a.Install(ctx, arc.APKPath(apk)); err != nil {
		s.Fatal("Failed installing app: ", err)
	}

	act, err := arc.NewActivity(a, pkg, activity)
	if err != nil {
		s.Fatalf("Failed to create a new activity %q: %v", activity, err)
	}

	if err := act.StartWithDefaultOptions(ctx, tconn); err != nil {
		s.Fatalf("Failed to start the activity %q: %v", activity, err)
	}

	if err := d.Object(ui.ID(fieldID), ui.Text(initText)).WaitForExists(ctx, 30*time.Second); err != nil {
		s.Fatal("Failed to find field: ", err)
	}

	field := d.Object(ui.ID(fieldID))
	if err := field.Click(ctx); err != nil {
		s.Fatal("Failed to click field: ", err)
	}
	if err := field.SetText(ctx, ""); err != nil {
		s.Fatal("Failed to empty field: ", err)
	}

	if err := d.Object(ui.ID(fieldID), ui.Focused(true)).WaitForExists(ctx, 5*time.Second); err != nil {
		s.Fatal("Failed to focus on field: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)

	auto := uiauto.New(tconn)
	emojiPickerFinder := nodewith.Name(nodeName).Role(role.RootWebArea)
	emojiItem := nodewith.Name(inputEmoji).Ancestor(emojiPickerFinder).First()
	if err := uiauto.Combine("Emoji",
		kb.AccelAction(emojiPickerShortcut),
		auto.WaitUntilEnabled(emojiItem),
		auto.WaitForLocation(emojiItem),
		auto.DoDefault(emojiItem),
	)(ctx); err != nil {
		s.Fatal("Failed to open launcher and search for query: ", err)
	}
	if err := d.Object(ui.ID(fieldID)).WaitForText(ctx, inputEmoji, 5*time.Second); err != nil {
		s.Fatal("Failed to wait for text: ", err)
	}

}
