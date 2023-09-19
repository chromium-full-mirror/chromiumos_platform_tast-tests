// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mgs

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/mgs"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         InstallIsolatedWebApp,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Checks if the isolated web apps can be installed for the MGS",
		Contacts: []string{
			"pwa-commercial@google.com",
			"peletskyi@google.com", // Test author
		},
		BugComponent: "b:1168200", // Chrome > Isolated Web Apps
		SoftwareDeps: []string{"reboot", "chrome"},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
		},
		Data:    []string{"iwa/telnet.swbn", "iwa/update_manifest.json"},
		Fixture: fixture.FakeDMSEnrolled,
		Timeout: time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.IsolatedWebAppInstallForceList{}, pci.VerifiedFunctionalityUI),
		},
	})
}

// InstallIsolatedWebApp installs an Isolated Web App (IWA) in a Managed Guest Session.
func InstallIsolatedWebApp(ctx context.Context, s *testing.State) {
	const port = 8080
	const updateManifestURLTemplate = "http://127.0.0.1:%v/iwa/update_manifest.json"
	updateManifestURL := fmt.Sprintf(updateManifestURLTemplate, port)

	// Start http server that will serve the update manifest and the .swbn file.
	mux := http.NewServeMux()
	fs := http.FileServer(s.DataFileSystem())
	mux.Handle("/", fs)

	server := &http.Server{Addr: fmt.Sprintf(":%v", port), Handler: mux}
	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			s.Fatal("Failed to create local server: ", err)
		}
	}()
	defer server.Shutdown(ctx)

	// The IWA name. It is specified in the manifest that is located in the signed bundle.
	const iwaName = "Telnet"

	// Web Bundle ID is the identifier of the app. It is derived from the public key.
	const webBundleID = "425tjo5yno5mk3ghxbvu5akxheq2sns5bnyehyyrqs7tuhibkaxqaaic"

	policyValue := policy.IsolatedWebAppInstallForceList{
		Val: []*policy.IsolatedWebAppInstallForceListValue{
			{
				UpdateManifestUrl: updateManifestURL,
				WebBundleId:       webBundleID,
			},
		},
	}

	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	s.Run(ctx, "Install IWA", func(ctx context.Context, s *testing.State) {
		// Launch a new MGS with default account.
		mgs, cr, err := mgs.New(
			ctx,
			fdms,
			mgs.DefaultAccount(),
			mgs.AutoLaunch(mgs.MgsAccountID),
			mgs.AddPublicAccountPolicies(mgs.MgsAccountID, []policy.Policy{&policyValue}),
		)
		if err != nil {
			s.Fatal("Failed to start Chrome on Signin screen with default MGS account: ", err)
		}
		defer func() {
			if err := mgs.Close(ctx); err != nil {
				s.Fatal("Failed close MGS: ", err)
			}
		}()
		defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_")

		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			s.Fatal("Failed to connect to test API: ", err)
		}

		ui := uiauto.New(tconn)
		iwaButton := nodewith.NameContaining(iwaName)
		if err := ui.WaitUntilExists(iwaButton)(ctx); err != nil {
			s.Fatal("Failed to wait for the IWA to be installed: ", err)
		}
	})
}
