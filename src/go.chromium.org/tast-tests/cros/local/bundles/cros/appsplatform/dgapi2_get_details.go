// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package appsplatform

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/playbilling"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Dgapi2GetDetails,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify DGAPI2 test app returns expected details",
		Contacts: []string{
			"chromeos-apps-foundation-team@google.com",
		},
		BugComponent: "crbug:Platform>Apps>Foundation>Stores",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "playBillingFixture",
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
		Timeout: 5 * time.Minute,
	})
}

// Dgapi2GetDetails Checks DGAPI2 test app returns details.
func Dgapi2GetDetails(ctx context.Context, s *testing.State) {
	p := s.FixtValue().(*playbilling.FixtData)
	cr := p.Chrome
	testApp := p.TestApp

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "Dgapi2GetDetails")

	if err := testApp.Launch(ctx); err != nil {
		s.Fatal("Failed to launch Play Billing test app: ", err)
	}

	details, err := testApp.GetDetails(ctx, "book")
	if err != nil {
		s.Fatal("Failed to get details: ", err)
	}
	if !strings.Contains(details, `"Test book to purchase"`) {
		s.Fatalf(`Unexpected details, getDetails("book") = %s, want "Test book to purchase"`, details)
	}
}
