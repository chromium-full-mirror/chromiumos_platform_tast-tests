// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExtensionInstallForceList,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Behavior of ExtensionForceList policy",
		Contacts: []string{
			"cros-commercial-chromeapps-eng@google.com",
			"giovax@google.com", // Test owner
		},
		BugComponent: "TBA",
		Attr:         []string{"group:golden_tier", "group:hw_agnostic"},
		VarDeps:      []string{"policy.ExtensionInstallForceList.username", "policy.ExtensionInstallForceList.password"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      chrome.GAIALoginTimeout + time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlagWithName("ExtensionInstallForceList", pci.VerifiedFunctionalityUI),
			{
				Key: "feature_id",
				// Test ExtensionInstallForcelist policy (COM_FOUND_CUJ3_TASK1_WF1).
				Value: "screenplay-2b459de1-2ea4-427b-88ae-20e14d4bc62c",
			},
		},
	})
}

func ExtensionInstallForceList(ctx context.Context, s *testing.State) {
	// The user has the ExtensionInstallForceList policy set.
	username := s.RequiredVar("policy.ExtensionInstallForceList.username")
	password := s.RequiredVar("policy.ExtensionInstallForceList.password")

	cr, err := chrome.New(ctx, chrome.GAIALogin(chrome.Creds{User: username, Pass: password}), chrome.ProdPolicy())
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctx)

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	const (
		extensionID = "hoppbgdeajkagempifacalpdapphfoai"
		downloadURL = "https://chrome.google.com/webstore/detail/platformkeys-test-extensi/" + extensionID
	)

	sconn, err := cr.NewConn(ctx, downloadURL)
	if err != nil {
		s.Fatal("Failed to connect to the extension page: ", err)
	}
	defer sconn.Close()

	// If the extension is installed, the Installed button will be present which is not clickable.
	installedButton := nodewith.Role(role.Button).Name("Installed")

	if err := uiauto.New(tconn).WithTimeout(15 * time.Second).WaitUntilExists(installedButton.First())(ctx); err != nil {
		s.Fatal("Finding button node failed: ", err)
	}
}
