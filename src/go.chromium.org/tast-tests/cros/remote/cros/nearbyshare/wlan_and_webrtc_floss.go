// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package nearbyshare

import (
	"time"

	nearbycommon "go.chromium.org/tast-tests/cros/common/cros/nearbyshare"
	"go.chromium.org/tast/core/testing"
)

// addFlossWebRTCAndWLANFixtures registers fixtures for tests using WebRTC and WLAN transfer mediums, with Floss enabled.
func addFlossWebRTCAndWLANFixtures() {
	testing.AddFixture(&testing.Fixture{
		Name:     "nearbyShareRemoteDataUsageOnlineNoOneWebRTCAndWLANFloss",
		Desc:     "Fixture for Nearby Share's CB -> CB tests. Each DUT is signed in with a real GAIA account that are in each other's contacts. Configured with 'Data Usage' set to 'Online' and 'Visibility' set to 'No One'. CrOS feature flags configured such that WebRTC and WLAN are eligible upgrade mediums",
		Impl:     NewNearbyShareFixture(nearbycommon.DataUsageOnline, nearbycommon.VisibilityNoOne /*skipReceiverOnboarding=*/, true /*enabledFeatures=*/, []string{webRTC, wlan, floss} /*disabledFeatures=*/, []string{}),
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
		Name:     "nearbyShareRemoteDataUsageOnlineNoOneWebRTCOnlyFloss",
		Desc:     "Fixture for Nearby Share's CB -> CB tests. Each DUT is signed in with a real GAIA account that are in each other's contacts. Configured with 'Data Usage' set to 'Online' and 'Visibility' set to 'No One'. CrOS feature flags configured such that WebRTC is the only upgrade medium",
		Impl:     NewNearbyShareFixture(nearbycommon.DataUsageOnline, nearbycommon.VisibilityNoOne /*skipReceiverOnboarding=*/, true /*enabledFeatures=*/, []string{webRTC, floss} /*disabledFeatures=*/, []string{wlan}),
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
		Name:     "nearbyShareRemoteDataUsageOnlineNoOneWLANOnlyFloss",
		Desc:     "Fixture for Nearby Share's CB -> CB tests. Each DUT is signed in with a real GAIA account that are in each other's contacts. Configured with 'Data Usage' set to 'Online' and 'Visibility' set to 'No One'. CrOS feature flags configured such that WLAN is the only upgrade medium",
		Impl:     NewNearbyShareFixture(nearbycommon.DataUsageOnline, nearbycommon.VisibilityNoOne /*skipReceiverOnboarding=*/, true /*enabledFeatures=*/, []string{wlan, floss} /*disabledFeatures=*/, []string{webRTC}),
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
