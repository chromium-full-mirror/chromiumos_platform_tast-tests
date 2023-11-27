// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vkb"
	"go.chromium.org/tast-tests/cros/local/inputs/fixture"
	"go.chromium.org/tast-tests/cros/local/inputs/testserver"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VirtualKeyboardHeaderBar,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks virtual keyboard header bar works properly",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{"group:mainline", "group:input-tools", "informational"},
		SoftwareDeps: []string{"inputs_deps", "chrome", "google_virtual_keyboard"},
		Timeout:      3 * time.Minute,
		Params: []testing.Param{
			{
				Name:    "tablet_dock",
				Val:     false,
				Fixture: fixture.TabletVK,
			},
			{
				Name:    "tablet_float",
				Val:     true,
				Fixture: fixture.TabletVK,
			},
			{
				Name:    "clamshell_dock",
				Val:     false,
				Fixture: fixture.ClamshellVK,
			},
			{
				Name:    "clamshell_float",
				Val:     true,
				Fixture: fixture.ClamshellVK,
			},
			{
				Name:    "tablet_dock_lacros",
				Val:     false,
				Fixture: fixture.LacrosTabletVK,
			},
			{
				Name:    "tablet_float_lacros",
				Val:     true,
				Fixture: fixture.LacrosTabletVK,
			},
			{
				Name:    "clamshell_dock_lacros",
				Val:     false,
				Fixture: fixture.LacrosClamshellVK,
			},
			{
				Name:    "clamshell_float_lacros",
				Val:     true,
				Fixture: fixture.LacrosClamshellVK,
			},
		},
	})
}

func VirtualKeyboardHeaderBar(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn
	uc := s.FixtValue().(fixture.FixtData).UserContext

	cleanupCtx := ctx
	// Use a shortened context for test operations to reserve time for cleanup.
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	its, err := testserver.LaunchBrowser(ctx, s.FixtValue().(fixture.FixtData).BrowserType, cr, tconn)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	defer its.CloseAll(cleanupCtx)

	inputField := testserver.TextAreaNoCorrectionInputField
	if err := its.ClickFieldUntilVKShown(inputField)(ctx); err != nil {
		s.Log("Failed to trigger virtual keyboard: ", err)
	}

	useFloatMode := s.Param().(bool)
	vkbCtx := vkb.NewContext(cr, tconn)
	ui := uiauto.New(tconn)

	if useFloatMode {
		if err := vkbCtx.SetFloatingMode(uc, true)(ctx); err != nil {
			s.Fatal("Failed to set VK to floating mode: ", err)
		}
	}

	// emoji buttoon is not using ACUITI to check the icon existing.
	if err := uiauto.Combine("Verify emoji button",
		vkbCtx.TapNode(nodewith.Name("emoji").First()),
		ui.WithTimeout(5*time.Second).WaitUntilExists(nodewith.Name("😀").First()),
		vkbCtx.TapNode(nodewith.Name("keyboard").First()),
	)(ctx); err != nil {
		s.Error("Failed to click emoji button: ", err)
	}

	type testData struct {
		testName         string
		nodeName         string
		verificationText string
	}

	subTests := []testData{
		{
			testName:         "verify keyboard menu button",
			nodeName:         "Open keyboard menu",
			verificationText: "Inputs and keyboards settings",
		},
		{
			testName:         "verify clipboard button",
			nodeName:         "clipboard",
			verificationText: "Access your clipboard here",
		},
		{
			testName:         "verify handwriting button",
			nodeName:         "handwriting",
			verificationText: "Your keyboard is becaming smarter",
		},
		{
			testName:         "verify voice button",
			nodeName:         "voice",
			verificationText: "Your voice input will be sent to Google servers to recognize text",
		},
	}

	ud := uidetection.NewDefault(tconn).WithScreenshotStrategy(uidetection.ImmediateScreenshot)

	for _, subTest := range subTests {
		s.Run(ctx, subTest.testName, func(ctx context.Context, s *testing.State) {
			if err := uiauto.Combine(subTest.testName,
				vkbCtx.TapNode(nodewith.Name(subTest.nodeName).First()),
				ud.WaitUntilExists(uidetection.TextBlock(strings.Split(subTest.verificationText, " "))),
			)(ctx); err != nil {
				s.Fatal(fmt.Sprintf("Failed to click %s button: ", subTest.testName), err)
			}

			vkbCtx.TapNode(nodewith.Name("keyboard").First())(ctx)
		})
	}
}
