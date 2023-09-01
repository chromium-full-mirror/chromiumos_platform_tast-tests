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
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/touch"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vkb"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VirtualKeyboardMultipasteSuggestion,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test multipaste suggestion functionality",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		SoftwareDeps: []string{"inputs_deps", "chrome", "google_virtual_keyboard"},
		Attr:         []string{"group:mainline", "group:input-tools", "group:hw_agnostic"},
		SearchFlags: util.SearchFlagsWithIMEAndScreenPlay(
			[]ime.InputMethod{ime.EnglishUS},
			[]string{
				"screenplay-534d932f-5fc8-4123-bc75-16a31a877ac1",
				"screenplay-a0b4e4ad-2251-43e7-a4b2-094b1786b390",
				"screenplay-a61240e5-41a1-4f35-8f47-7af4528a222f",
				"screenplay-0e482793-a91b-4a29-bda7-b5efa25af80a",
			}),
		Params: []testing.Param{
			{
				Fixture:           fixture.TabletVK,
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
				ExtraAttr:         []string{"group:input-tools-upstream"},
			},
			{
				Name:              "informational",
				Fixture:           fixture.TabletVK,
				ExtraAttr:         []string{"informational"},
				ExtraHardwareDeps: hwdep.D(pre.InputsUnstableModels, hwdep.SkipOnPlatform("puff", "fizz")),
			},
			{
				Name:              "lacros",
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
				Fixture:           fixture.LacrosTabletVK,
				ExtraAttr:         []string{"group:input-tools-upstream"},
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
			},
		},
	})
}

func VirtualKeyboardMultipasteSuggestion(ctx context.Context, s *testing.State) {
	const (
		text = "Hello world"
	)

	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn
	uc := s.FixtValue().(fixture.FixtData).UserContext

	cleanupCtx := ctx
	// Use a shortened context for test operations to reserve time for cleanup.
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	// Launch inputs test web server.
	its, err := testserver.LaunchBrowser(ctx, s.FixtValue().(fixture.FixtData).BrowserType, cr, tconn)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	defer its.CloseAll(cleanupCtx)

	// Select the input field being tested.
	inputField := testserver.TextAreaInputField
	vkbCtx := vkb.NewContext(cr, tconn)
	touchCtx, err := touch.New(ctx, tconn)
	if err != nil {
		s.Fatal("Fail to get touch screen: ", err)
	}
	defer touchCtx.Close(cleanupCtx)

	if err := ash.SetClipboard(ctx, tconn, text); err != nil {
		s.Fatal("Failed to set clipboard: ", err)
	}

	actionName := "Input text through multipaste suggestion bar"
	if err := uiauto.UserAction(
		actionName,
		uiauto.Combine(actionName,
			its.ClickFieldUntilVKShown(inputField),
			vkbCtx.TapMultipasteSuggestion(text),
			util.WaitForFieldTextToBeIgnoringCase(tconn, inputField.Finder(), text),
		),
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeInputField: string(inputField),
				useractions.AttributeFeature:    useractions.FeatureMultiPaste,
			},
		},
	)(ctx); err != nil {
		s.Fatal("Fail to paste text through multipaste suggestion: ", err)
	}
}
