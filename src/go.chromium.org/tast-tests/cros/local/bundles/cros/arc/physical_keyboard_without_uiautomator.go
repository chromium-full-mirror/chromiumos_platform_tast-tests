// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/arc"
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
		Func:         PhysicalKeyboardWithoutUiautomator,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks physical keyboard works on Android without uiautomator",
		Contacts:     []string{"arc-framework+tast@google.com", "yhanada@chromium.org"},
		// ChromeOS > Software > ARC++ > Framework > Input
		BugComponent: "b:536706",
		SoftwareDeps: []string{"chrome", "android_vm"},
		Fixture:      "arcBootedWithoutUIAutomator",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		Timeout:      5 * time.Minute,
	})
}

// PhysicalKeyboardWithoutUiautomator is basically a copy of physicalKeyboardBasicEditingTest in physical_keyboard.go, but without using uiautomator.
func PhysicalKeyboardWithoutUiautomator(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	a := s.FixtValue().(*arc.PreData).ARC
	cr := s.FixtValue().(*arc.PreData).Chrome

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	const (
		apk      = "ArcKeyboardTest.apk"
		pkg      = "org.chromium.arc.testapp.keyboard"
		activity = ".MainActivity"
	)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(cleanupCtx)

	// Enable an a11y feature to enable ArcAccessibilityHelperService.
	// TODO(yhanada): Add a dedicated autotestPrivate API to enable it later.
	if err := a11y.SetFeatureEnabled(ctx, tconn, a11y.SelectToSpeak, true); err != nil {
		s.Fatal("Failed to enable select to speak: ", err)
	}
	defer func() {
		if err := a11y.ClearFeature(cleanupCtx, tconn, a11y.SelectToSpeak); err != nil {
			s.Fatal("Failed to disable select to speak: ", err)
		}
	}()

	s.Log("Installing app")
	if err := a.Install(ctx, arc.APKPath(apk)); err != nil {
		s.Fatal("Failed installing app: ", err)
	}

	act, err := arc.NewActivity(a, pkg, activity)
	if err != nil {
		s.Fatal("Failed to create a new activity: ", err)
	}
	if err := act.StartWithDefaultOptions(ctx, tconn); err != nil {
		s.Fatal("Failed to start the activity: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotWithTestAPIOnErrorToContextOutDir(cleanupCtx, s.HasError, tconn, "physical_keyboard")

	ui := uiauto.New(tconn)

	appFinder := nodewith.NameStartingWith("ARC Keyboard").Role(role.Application)
	finder := nodewith.ClassName("android.widget.EditText").Ancestor(appFinder).Editable().First()

	// Clear the text field.
	if err := uiauto.Combine("clear the text field",
		ui.WaitUntilExists(finder),
		ui.EnsureFocused(finder),
		kb.AccelAction("ctrl+a"),
		kb.AccelAction("Backspace"))(ctx); err != nil {
		s.Fatal("Failed to clear the textfield: ", err)
	}

	// Type 'google' into the field.
	typedText := "google"
	if err := kb.Type(ctx, typedText); err != nil {
		s.Fatal("Failed to type the text: ", err)
	}

	// Check that the typed text is set.
	if err := ui.WaitUntilExists(finder.Attribute("value", typedText))(ctx); err != nil {
		s.Fatal("Failed to type the text: ", err)
	}
}
