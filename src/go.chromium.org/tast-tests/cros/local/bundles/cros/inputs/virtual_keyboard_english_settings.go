// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/pre"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/testserver"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/util"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/imesettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vkb"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VirtualKeyboardEnglishSettings,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that the input settings works in Chrome",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{"group:mainline", "group:input-tools", "group:hw_agnostic"},
		SoftwareDeps: []string{"inputs_deps", "chrome", "google_virtual_keyboard"},
		// SearchFlags:  util.IMESearchFlags([]ime.InputMethod{ime.EnglishUS}),
		SearchFlags: util.SearchFlagsWithIMEAndScreenPlay(
			[]ime.InputMethod{ime.EnglishUS},
			[]string{
				"screenplay-acdad816-f5d7-4f02-88a2-a766a1efd82a",
				"screenplay-e34be29f-5528-4b9c-8a4d-5a7d82a08c91",
			}),
		Timeout: 5 * time.Minute,
		Params: []testing.Param{
			{
				Fixture:           fixture.TabletVKRestart,
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
				ExtraAttr:         []string{"group:input-tools-upstream"},
			},
			{
				Name:              "informational",
				Fixture:           fixture.TabletVKRestart,
				ExtraAttr:         []string{"informational"},
				ExtraHardwareDeps: hwdep.D(pre.InputsUnstableModels),
			},
			{
				Name:              "lacros",
				Fixture:           fixture.LacrosTabletVKRestart,
				ExtraAttr:         []string{"group:input-tools-upstream"},
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
			},
		},
	})
}

func VirtualKeyboardEnglishSettings(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn
	uc := s.FixtValue().(fixture.FixtData).UserContext

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Revert settings to default after testing.
	defer func(ctx context.Context) {
		if err := tconn.Eval(ctx, `chrome.inputMethodPrivate.setSettings(
			"xkb:us::eng",
			{"virtualKeyboardEnableCapitalization": true,
			"virtualKeyboardAutoCorrectionLevel": 1})`, nil); err != nil {
			s.Log("Failed to revert language settings")
		}
	}(cleanupCtx)

	its, err := testserver.LaunchBrowser(ctx, s.FixtValue().(fixture.FixtData).BrowserType, cr, tconn)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	defer its.CloseAll(cleanupCtx)

	// Use text input field as testing target.
	inputField := testserver.TextInputField

	type testData struct {
		name                  string
		capitalizationEnabled bool
		keySeq                []string
		expectedText          string
		ime                   ime.InputMethod
	}

	subTests := []testData{
		{
			name:                  "capitalizationEnabled",
			capitalizationEnabled: true,
			keySeq:                strings.Split("Hello", ""),
			expectedText:          "Hello",
			ime:                   ime.EnglishUS,
		}, {
			name:                  "capitalizationDisabled",
			capitalizationEnabled: false,
			keySeq:                strings.Split("hello", ""),
			expectedText:          "hello",
			ime:                   ime.EnglishUS,
		},
	}

	for _, subTest := range subTests {
		s.Run(ctx, subTest.name, func(ctx context.Context, s *testing.State) {
			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+subTest.name)
			if err := imesettings.SetVKAutoCapitalization(uc, subTest.ime, subTest.capitalizationEnabled)(ctx); err != nil {
				s.Fatal("Failed to change IME settings: ", err)
			}

			vkbCtx := vkb.NewContext(cr, tconn)

			validateAction := uiauto.Combine("verify VK input",
				vkbCtx.WaitForDecoderEnabled(true),
				vkbCtx.HideVirtualKeyboard(),
				its.Clear(inputField),
				its.ClickFieldUntilVKShown(inputField),
				vkbCtx.TapKeys(subTest.keySeq),
				util.WaitForFieldTextToBe(tconn, inputField.Finder(), subTest.expectedText),
			)

			testScenario := "VK typing with auto-cap disabled"
			if subTest.capitalizationEnabled {
				testScenario = "VK typing with auto-cap enabled"
			}

			if err := uiauto.UserAction(
				"VK typing input",
				validateAction,
				uc,
				&useractions.UserActionCfg{
					Attributes: map[string]string{
						useractions.AttributeInputField:   string(inputField),
						useractions.AttributeTestScenario: testScenario,
						useractions.AttributeFeature:      useractions.FeatureAutoCapitalization,
					},
				},
			)(ctx); err != nil {
				s.Fatal("Failed to verify input: ", err)
			}
		})
	}
}
