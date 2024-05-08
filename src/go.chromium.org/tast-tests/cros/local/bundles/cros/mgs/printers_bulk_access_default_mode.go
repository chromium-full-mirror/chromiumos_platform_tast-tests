// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mgs

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/mgs"
	"go.chromium.org/tast-tests/cros/local/strcmp"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PrintersBulkAccessDefaultMode,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify behavior of default PrintersBulkAccessMode policy on Managed Guest Session",
		Contacts: []string{
			"chromeos-commercial-printing@google.com",
			"mohamedaomar@google.com", // Test author
		},
		BugComponent: "b:1111614", // ChromeOS > Software > Commercial (Enterprise) > Printing
		SoftwareDeps: []string{"reboot", "chrome"},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic",
		},
		Fixture: fixture.FakeDMSEnrolled,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.PrintersBulkAccessMode{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.PrintersBulkConfiguration{}, pci.VerifiedFunctionalityJS),
		},
	})
}

func PrintersBulkAccessDefaultMode(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Launch a new MGS with default account.
	mgs, cr, err := mgs.New(
		ctx,
		fdms,
		mgs.DefaultAccount(),
		mgs.AutoLaunch(mgs.MgsAccountID),
		mgs.AddPublicAccountPolicies(mgs.MgsAccountID, []policy.Policy{
			&policy.PrintersBulkConfiguration{Val: &policy.PrintersBulkConfigurationValue{
				Url:  "https://storage.googleapis.com/chromiumos-test-assets-public/enterprise/managed_printers.json",
				Hash: "5085c7d240b89aac22ed52bfa48a046ae36e8a8389664f1d834f68ae5f3fbde7",
			}},
		}),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome on Signin screen with default MGS account: ", err)
	}
	defer func() {
		if err := mgs.Close(ctx); err != nil {
			s.Fatal("Failed close MGS: ", err)
		}
	}()

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// The default PrintersBulkAccessMode will allow all printers.
	expectedIDs := []string{"bl", "al", "other", "both"}

	defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree")

	// Retrieve Printers seen by user.
	printers := make([]map[string]string, 0)
	if err := tconn.Call(ctx, &printers, `tast.promisify(chrome.autotestPrivate.getPrinterList)`); err != nil {
		s.Fatal("Failed to evaluate JS expression and get printers: ", err)
	}

	// Get Printers IDs.
	foundIDs := make(map[string]bool)
	ids := make([]string, 0)
	for _, printer := range printers {
		if id, ok := printer["printerId"]; ok {
			foundIDs[id] = true
			ids = append(ids, id)
		} else {
			s.Fatal("Missing printerId field")
		}
	}
	if len(foundIDs) < len(printers) {
		s.Error("Received response contains duplicates")
	}

	if diff := strcmp.SameList(expectedIDs, ids); diff != "" {
		s.Error(errors.Errorf("unexpected IDs (-want +got): %v", diff))
	}
}
