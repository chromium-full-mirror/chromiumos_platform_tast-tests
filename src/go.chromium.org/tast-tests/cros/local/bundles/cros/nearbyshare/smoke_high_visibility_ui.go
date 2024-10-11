// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package nearbyshare

import (
	"context"
	"strconv"

	nearbycommon "go.chromium.org/tast-tests/cros/common/cros/nearbyshare"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/nearbyshare"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           SmokeHighVisibilityUI,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Checks that Nearby Share high-visibility receiving can be initiated from Quick Settings",
		Contacts: []string{
			"chromeos-cross-device-eng@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1131838",
		// Use this variable to preserve user accounts on the DUT when running locally,
		// i.e. tast run -var=keepState=true <dut> nearbyshare.SmokeHighVisibilityUI
		Vars:         []string{nearbycommon.KeepStateVar},
		Attr:         []string{"group:cross-device", "cross-device_nearbyshare"},
		SoftwareDeps: []string{"chrome"},
	})
}

// SmokeHighVisibilityUI tests that we can open the receiving UI surface from Quick Settings.
func SmokeHighVisibilityUI(ctx context.Context, s *testing.State) {
	var opts []chrome.Option
	opts = append(opts, chrome.ExtraArgs("--nearby-share-verbose-logging"))
	if val, ok := s.Var(nearbycommon.KeepStateVar); ok {
		b, err := strconv.ParseBool(val)
		if err != nil {
			s.Fatalf("Unable to convert %v var to bool: %v", nearbycommon.KeepStateVar, err)
		}
		if b {
			opts = append(opts, chrome.KeepState())
		}
	}
	cr, err := chrome.New(
		ctx,
		opts...,
	)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}

	// Set up Nearby Share on the CrOS device.
	const crosBaseName = "cros_test"
	crosDisplayName := nearbycommon.RandomDeviceName(crosBaseName)
	if err := nearbyshare.CrOSSetup(ctx, tconn, cr, nearbycommon.DataUsageOffline, nearbycommon.VisibilityAllContacts, crosDisplayName); err != nil {
		s.Fatal("Failed to set up Nearby Share: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)
	if err := nearbyshare.StartHighVisibilityMode(ctx, tconn, crosDisplayName); err != nil {
		s.Fatal("Failed to enable Nearby Share's high visibility mode: ", err)
	}
}
