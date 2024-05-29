// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package nearbyshare

import (
	"time"

	nearbycommon "go.chromium.org/tast-tests/cros/common/cros/nearbyshare"
	"go.chromium.org/tast/core/testing"
)

// addBasicOfflineFixtures registers fixtures for basic Nearby Sharing tests with online data usage setting.
func addBasicOfflineFixtures() {
	testing.AddFixture(&testing.Fixture{
		Name:     "nearbyShareRemoteDataUsageOfflineAllContacts",
		Desc:     "Fixture for Nearby Share's CB -> CB tests. Each DUT is signed in with a real GAIA account that are in each other's contacts. Configured with 'Data Usage' set to 'Offline' and 'Visibility' set to 'All Contacts'",
		Impl:     NewNearbyShareFixture(nearbycommon.DataUsageOffline, nearbycommon.VisibilityAllContacts /*skipReceiverOnboarding=*/, true /*enabledFeatures=*/, []string{} /*disabledFeatures=*/, []string{}),
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
		Name:     "nearbyShareRemoteDataUsageOfflineSomeContacts",
		Desc:     "Fixture for Nearby Share's CB -> CB tests. Each DUT is signed in with a real GAIA account that are in each other's contacts. Configured with 'Data Usage' set to 'Offline' and 'Visibility' set to 'Some Contacts' with the sender selected as a contact on the receiver side",
		Impl:     NewNearbyShareFixture(nearbycommon.DataUsageOffline, nearbycommon.VisibilitySelectedContacts /*skipReceiverOnboarding=*/, true /*enabledFeatures=*/, []string{} /*disabledFeatures=*/, []string{}),
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
		Name:     "nearbyShareRemoteDataUsageOfflineNoOne",
		Desc:     "Fixture for Nearby Share's CB -> CB tests. Each DUT is signed in with a real GAIA account that are in each other's contacts. Configured with 'Data Usage' set to 'Offline' and 'Visibility' set to 'No One'",
		Impl:     NewNearbyShareFixture(nearbycommon.DataUsageOffline, nearbycommon.VisibilityNoOne /*skipReceiverOnboarding=*/, true /*enabledFeatures=*/, []string{} /*disabledFeatures=*/, []string{}),
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
		Name:     "nearbyShareRemoteDataUsageOfflineSelfShare",
		Desc:     "Fixture for Nearby Share's CB -> CB tests. Each DUT is signed in to the same real GAIA account. Configured with 'Data Usage' set to 'Offline' and 'Visibility' set to 'All Contacts'",
		Impl:     NewNearbyShareSelfShareFixture(nearbycommon.DataUsageOffline, nearbycommon.VisibilityAllContacts /*skipReceiverOnboarding=*/, true /*enabledFeatures=*/, []string{} /*disabledFeatures=*/, []string{}),
		Contacts: []string{"chromeos-sw-engprod@google.com"},
		Vars: []string{
			"nearbyshare.cros_username",
			"nearbyshare.cros_password",
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
