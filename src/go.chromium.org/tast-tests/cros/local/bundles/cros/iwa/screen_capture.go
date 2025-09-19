// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package iwa

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/vdi/fixtures"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ScreenCapture,
		Desc: "Checks screen capture in the IWA Sink",
		Contacts: []string{
			"iwa-team@google.com",
			"mohamedaomar@google.com",
		},
		BugComponent: "b:1168200", // Chrome > Isolated Web Apps
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier_secondary", "group:hw_agnostic"},
		Timeout:      2*chrome.LoginTimeout + 2*time.Minute,
		Fixture:      fixture.FakeDMS,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.IsolatedWebAppInstallForceList{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.MultiScreenCaptureAllowedForUrls{}, pci.VerifiedFunctionalityOS),
		},
	})
}

// ScreenCapture launches Kitchen Sink IWA and captures the screens.
func ScreenCapture(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	// Login into chrome with fake creds.
	crOpts := []chrome.Option{
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
	}
	cr, err := chrome.New(ctx, crOpts...)
	if err != nil {
		s.Fatal("Failed to start chrome: ", err)
	}

	const (
		kitchenSinkIWAUpdateManifestURL = "https://github.com/chromeos/iwa-sink/releases/latest/download/update.json"
		kitchenSinkIWAWebBundleID       = "aiv4bxauvcu3zvbu6r5yynoh4atkzqqaoeof5mwz54b4zfywcrjuoaacai"
		originURL                       = "isolated-app://aiv4bxauvcu3zvbu6r5yynoh4atkzqqaoeof5mwz54b4zfywcrjuoaacai"
	)

	pb := policy.NewBlob()
	policies := []policy.Policy{
		&policy.IsolatedWebAppInstallForceList{
			Val: []*policy.IsolatedWebAppInstallForceListValue{
				{
					UpdateManifestUrl: kitchenSinkIWAUpdateManifestURL,
					WebBundleId:       kitchenSinkIWAWebBundleID,
					PinnedVersion:     "0.17.0",
				},
			},
		},
		&policy.MultiScreenCaptureAllowedForUrls{
			Val: []string{originURL},
		},
	}
	if err := pb.AddPolicies(policies); err != nil {
		s.Fatal("Failed to add policies for public account setup: ", err)
	}
	if err := policyutil.ServeBlobAndRefresh(ctx, fdms, cr, pb); err != nil {
		s.Fatal("Failed to update policies: ", err)
	}

	// Log out and back into chrome again, since dynamic refresh is intentionally disabled for MultiScreenCaptureAllowedForUrls policy.
	// Close the old chrome instance.
	if err := cr.Close(ctx); err != nil {
		s.Fatal("Failed to close chrome: ", err)
	}
	// Reopen a new chrome instance.
	cr, err = chrome.New(ctx, append(crOpts, chrome.KeepState())...)
	if err != nil {
		s.Fatal("Failed to restart chrome: ", err)
	}
	defer cr.Close(ctx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupContext, s.OutDir(), s.HasError, cr, "ui_tree_dump")

	kb, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	// Setup Chrome Test API Connection.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	ui := uiauto.New(tconn)
	screenCaptureButton := nodewith.Name("screen capture").Role(role.Link)
	captureTheScreensButton := nodewith.Name("Capture the screens").Role(role.Button)
	notificationsTray := nodewith.ClassName("NotificationCenterTray").Role(role.Button)
	recordingNotification := nodewith.NameContaining("Kitchen Sink IWA is recording your screen").ClassName("AshNotificationView")

	if err := uiauto.Combine("Launch Kitchen Sink IWA",
		// Launch Kitchen Sink IWA.
		launcher.SearchAndLaunch(tconn, kb, "Kitchen Sink IWA"),
		// Wait till the IWA is launched and the screen capture button appears.
		ui.WithTimeout(30*time.Second).WaitUntilExists(screenCaptureButton),
	)(ctx); err != nil {
		s.Fatal("Failed to launch Kitchen Sink IWA: ", err)
	}

	if err := uiauto.Combine("Capture the screens on the Kitchen Sink IWA UI",
		ui.LeftClick(screenCaptureButton),
		ui.WaitUntilExists(captureTheScreensButton),
		ui.LeftClick(captureTheScreensButton),
		ui.LeftClick(notificationsTray),
		ui.WaitUntilExists(recordingNotification),
	)(ctx); err != nil {
		s.Fatal("Failed to capture the screens on the Kitchen Sink IWA: ", err)
	}
}
