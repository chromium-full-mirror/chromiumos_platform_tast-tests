// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/conndiag"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ConnectivityDiagnosticsApp,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests launching the connectivity diagnostics UI",
		Contacts: []string{
			"cros-network-health-team@google.com", // network-health team
			"khegde@chromium.org",                 // test maintainer
			"stevenjb@chromium.org",               // network-health tech lead
		},
		BugComponent: "b:1166446",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline", "informational", "group:release-health", "release-health_network"},
		Timeout:      chrome.LoginTimeout + (30 * time.Second),
	})
}

// ConnectivityDiagnosticsApp ensures that the connectivity diagnostics
// application launches and displays the HTML.
func ConnectivityDiagnosticsApp(ctx context.Context, s *testing.State) {
	// Use cleanupCtx for any deferred cleanups in case of timeouts or
	// cancellations on the shortened context.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to create Chrome instance: ", err)
	}
	defer cr.Close(cleanupCtx)

	if _, err := conndiag.Launch(ctx, cr); err != nil {
		s.Fatal("Error launching Connectivity Diagnostics App: ", err)
	}
}
