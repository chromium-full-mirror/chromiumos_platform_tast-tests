// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/touch"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vkb"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/inputs/fixture"
	"go.chromium.org/tast-tests/cros/local/inputs/pre"
	"go.chromium.org/tast-tests/cros/local/inputs/testserver"
	"go.chromium.org/tast-tests/cros/local/inputs/util"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VirtualKeyboardMultitouch,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks typing on virtual keyboard with multiple simultaneous touches",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{"group:mainline", "group:input-tools", "group:hw_agnostic"},
		SoftwareDeps: []string{"inputs_deps", "chrome", "google_virtual_keyboard"},
		HardwareDeps: hwdep.D(pre.InputsStableModels),
		SearchFlags: util.SearchFlagsWithIMEAndScreenPlay(
			[]ime.InputMethod{ime.EnglishUSWithInternationalKeyboard},
			[]string{
				"screenplay-e6e0f446-6e57-4058-9a13-d4d84754fba0",
				"screenplay-6826842d-a2b4-4061-83b6-25b28fe746b9",
				"screenplay-8f8898d3-ecc4-454c-9b11-7365379f61ff",
				"screenplay-bbe796e7-250a-4850-8007-351fc172c62a",
			}),
		Timeout: 5 * time.Minute,
		Params: []testing.Param{
			{
				Fixture: fixture.TabletVK,
			},
			{
				Name:              "lacros",
				Fixture:           fixture.LacrosTabletVK,
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
				ExtraAttr:         []string{"informational"},
			},
		},
	})
}

func VirtualKeyboardMultitouch(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn
	uc := s.FixtValue().(fixture.FixtData).UserContext

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	its, err := testserver.LaunchBrowser(ctx, s.FixtValue().(fixture.FixtData).BrowserType, cr, tconn)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	defer its.CloseAll(cleanupCtx)

	vkbCtx := vkb.NewContext(cr, tconn)
	ui := uiauto.New(tconn)

	tsw, tcc, err := touch.NewTouchscreenAndConverter(ctx, tconn)
	if err != nil {
		s.Fatal("Fail to get touch screen: ", err)
	}
	defer tsw.Close(ctx)

	inputMethod := ime.EnglishUSWithInternationalKeyboard
	if err := inputMethod.InstallAndActivateUserAction(uc)(ctx); err != nil {
		s.Fatal("Failed to set input method: ", err)
	}
	uc.SetAttribute(useractions.AttributeInputMethod, inputMethod.Name)

	inputField := testserver.TextAreaAutoShiftInSentence

	stw, err := tsw.NewSingleTouchWriter()
	if err != nil {
		s.Fatal("Failed to get touch writer: ", err)
	}
	defer stw.Close()

	touchAndHold := func(finder *nodewith.Finder) uiauto.Action {
		return func(ctx context.Context) error {
			loc, err := ui.Location(ctx, finder)
			if err != nil {
				return errors.Wrapf(err, "failed to get the location of the node %v", finder)
			}
			x, y := tcc.ConvertLocation(loc.CenterPoint())
			if err := stw.Move(x, y); err != nil {
				return errors.Wrap(err, "failed to move the touch")
			}
			// GoBigSleepLint: Sleep is needed to mimic real user scenario.
			testing.Sleep(ctx, 50*time.Millisecond)
			return nil
		}
	}

	mousePressAndHold := func(finder *nodewith.Finder) uiauto.Action {
		return uiauto.Combine("mouse press on node center point and hold",
			ui.MouseMoveTo(finder, 10*time.Millisecond),
			mouse.Press(tconn, mouse.LeftButton),
			uiauto.Sleep(50*time.Millisecond),
		)
	}

	releaseTouch := func() uiauto.Action {
		return func(ctx context.Context) error {
			return stw.End()
		}
	}

	waitUntilLowercase := vkbCtx.WaitForKeysExist([]string{"a"})
	waitUntilUppercase := vkbCtx.WaitForKeysExist([]string{"A"})

	shiftKeyFinder := nodewith.Name("shift").Ancestor(vkb.NodeFinder.HasClass("key_pos_shift_left"))
	backspaceKeyFinder := nodewith.Name("backspace")
	zKeyFinder := vkb.KeyByNameIgnoringCase("z")
	xKeyFinder := vkb.KeyByNameIgnoringCase("x")
	vKeyFinder := vkb.KeyByNameIgnoringCase("v")

	validateAction := uiauto.NamedCombine("Verify multitouch typing on VK",
		// Basic multitouch typing.
		its.ClickFieldUntilVKShown(inputField),
		touchAndHold(zKeyFinder),
		mousePressAndHold(xKeyFinder),
		// First character should be uppercase for autoshifted text field.
		util.WaitForFieldTextToBe(tconn, inputField.Finder(), "Z"),
		releaseTouch(),
		touchAndHold(vKeyFinder),
		// VK should now be deshifted, so remaining characters should be lowercase.
		util.WaitForFieldTextToBe(tconn, inputField.Finder(), "Zx"),
		mouse.Release(tconn, mouse.LeftButton),
		releaseTouch(),
		util.WaitForFieldTextToBe(tconn, inputField.Finder(), "Zxv"),

		// Holding shift while typing.
		touchAndHold(shiftKeyFinder),
		waitUntilUppercase,
		vkbCtx.TapKeys(strings.Split("AB", "")),
		releaseTouch(),
		waitUntilLowercase,
		util.WaitForFieldTextToBe(tconn, inputField.Finder(), "ZxvAB"),

		// Holding shift while typing with caps lock on.
		vkbCtx.TapKey("caps lock"),
		waitUntilUppercase,
		touchAndHold(shiftKeyFinder),
		waitUntilLowercase,
		vkbCtx.TapKeys(strings.Split("cd", "")),
		releaseTouch(),
		waitUntilUppercase,
		util.WaitForFieldTextToBe(tconn, inputField.Finder(), "ZxvABcd"),
		vkbCtx.TapKey("caps lock"),
		waitUntilLowercase,

		vkbCtx.HideVirtualKeyboard(),
		its.Clear(inputField),
		its.ClickFieldUntilVKShown(inputField),

		// Holding backspace while typing other keys.
		vkbCtx.TapKey("A"),
		util.WaitForFieldTextToBe(tconn, inputField.Finder(), "A"),
		touchAndHold(backspaceKeyFinder),
		util.WaitForFieldTextToBe(tconn, inputField.Finder(), ""),
		vkbCtx.TapKey("B"),
		util.WaitForFieldTextToBe(tconn, inputField.Finder(), "B"),
		// Backspace should be cancelled and not delete any more characters.
		uiauto.Sleep(2*time.Second),
		releaseTouch(),
		util.WaitForFieldTextToBe(tconn, inputField.Finder(), "B"),
	)

	if err := uiauto.UserAction("Multitouch typing on virtual keyboard",
		validateAction,
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeFeature:    useractions.FeatureVKTyping,
				useractions.AttributeInputField: string(inputField),
			},
		},
	)(ctx); err != nil {
		s.Fatal("Failed to multitouch type on virtual keyboard: ", err)
	}
}
