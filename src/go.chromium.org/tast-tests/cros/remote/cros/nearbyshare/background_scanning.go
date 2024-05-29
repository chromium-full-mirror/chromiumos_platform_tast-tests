// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package nearbyshare

import (
	"time"

	nearbycommon "go.chromium.org/tast-tests/cros/common/cros/nearbyshare"
	"go.chromium.org/tast/core/testing"
)

// addBackgroundScanningFixtures registers fixtures for tests with background scanning enabled.
func addBackgroundScanningFixtures() {
	testing.AddFixture(&testing.Fixture{
		Name:     "nearbyShareRemoteDataUsageOfflineNoOneBackgroundScanning",
		Desc:     "Fixture for Nearby Share's CB -> CB tests. Each DUT is signed in with a real GAIA account that are in each other's contacts. Configured with 'Data Usage' set to 'Offline' and 'Visibility' set to 'No One'",
		Impl:     NewNearbyShareFixture(nearbycommon.DataUsageOffline, nearbycommon.VisibilityNoOne /*skipReceiverOnboarding=*/, true /*enabledFeatures=*/, []string{advertisementMonitoring, backgroundScanning} /*disabledFeatures=*/, []string{}),
		Contacts: []string{"chromeos-sw-engprod@google.com"},
		Vars: []string{
			"nearbyshare.cros_username",
			"nearbyshare.cros_password",
			"nearbyshare.cros2_username",
			"nearbyshare.cros2_password",
			nearbycommon.KeepStateVar,
		},
		ServiceDeps:     []string{"tast.cros.nearbyservice.NearbyShareService"},
		SetUpTimeout:    3 * time.Minute,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: resetTimeout,
		Data:            []string{"big_txt.zip", "small_jpg.zip", "small_png.zip"},
		BugComponent:    "b:1108889", // ChromeOS > Software > System Services > Cross Device
	})
	testing.AddFixture(&testing.Fixture{
		Name:     "nearbyShareRemoteDataUsageOfflineNoOneBackgroundScanningPreSetup",
		Desc:     "Fixture for Nearby Share's CB -> CB tests. Each DUT is signed in with a real GAIA account that are in each other's contacts. Nearby Share onboarding is not complete",
		Impl:     NewNearbyShareFixture(nearbycommon.DataUsageOffline, nearbycommon.VisibilityNoOne /*skipReceiverOnboarding=*/, false /*enabledFeatures=*/, []string{advertisementMonitoring, backgroundScanning} /*disabledFeatures=*/, []string{}),
		Contacts: []string{"chromeos-sw-engprod@google.com"},
		Vars: []string{
			"nearbyshare.cros_username",
			"nearbyshare.cros_password",
			"nearbyshare.cros2_username",
			"nearbyshare.cros2_password",
			nearbycommon.KeepStateVar,
		},
		ServiceDeps:     []string{"tast.cros.nearbyservice.NearbyShareService"},
		SetUpTimeout:    3 * time.Minute,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: resetTimeout,
		Data:            []string{"big_txt.zip", "small_jpg.zip", "small_png.zip"},
		BugComponent:    "b:1108889", // ChromeOS > Software > System Services > Cross Device
	})
}
