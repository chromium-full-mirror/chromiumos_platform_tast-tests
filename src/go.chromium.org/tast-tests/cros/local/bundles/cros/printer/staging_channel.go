// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package printer

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/printer/uitools"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     StagingChannel,
		Desc:     "Logs in with the PPD staging channel active for manual testing",
		Contacts: []string{"cros-device-enablement@google.com", "project-bolton@google.com", "bmgordon@google.com"},
		// ChromeOS > Platform > Services > Printing
		BugComponent: "b:167231",
		Attr: []string{
			"group:paper-io",
			"paper-io_printing",
			"group:hw_agnostic",
		},
		SoftwareDeps: []string{"chrome", "cros_internal", "cups"},
		Fixture:      "virtualUsbPrinterModulesLoaded",
	})
}

func StagingChannel(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx, chrome.ExtraArgs("--printing-ppd-channel=staging"))
	if err != nil {
		s.Fatal("Failed to create chrome instance: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}
	ui := uiauto.New(tconn)
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	// Open OS Settings and navigate to the Printing page.
	if err := uitools.NavigateToPrintersSettingsPage(ctx, tconn, cr, ui); err != nil {
		s.Fatal("Failed to launch Settings page: ", err)
	}
}
