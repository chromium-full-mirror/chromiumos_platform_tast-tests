// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package appsplatform

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/playbilling"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: CheckSkuPurchase,
		Desc: "Verify the ARC Payments overlay appears and can be navigated",
		Contacts: []string{
			"lt-web-apps-team@google.com",
		},
		BugComponent: "b:1389907",
		Attr:         []string{
			// Disabled by TORA. See: b/336143181
			// "group:mainline",
			// "informational",
			// "group:hw_agnostic",
		},
		SoftwareDeps: []string{"chrome", "gaia"},
		Fixture:      "playBillingFixture",
		Params: []testing.Param{
			{
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm"},
			},
		},
		Timeout: 4 * time.Minute,
	})
}

// CheckSkuPurchase uses the test SKU android.test.purchased to test the purchase flow.
func CheckSkuPurchase(ctx context.Context, s *testing.State) {
	p := s.FixtValue().(*playbilling.FixtData)
	cr := p.Chrome
	testApp := p.TestApp

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "CheckSkuPurchase")

	if err := testApp.Launch(ctx); err != nil {
		s.Fatal("Failed to launch Play Billing test app: ", err)
	}

	if err := testApp.OpenBillingDialog(ctx, "android_test_purchased"); err != nil {
		s.Fatal("Failed to find and click the \"Buy Test SKU\" button: ", err)
	}

	// Successful payment and required auth(if rendered) screens are rendered together.
	// Successful payment will disappear soon after being rendered.
	// Need to check for successful payment presence first, because if required auth is
	// not rendered, we wait for it. In this case by the time we check for successful
	// payment, the screen will disappear.
	if err := testApp.CheckPaymentSuccessful(ctx); err != nil {
		s.Fatal("Failed to find Payment successful: ", err)
	}

	if err := testApp.RequiredAuthConfirm(ctx); err != nil {
		s.Fatal("Failed to confirm required auth: ", err)
	}
}
