// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: PrintPdfAsImageDefault,
		Desc: "Checking if the 'Print as image' option is set by default depending on the value of this policy",
		Contacts: []string{
			"chromeos-commercial-printing@google.com",
		},
		// ChromeOS > Software > Commercial (Enterprise) > Printing
		BugComponent: "b:1111614",
		SoftwareDeps: []string{"chrome"},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic",
		},
		Fixture: fixture.ChromePolicyLoggedIn,
		Data:    []string{"print_pdf_as_image_default.pdf"},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.PrintPdfAsImageDefault{}, pci.VerifiedFunctionalityUI),
		},
	})
}

// PrintPdfAsImageDefault tests the PrintPdfAsImageDefault policy.
func PrintPdfAsImageDefault(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Reserve 10 seconds for cleanup.
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	url := fmt.Sprintf("%s/print_pdf_as_image_default.pdf", server.URL)

	for _, param := range []struct {
		name                    string
		wantPrintAsImageChecked checked.Checked
		policy                  *policy.PrintPdfAsImageDefault
	}{
		{
			name:                    "enabled",
			wantPrintAsImageChecked: checked.True,
			policy:                  &policy.PrintPdfAsImageDefault{Val: true},
		},
		{
			name:                    "disabled",
			wantPrintAsImageChecked: checked.False,
			policy:                  &policy.PrintPdfAsImageDefault{Val: false},
		},
		{
			name:                    "unset",
			wantPrintAsImageChecked: checked.False,
			policy:                  &policy.PrintPdfAsImageDefault{Stat: policy.StatusUnset},
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.policy}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			conn, err := cr.NewConn(ctx, url)
			if err != nil {
				s.Fatal("Failed to open url: ", err)
			}
			defer conn.Close()
			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			kb, err := input.Keyboard(ctx)
			if err != nil {
				s.Fatal("Failed to get the keyboard: ", err)
			}
			defer kb.Close(ctx)

			// Connect to Test API to use it with the UI library.
			tconn, err := cr.TestAPIConn(ctx)
			if err != nil {
				s.Fatal("Failed to create Test API connection: ", err)
			}

			printAsImageCheckbox := nodewith.Role(role.CheckBox).Name("Print as image")

			ui := uiauto.New(tconn)
			if err := uiauto.Combine("open print preview",
				// Wait until the PDF viewer is ready and displays the content of the PDF.
				ui.WaitUntilExists(nodewith.Role(role.StaticText).Name("Hello World")),
				kb.AccelAction("Ctrl+P"),
				ui.WaitUntilExists(printAsImageCheckbox),
			)(ctx); err != nil {
				s.Fatal("Failed to open print preview: ", err)
			}
			nodeInfo, err := ui.Info(ctx, printAsImageCheckbox)
			if err != nil {
				s.Fatal("Failed to check state of 'Print as image' checkbox: ", err)
			}

			if nodeInfo.Checked != param.wantPrintAsImageChecked {
				s.Errorf("Unexpected state of the 'Print as image' checkbox: got %s; want %s", nodeInfo.Checked, param.wantPrintAsImageChecked)
			}
		})
	}
}
