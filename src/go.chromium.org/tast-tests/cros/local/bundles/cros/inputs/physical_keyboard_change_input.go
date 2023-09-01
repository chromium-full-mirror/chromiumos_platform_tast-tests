// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/pre"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/testserver"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/util"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PhysicalKeyboardChangeInput,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that changing input method in different ways on physical keyboard",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{"group:mainline", "group:input-tools", "group:hw_agnostic"},
		SoftwareDeps: []string{"inputs_deps", "chrome", "chrome_internal"},
		SearchFlags: util.SearchFlagsWithIMEAndScreenPlay(
			[]ime.InputMethod{ime.DefaultInputMethod, ime.EnglishUK, ime.ChinesePinyin},
			[]string{
				"screenplay-36dacf68-4559-473d-a936-7a44acc579b8",
				"screenplay-1a03f485-f795-43dd-8822-e55da1c368fc",
			}),
		Timeout: 3 * time.Minute,
		Params: []testing.Param{
			{
				Fixture:           fixture.ClamshellNonVK,
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
				ExtraAttr:         []string{"group:input-tools-upstream"},
			},
			{
				Name:              "informational",
				Fixture:           fixture.ClamshellNonVK,
				ExtraAttr:         []string{"informational"},
				ExtraHardwareDeps: hwdep.D(pre.InputsUnstableModels),
			},
			{
				Name:              "lacros",
				Fixture:           fixture.LacrosClamshellNonVK,
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
				ExtraAttr:         []string{"group:input-tools-upstream"},
			},
		},
	})
}

func PhysicalKeyboardChangeInput(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn
	uc := s.FixtValue().(fixture.FixtData).UserContext

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	newInputMethods := []ime.InputMethod{ime.EnglishUK, ime.ChinesePinyin}

	for _, newInputMethod := range newInputMethods {
		if err := newInputMethod.Install(tconn)(ctx); err != nil {
			s.Fatalf("Failed to install new input method %q: %v", newInputMethod, err)
		}
	}

	its, err := testserver.LaunchBrowser(ctx, s.FixtValue().(fixture.FixtData).BrowserType, cr, tconn)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	defer its.CloseAll(cleanupCtx)

	// Retrieve all installed input methods via Chrome API.
	// Then parse it into ime.InputMethod struct and append to installedInputMethods list.
	var installedInputMethods []*ime.InputMethod
	if installedBindingInputMethod, err := ime.InstalledInputMethods(ctx, tconn); err != nil {
		s.Fatal("Failed to get installed input methods: ", err)
	} else {
		for _, bindingInputMethod := range installedBindingInputMethod {
			if im, err := ime.FindInputMethodByFullyQualifiedIMEID(ctx, tconn, bindingInputMethod.ID); err != nil {
				s.Fatalf("Failed to recognize input method id %q: %v", bindingInputMethod.ID, err)
			} else {
				installedInputMethods = append(installedInputMethods, im)
			}
		}
	}

	currentInputMethod := ime.EnglishUS
	var lastActiveInputMethod ime.InputMethod

	// expectedNextInputMethod finds the expected next input method in order.
	findNextInputMethod := func() ime.InputMethod {
		for index, im := range installedInputMethods {
			if im.Equal(currentInputMethod) {
				return *installedInputMethods[(index+1)%len(installedInputMethods)]
			}
		}
		s.Fatalf("Failed to find the next input method of %q", currentInputMethod)
		return ime.DefaultInputMethod
	}

	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer keyboard.Close(ctx)

	// TODO(b/196771467) Replace this function with im.WaitUntilActivated() once adding typing validation.
	waitUntilCurrentInputMethod := func(im ime.InputMethod) uiauto.Action {
		return func(ctx context.Context) error {
			fullyQualifiedIMEID, err := im.FullyQualifiedIMEID(ctx, tconn)
			if err != nil {
				return errors.Wrapf(err, "failed to get fully qualified IME ID of %q", im)
			}
			return ime.WaitForInputMethodMatches(ctx, tconn, fullyQualifiedIMEID, 20*time.Second)
		}
	}

	switchToNextInputMethod := func(ctx context.Context) error {
		nextInputMethod := findNextInputMethod()
		if err := uiauto.Combine("switch to next input method in order",
			keyboard.AccelAction("Ctrl+Shift+Space"),
			waitUntilCurrentInputMethod(nextInputMethod),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to switch to next input method")
		}
		lastActiveInputMethod = currentInputMethod
		currentInputMethod = nextInputMethod
		return nil
	}

	switchToNextInputMethodUserAction := uiauto.UserAction(
		"Switch input method with shortcut Ctrl+Shift+Space",
		switchToNextInputMethod,
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeFeature: useractions.FeatureIMEManagement,
			},
		},
	)

	switchToLastActiveInputMethod := func(ctx context.Context) error {
		if err := uiauto.Combine("switch to recent input method",
			keyboard.AccelAction("Ctrl+Space"),
			waitUntilCurrentInputMethod(lastActiveInputMethod),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to switch to recent input method")
		}
		currentInputMethod, lastActiveInputMethod = lastActiveInputMethod, currentInputMethod
		return nil
	}

	switchToLastActiveInputMethodUserAction := uiauto.UserAction(
		"Switch input method with shortcut Ctrl+Space",
		switchToLastActiveInputMethod,
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeFeature: useractions.FeatureIMEManagement,
			},
		},
	)

	// TODO(b/196771467) Validate typing after switching IME.
	if err := uiauto.Combine("switch IME in different ways",
		switchToNextInputMethodUserAction,
		switchToLastActiveInputMethodUserAction,
		switchToNextInputMethodUserAction,
		switchToNextInputMethodUserAction,
		switchToLastActiveInputMethodUserAction,
	)(ctx); err != nil {
		s.Fatal("Failed to switch input method: ", err)
	}
}
