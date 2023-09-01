// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/pre"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/util"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vkb"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VirtualKeyboardJapaneseInputMode,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks switching between Romaji and Kana mode for Japanese inputs",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{"group:mainline", "group:input-tools"},
		SoftwareDeps: []string{"inputs_deps", "chrome", "google_virtual_keyboard"},
		Fixture:      fixture.TabletVK,
		SearchFlags:  util.SearchFlagsWithIMEAndScreenPlay([]ime.InputMethod{ime.EnglishUS}, []string{"screenplay-e9418431-29e1-4ee9-bac8-ec584383c2c0"}),
		Timeout:      3 * time.Minute,
		Params: []testing.Param{
			{
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
				ExtraAttr:         []string{"group:input-tools-upstream"},
			},
			{
				Name:              "informational",
				ExtraHardwareDeps: hwdep.D(pre.InputsUnstableModels),
				ExtraAttr:         []string{"informational"},
			},
		},
	})
}

func VirtualKeyboardJapaneseInputMode(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn
	uc := s.FixtValue().(fixture.FixtData).UserContext

	vkbCtx := vkb.NewContext(cr, tconn)
	ui := uiauto.New(tconn)

	cleanupCtx := ctx
	// Use a shortened context for test operations to reserve time for cleanup.
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	im := ime.Japanese
	s.Log("Set current input method to: ", im)
	if err := im.InstallAndActivateUserAction(uc)(ctx); err != nil {
		s.Fatalf("Failed to set input method to %v: %v: ", im, err)
	}
	uc.SetAttribute(useractions.AttributeInputMethod, im.Name)

	s.Log("Opening Japanese IME options page")
	optionPage, err := cr.NewConn(ctx, "chrome-extension://jkghodnilhceideoidjikpgommlajknk/mozc_option.html")
	if err != nil {
		s.Error("Failed to open Japanese IME options page: ", err)
	}
	defer optionPage.Close()

	type inputMode struct {
		name    string
		typeKey string
		output  string
	}

	romajiInput := inputMode{
		name:    "ROMAN",
		typeKey: "a",
		output:  "あ",
	}

	kanaInput := inputMode{
		name:    "KANA",
		typeKey: "ち",
		output:  "ち",
	}

	omniboxFinder := nodewith.Role(role.TextField).Attribute("inputType", "url")
	omniboxFirstResultFinder := nodewith.ClassName("OmniboxResultView").First()
	settingPageHeaderFinder := nodewith.Role(role.Heading).Name("Japanese input settings")

	const loadNewSettingDuration = 2 * time.Second

	assertInputMode := func(ctx context.Context, mode inputMode) {
		action := uiauto.Combine(fmt.Sprintf("assert input mode is %s", mode.name),
			vkbCtx.ClickUntilVKShown(omniboxFinder),
			vkbCtx.TapKey(mode.typeKey),
			ui.RetrySilently(5, func(ctx context.Context) error {
				omniboxFirstResultInfo, err := ui.Info(ctx, omniboxFirstResultFinder)
				if err != nil {
					return errors.Wrap(err, "failed to find omnibox results")
				}
				if !strings.Contains(omniboxFirstResultInfo.Name, mode.output) {
					return errors.Errorf("unexpected output found: got %s; want %s", omniboxFirstResultInfo.Name, mode.output)
				}
				return nil
			}),
			// Press backspace button to clear omnibox result.
			vkbCtx.TapKey("backspace"),
			ui.WaitUntilGone(omniboxFirstResultFinder),
		)

		inputField := "Omnibox"
		if err := uiauto.UserAction("Japanese VK typing",
			action,
			uc,
			&useractions.UserActionCfg{
				Attributes: map[string]string{
					useractions.AttributeInputField:   inputField,
					useractions.AttributeTestScenario: fmt.Sprintf("Japanese VK typing in %q mode in %q field", mode.name, inputField),
					useractions.AttributeFeature:      useractions.FeatureIMESpecific,
				},
			},
		)(ctx); err != nil {
			s.Fatal("Failed to assert input mode: ", err)
		}
	}

	switchInputMode := func(ctx context.Context, mode inputMode) {
		action := uiauto.Combine(fmt.Sprintf("switch input mode to %q", mode.name),
			// Click page header to deactivate virtualkeyboard.
			// Note: vkb.HideVirtualKeyboard() will not trigger reloading of setting changes.
			ui.LeftClickUntil(settingPageHeaderFinder, vkbCtx.WaitUntilHidden()),
			func(ctx context.Context) error {
				return optionPage.Eval(ctx,
					fmt.Sprintf(`document.getElementById('preedit_method').value = '%s';
					document.getElementById('preedit_method').dispatchEvent(new Event('change'));`, mode.name), nil)
			},
			// No available method to check that settings being loaded. On a low-end device, it might take a second.
			// So added sleep to wait for loading.
			uiauto.Sleep(loadNewSettingDuration),
		)

		if err := uiauto.UserAction("Switch Japanese input mode",
			action,
			uc,
			&useractions.UserActionCfg{
				Tags: []useractions.ActionTag{useractions.ActionTagIMESettings},
				Attributes: map[string]string{
					useractions.AttributeTestScenario: fmt.Sprintf("Switch input mode to %q", mode.name),
					useractions.AttributeFeature:      useractions.FeatureIMESpecific,
				},
			},
		)(ctx); err != nil {
			s.Fatalf("Failed to switch input mode to %q: %v", mode.name, err)
		}
	}

	assertInputMode(ctx, romajiInput)
	switchInputMode(ctx, kanaInput)
	assertInputMode(ctx, kanaInput)
	switchInputMode(ctx, romajiInput)
	assertInputMode(ctx, romajiInput)
}
