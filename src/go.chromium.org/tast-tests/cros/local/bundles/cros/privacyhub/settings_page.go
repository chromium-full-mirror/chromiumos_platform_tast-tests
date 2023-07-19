// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package privacyhub contains tests for privacy hub.
package privacyhub

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
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
	chromeFeature          string
	checkCameraControl     bool
	checkMicrophoneControl bool
	checkLocationControl   bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         SettingsPage,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that PrivacyHub settings page exists and that it contains the expected elements",
		Contacts:     []string{"chromeos-privacyhub@google.com", "janlanik@google.com"},
		// ChromeOS > Privacy > ChromeOS Privacy Feature Development.
		BugComponent: "b:1178745",
		SoftwareDeps: []string{"chrome"},
		Timeout:      5 * time.Minute,

		// Note about Params array with tests:
		// * The Test Strategy is to keep a balance between scope and a risk of getting a flaky test.
		// * If the test name is xxxxxxxxxx_golden:
		//    * it must pass on selected devices (to be non-flaky),
		//    * shall cover all functionality for upcoming milestones.
		// * Otherwise (test is not xxxxxxxxxx_golden):
		//    * test will be running on all devices (no filtering to goldeModels),
		//    * on some models it may be flaky (that's the reason of introducing golden tests),
		//    * outcome (pass/fail) will be informative due to flakiness on all models.
		Params: []testing.Param{

			{
				Name: "feature_v1_mic_cam_loc",
				Val: testSettingsParam{
					chromeFeature:          "CrosPrivacyHub",
					checkCameraControl:     true,
					checkMicrophoneControl: true,
					checkLocationControl:   true,
				},
				ExtraAttr: []string{"group:mainline", "informational"},
			},
			// Remove informational after succeeding for 10 days.
			{
				Name: "feature_v1_mic_cam_loc_golden",
				Val: testSettingsParam{
					chromeFeature:          "CrosPrivacyHub",
					checkCameraControl:     true,
					checkMicrophoneControl: true,
					checkLocationControl:   true,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(goldenModels...)),
				ExtraAttr:         []string{"group:mainline", "group:privacyhub-golden"},
			},
			// Legacy to be removed before V0 is removed from chromium.
			{
				Name: "feature_v0_mic_cam",
				Val: testSettingsParam{
					chromeFeature:          "CrosPrivacyHubV0",
					checkCameraControl:     true,
					checkMicrophoneControl: true,
					checkLocationControl:   false,
				},
				ExtraAttr: []string{"group:mainline", "informational"},
			},
			// Legacy to be removed before V0 is removed from chromium.
			{
				Name: "feature_v0_mic_cam_golden",
				Val: testSettingsParam{
					chromeFeature:          "CrosPrivacyHubV0",
					checkCameraControl:     true,
					checkMicrophoneControl: true,
					checkLocationControl:   false,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(goldenModels...)),
				ExtraAttr:         []string{"group:mainline", "group:privacyhub-golden"},
			},
			{
				Name: "feature_off",
				Val: testSettingsParam{
					chromeFeature:          "",
					checkCameraControl:     false,
					checkMicrophoneControl: false,
					checkLocationControl:   false,
				},
				ExtraAttr: []string{"group:mainline", "informational"},
			},
			{
				Name: "feature_off_golden",
				Val: testSettingsParam{
					chromeFeature:          "",
					checkCameraControl:     false,
					checkMicrophoneControl: false,
					checkLocationControl:   false,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(goldenModels...)),
				ExtraAttr:         []string{"group:mainline", "group:privacyhub-golden"},
			},
		},
	})
}

func SettingsPage(ctx context.Context, s *testing.State) {
	// Shorten deadline to leave time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	param := s.Param().(testSettingsParam)
	chromeFeature := param.chromeFeature
	featureOn := (chromeFeature != "")
	s.Log("ChromeFeature to check: ", chromeFeature)

	// Instantiate Chrome & test API.
	var cr *chrome.Chrome
	var err error
	cr, err = chrome.New(ctx, chrome.EnableFeatures(chromeFeature))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err, " with feature ", chromeFeature)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	// Trigger opening ChromeOS settings.
	settings, err := ossettings.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch OS settings: ", err)
	}
	defer settings.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	// Wait for controls element (and check existence depending on featureOn flag).
	ui := uiauto.New(tconn)
	privacyMenu := nodewith.NameStartingWith("Privacy controls")

	// If camera to be checked - we checking other Privacy Controls menu and expected buttons.
	// If camere to not be checked - it shall be no even the Privacy Controls menu.
	if featureOn {
		s.Log("Schedule a check for: Privacy Controls menu")
		if err := ui.WithTimeout(20 * time.Second).WaitUntilExists(privacyMenu)(ctx); err != nil {
			s.Fatal("Failed to find Privacy Hub in OS setting page: ", err)
		}

		// Check that the Privacy Hub section contains the required buttons.
		if err := uiauto.Combine("Verify Privacy Hub menu page",
			ui.DoDefault(privacyMenu),
			func(ctx context.Context) error {
				if param.checkCameraControl {
					s.Log("Schedule a check for: Camera access label")
					cameraAccessLabel := nodewith.NameStartingWith("Camera access").Role(role.ToggleButton)
					if err := ui.WaitUntilExists(cameraAccessLabel); err != nil {
						return err(ctx)
					}
				}
				return nil
			},
			func(ctx context.Context) error {
				if param.checkMicrophoneControl {
					s.Log("Schedule a check for: Microphone access label")
					microphoneAccessLabel := nodewith.NameStartingWith("Microphone access").Role(role.ToggleButton)
					if err := ui.WaitUntilExists(microphoneAccessLabel); err != nil {
						return err(ctx)
					}
				}
				return nil
			},
			func(ctx context.Context) error {
				if param.checkLocationControl {
					s.Log("Schedule a check for: Location access label")
					microphoneAccessLabel := nodewith.NameStartingWith("Location access").Role(role.ToggleButton)
					if err := ui.WaitUntilExists(microphoneAccessLabel); err != nil {
						return err(ctx)
					}
				}
				return nil
			},
		)(ctx); err != nil {
			s.Fatal("Failed to verify Privacy Hub menu: ", err)
		}

	} else {
		s.Log("Schedule a check for: Privacy Controls menu (absence of it)")
		// Check that the Privacy Hub section does not exist if feature flag is not explicitly set.
		// This will be removed when PrivacyHub is in production.
		if err := ui.WithTimeout(20 * time.Second).WaitUntilExists(privacyMenu)(ctx); err == nil {
			s.Fatal("Found Privacy Hub in OS setting page even though the flag is not set: ")
		}
	}
}
