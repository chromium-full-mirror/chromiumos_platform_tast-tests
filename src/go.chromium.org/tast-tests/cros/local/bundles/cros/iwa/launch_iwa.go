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
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: LaunchIWA,
		Desc: "Checks installing and launching the Kitchen Sink IWA and sending messages via direct sockets",
		Contacts: []string{
			"iwa-team@google.com",
			"mohamedaomar@google.com",
		},
		BugComponent: "b:1168200", // Chrome > Isolated Web Apps
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier_secondary", "group:hw_agnostic"},
		Timeout:      2*chrome.LoginTimeout + 2*time.Minute,
		Fixture:      fixture.ChromePolicyLoggedIn,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.IsolatedWebAppInstallForceList{}, pci.VerifiedFunctionalityOS),
		},
	})
}

// LaunchIWA launches Kitchen Sink IWA and sends messages.
func LaunchIWA(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupContext, s.OutDir(), s.HasError, cr, "ui_tree_dump")

	const (
		kitchenSinkIWAUpdateManifestURL = "https://github.com/chromeos/iwa-sink/releases/latest/download/update.json"
		kitchenSinkIWAWebBundleID       = "aiv4bxauvcu3zvbu6r5yynoh4atkzqqaoeof5mwz54b4zfywcrjuoaacai"
		kitchenSinkIWAVersion           = "0.17.0"
	)

	pb := policy.NewBlob()
	policies := []policy.Policy{
		&policy.IsolatedWebAppInstallForceList{
			Val: []*policy.IsolatedWebAppInstallForceListValue{
				{
					UpdateManifestUrl: kitchenSinkIWAUpdateManifestURL,
					WebBundleId:       kitchenSinkIWAWebBundleID,
					PinnedVersion:     kitchenSinkIWAVersion,
				},
			},
		},
	}
	if err := pb.AddPolicies(policies); err != nil {
		s.Fatal("Failed to add policies for public account setup: ", err)
	}
	if err := policyutil.ServeBlobAndRefresh(ctx, fdms, cr, pb); err != nil {
		s.Fatal("Failed to update policies: ", err)
	}

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
	createSocketConnButton := nodewith.Name("Create new socket connection").Role(role.Button)
	sendMessageTextField := nodewith.Name("Send a message").Role(role.TextField)
	sendButton := nodewith.Name("Send").Role(role.Button)

	if err := uiauto.Combine("Launch Kitchen Sink IWA",
		// Launch Kitchen Sink IWA.
		launcher.SearchAndLaunch(tconn, kb, "Kitchen Sink IWA"),
		// Wait till the IWA is launched and the Create Socket button appears.
		ui.WithTimeout(30*time.Second).WaitUntilExists(createSocketConnButton),
	)(ctx); err != nil {
		s.Fatal("Failed to launch Kitchen Sink IWA: ", err)
	}

	if err := uiauto.Combine("Interact with Kitchen Sink IWA UI",
		// Create a new socket connection.
		ui.LeftClick(createSocketConnButton),
		ui.WaitUntilExists(sendMessageTextField.Nth(1)),
		// Send messages to the TCP Server.
		ui.LeftClickUntil(sendMessageTextField.First(), ui.Exists(sendMessageTextField.Focused())),
		kb.TypeAction("Na?"),
		ui.LeftClick(sendButton.First()),
		ui.WaitUntilExists(nodewith.NameContaining("Na?").First()),

		// Send a message from the TCP Server.
		ui.LeftClickUntil(sendMessageTextField.Nth(1), ui.Exists(sendMessageTextField.Focused())),
		kb.TypeAction("na ja!"),
		ui.LeftClick(sendButton.Nth(1)),
		ui.WaitUntilExists(nodewith.NameContaining("na ja!").First()),
	)(ctx); err != nil {
		s.Fatal("Failed to interact with the Kitchen Sink IWA: ", err)
	}
}
