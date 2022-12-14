// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package privacyhub contains tests for privacy hub.
package privacyhub

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/ossettings"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

// High-end models, where the test should not be flaky.
// To make any change to this test, please verify everything passes on at least
// 1 golden model locally (ideally more). You can use go/crosfleet if you don't
// have access to physical DUTs.
var goldenModels = []string{
	"eldrid",
	"chronicler",
	"volta",
	"jinlon",
	"dragonair",
	"dratini",
	"gimble",
	"redrix",
	"atlas",
	"eve",
}

type testSettingsParam struct {
	chromeFeature string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         SettingsPage,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that PrivacyHub settings page exists and that it contains the expected elements",
		Contacts:     []string{"janlanik@google.com", "chromeos-privacyhub@google.com"},
		// ChromeOS > Privacy > ChromeOS Privacy Feature Development.
		BugComponent: "b:1178745",
		SoftwareDeps: []string{"chrome"},
		Timeout:      5 * time.Minute,
		Params: []testing.Param{
			{
				Name:      "feature_on",
				Val:       testSettingsParam{chromeFeature: "CrosPrivacyHub"},
				ExtraAttr: []string{"group:mainline", "informational"},
			},
			{
				Name:      "feature_off",
				Val:       testSettingsParam{chromeFeature: ""},
				ExtraAttr: []string{"group:mainline", "informational"},
			},
			{
				Name:              "feature_on_golden",
				Val:               testSettingsParam{chromeFeature: "CrosPrivacyHubV0"},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(goldenModels...)),
				ExtraAttr:         []string{"group:mainline"},
			},
			{
				Name:              "feature_off_golden",
				Val:               testSettingsParam{chromeFeature: ""},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(goldenModels...)),
				ExtraAttr:         []string{"group:mainline"},
			},
		},
	})
}

func SettingsPage(ctx context.Context, s *testing.State) {
	// Shorten deadline to leave time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	chromeFeature := s.Param().(testSettingsParam).chromeFeature
	featureOn := (chromeFeature != "")

	var cr *chrome.Chrome
	var err error
	cr, err = chrome.New(ctx, chrome.EnableFeatures(chromeFeature))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	settings, err := ossettings.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch OS settings: ", err)
	}
	defer settings.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	ui := uiauto.New(tconn)
	privacyMenu := nodewith.NameStartingWith("Privacy controls")
	if featureOn {
		if err := ui.WithTimeout(20 * time.Second).WaitUntilExists(privacyMenu)(ctx); err != nil {
			s.Fatal("Failed to find Privacy Hub in OS setting page: ", err)
		}
		// Check that the Privacy Hub section contains the required buttons.
		cameraLabel := nodewith.NameStartingWith("Camera").Role(role.ToggleButton)
		microphoneLabel := nodewith.NameStartingWith("Microphone").Role(role.ToggleButton)
		if err := uiauto.Combine("Verify privacy menu page",
			ui.DoDefault(privacyMenu),
			ui.WaitUntilExists(cameraLabel),
			ui.WaitUntilExists(microphoneLabel),
		)(ctx); err != nil {
			s.Fatal("Failed to verify privacy menu: ", err)
		}
	} else {
		// Check that the Privacy Hub section does not exist if feature flag is not explicitly set.
		// This will be removed when PrivacyHub is in production.
		if err := ui.WithTimeout(20 * time.Second).WaitUntilExists(privacyMenu)(ctx); err == nil {
			s.Fatal("Found Privacy Hub in OS setting page even though the flag is not set: ")
		}
	}
}
