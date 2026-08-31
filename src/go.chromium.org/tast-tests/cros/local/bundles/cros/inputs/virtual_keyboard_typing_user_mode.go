// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/testrunner"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vkb"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/inputs/data"
	"go.chromium.org/tast-tests/cros/local/inputs/fixture"
	"go.chromium.org/tast-tests/cros/local/inputs/pre"
	"go.chromium.org/tast-tests/cros/local/inputs/testserver"
	"go.chromium.org/tast-tests/cros/local/inputs/util"
	"go.chromium.org/tast-tests/cros/local/mountns"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

var typingModeTestIMEs = []ime.InputMethod{
	ime.EnglishUS,
	ime.JapaneseWithUSKeyboard,
}
var typingModeTestMessages = []data.Message{data.TypingMessageHello}

var typingModeTestIMEInstallTimeout = 2 * time.Minute

func init() {
	testing.AddTest(&testing.Test{
		Func:         VirtualKeyboardTypingUserMode,
		Desc:         "Checks that virtual keyboard works in different user modes",
		Contacts:     []string{"essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{"group:mainline", "group:input-tools", "group:hw_agnostic", "group:release-health", "release-health_essential_inputs"},
		SearchFlags: util.SearchFlagsWithIMEAndScreenPlay(
			typingModeTestIMEs,
			[]string{
				"screenplay-4c0b7e73-7be2-4526-bfef-79be6c4af8d7",
				"screenplay-60e4337b-2ffe-4d5d-9f5a-ebad68c51779",
			}),
		SoftwareDeps: []string{"inputs_deps", "chrome", "google_virtual_keyboard"},
		HardwareDeps: hwdep.D(pre.InputsStableModels),
		Timeout:      time.Duration(len(typingModeTestIMEs)) * (time.Duration(len(typingModeTestMessages))*time.Minute + typingModeTestIMEInstallTimeout),
		Params: []testing.Param{
			{
				Name:      "guest",
				ExtraAttr: []string{"group:input-tools-upstream", "informational"},
				Fixture:   fixture.AnyVKInGuest,
			},
			{
				Name:      "incognito",
				ExtraAttr: []string{"group:input-tools-upstream"},
				Fixture:   fixture.AnyVK,
			},
		},
	})
}

func VirtualKeyboardTypingUserMode(ctx context.Context, s *testing.State) {
	// In order for the "guest_lacros" case to work correctly, we need to
	// run the test body in the user mount namespace. See b/244513681.
	if err := mountns.WithUserSessionMountNS(ctx, func(ctx context.Context) error {
		virtualKeyboardTypingUserMode(ctx, s)
		return nil
	}); err != nil {
		s.Fatal("Failed to run test in correct mount namespace: ", err)
	}
}

func virtualKeyboardTypingUserMode(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn
	uc := s.FixtValue().(fixture.FixtData).UserContext

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	its, err := testserver.LaunchBrowserInMode(ctx, cr, tconn, strings.Contains(s.TestName(), "incognito"))
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	defer its.CloseAll(cleanupCtx)

	if strings.Contains(s.TestName(), "incognito") {
		uc.SetAttribute(useractions.AttributeIncognitoMode, strconv.FormatBool(true))
	}

	inputField := testserver.TextAreaInputField
	vkbCtx := vkb.NewContext(cr, tconn)

	subtest := func(testName string, inputData data.InputData, _ ime.InputMethod) func(ctx context.Context, s *testing.State) {
		return func(ctx context.Context, s *testing.State) {
			cleanupCtx := ctx
			// Use a shortened context for test operations to reserve time for cleanup.
			ctx, shortCancel := ctxutil.Shorten(ctx, 10*time.Second)
			defer shortCancel()

			defer func(ctx context.Context) {
				outDir := filepath.Join(s.OutDir(), testName)
				faillog.DumpUITreeWithScreenshotOnError(ctx, outDir, s.HasError, cr, "ui_tree_"+testName)

				if err := vkbCtx.HideVirtualKeyboard()(ctx); err != nil {
					s.Log("Failed to hide virtual keyboard: ", err)
				}
			}(cleanupCtx)

			if err := its.ValidateInputFieldForMode(uc, inputField, util.InputWithVK, inputData, s.DataPath)(ctx); err != nil {
				s.Fatal("Failed to validate virtual keyboard input: ", err)
			}
		}
	}

	// Run defined subtest per input method and message combination.
	testrunner.RunSubtestsPerInputMethodAndMessage(ctx, uc, s, typingModeTestIMEs, typingModeTestMessages, subtest)
}
