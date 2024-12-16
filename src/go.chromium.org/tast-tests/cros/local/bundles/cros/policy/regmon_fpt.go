// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RegmonFpt,
		Desc:         "Feature Power Test for Regmon feature",
		BugComponent: "b:1129862",
		Contacts: []string{
			"dp-chromeos-eng@google.com",
			"chiav@google.com",
		},
		SoftwareDeps: []string{"chrome", "amd64"},
		Data:         []string{"autofill_address_enabled.html"},
		Attr:         []string{"group:crosbolt", "crosbolt_nightly"},
		Timeout:      6*time.Minute + power.RecorderTimeout,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.PasswordManagerEnabled{}, pci.VerifiedValue),
		},
		// Run with feature enabled and disabled.
		Params: []testing.Param{
			{
				Name:    "enabled",
				Fixture: setup.PowerAshRegmonEnabled,
			},
			{
				Name:    "disabled",
				Fixture: setup.PowerAshRegmonDisabled,
			},
		},
	})
}

// RegmonFpt is a Feature Power Test for the Regmon feature. It repeatedly triggers D-BUS calls to regmond while
// measuring the power usage.
func RegmonFpt(ctx context.Context, s *testing.State) {
	// Example policy value, required to trigger Regmon violation reporting.
	var policyValue = policy.PasswordManagerEnabled{Val: false}
	// Recommended power time interval.
	var interval = 10 * time.Second

	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	discharge := s.FixtValue().(setup.PowerUIFixtureData).Discharge
	fdms := s.FixtValue().(setup.PowerUIFixtureData).Fdms
	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	// Update policies.
	policies := []policy.Policy{&policyValue}
	if err := policyutil.ServeAndVerify(ctx, fdms, cr, policies); err != nil {
		s.Fatal("Failed to update policies: ", err)
	}

	// Power test startup code (copied from template).
	r := power.NewRecorder(ctx, interval, s.OutDir(), s.TestName(), power.DischargeWatchdogOption(discharge))
	defer r.Close(cleanupCtx)
	if err := power.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}
	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}

	// Feature test code. Run a bunch of times to get some meaningful power results.
	for i := 0; i < 1000; i++ {
		// Open the website with the address form. This will trigger the 'autofill_query' network
		// annotation, which then triggers a D-BUS call to regmond.
		conn, err := cr.NewConn(ctx, server.URL+"/"+"autofill_address_enabled.html")
		if err != nil {
			s.Fatal("Failed to open website: ", err)
		}

		defer conn.Close()
		defer conn.CloseTarget(ctx)
	}

	// Power test finish code (copied from template).
	if err := r.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}
	if err := power.SaveScreenshot(ctx, cr); err != nil {
		s.Error("Failed to take screenshot: ", err)
	}
}
