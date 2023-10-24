// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package appsplatform

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/playbilling/dgapi2"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Dgapi2OneTime,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify it is possible to go through a one-time purchase flow in the DGAPI2 test app",
		Contacts: []string{
			"chromeos-apps-foundation-team@google.com",
			"jshikaram@chromium.org",
		},
		BugComponent: "crbug:Platform>Apps>Foundation>Stores",
		Attr:         []string{"group:hw_agnostic"}, // TODO(crbug.com/1441386) reintroduce the test once sample app is restored
		SoftwareDeps: []string{"chrome"},
		Fixture:      "playBillingDgapi2Fixture",
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
		Timeout: 5 * time.Minute,
	})
}

// Dgapi2OneTime Checks DGAPI2 test app allows to purchase a onetime sku.
func Dgapi2OneTime(ctx context.Context, s *testing.State) {
	p := s.FixtValue().(*dgapi2.FixtDgapiData)
	cr := p.Chrome
	testApp := p.TestApp

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "Dgapi2OneTime")

	// As the tested app is stateful, we might observe purchases from the previously failed test runs.
	// Need to consume them, if available, before we proceed with the test.
	if err := testApp.TryConsumeOneTime(ctx); err != nil {
		s.Fatal("Failed to consume a onetime sku: ", err)
	}

	if err := testApp.PurchaseOneTime(ctx); err != nil {
		s.Fatal("Failed to purchase a onetime sku: ", err)
	}
}
