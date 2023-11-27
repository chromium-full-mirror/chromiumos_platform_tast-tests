// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vkb"
	"go.chromium.org/tast-tests/cros/local/inputs/fixture"
	"go.chromium.org/tast-tests/cros/local/inputs/pre"
	"go.chromium.org/tast-tests/cros/local/inputs/testserver"
	"go.chromium.org/tast-tests/cros/local/inputs/util"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const password = "password"

var imes = []ime.InputMethod{
	ime.EnglishUS,
	ime.FrenchFrance,
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         VirtualKeyboardSuggestionPasswordField,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that virtual keyboard shows digits as candidates when user typing in password field",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{"group:mainline", "group:input-tools", "informational"},
		SearchFlags:  util.SearchFlagsWithIMEAndScreenPlay(imes, []string{"screenplay-56ac7c03-9ec5-41a8-9d0c-3431f739c3c2"}),
		SoftwareDeps: []string{"inputs_deps", "chrome", "google_virtual_keyboard"},
		HardwareDeps: hwdep.D(pre.InputsStableModels),
		Timeout:      2 * time.Minute,
		Params: []testing.Param{
			{
				Fixture: fixture.TabletVK,
			},
			{
				Name:    "lacros",
				Fixture: fixture.LacrosTabletVK,
			},
		},
	})
}

func VirtualKeyboardSuggestionPasswordField(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn
	uc := s.FixtValue().(fixture.FixtData).UserContext

	cleanupCtx := ctx
	// Use a shortened context for test operations to reserve time for cleanup.
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	its, err := testserver.LaunchBrowser(ctx, s.FixtValue().(fixture.FixtData).BrowserType, cr, tconn)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	defer its.CloseAll(cleanupCtx)

	vkbCtx := vkb.NewContext(cr, tconn)
	ui := uiauto.New(tconn)

	for _, ime := range imes {
		s.Run(ctx, ime.Name, func(ctx context.Context, s *testing.State) {
			if err := ime.InstallAndActivateUserAction(uc)(ctx); err != nil {
				s.Fatal("Fail to set input method: ", err)
			}

			if err := uiauto.NamedCombine("Input some letters into password field and click makeText button",
				its.CleanFieldAndTriggerVK(testserver.PasswordInputField),
				vkbCtx.TapKeysIgnoringCase(strings.Split(password, "")),
				its.ClickButton(testserver.MakeTextButton),
				its.ClickFieldUntilVKShown(testserver.PasswordInputField),
			)(ctx); err != nil {
				s.Fatal("Failed to trigger vk after click makeText button: ", err)
			}

			if err := ui.WaitUntilExists(nodewith.Name("word suggestion 1"))(ctx); err != nil {
				s.Fatal("Failed to confirm digits bar shown: ", err)
			}
		})
	}
}
