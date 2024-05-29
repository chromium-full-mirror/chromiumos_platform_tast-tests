// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package nearbyshare

import (
	"time"

	nearbycommon "go.chromium.org/tast-tests/cros/common/cros/nearbyshare"
	"go.chromium.org/tast/core/testing"
)

// addBasicOnlineFixtures registers fixtures for basic Nearby Sharing tests with online data usage setting.
func addBasicOnlineFixtures() {
	testing.AddFixture(&testing.Fixture{
		Name:     "nearbyShareRemoteDataUsageOnlineAllContacts",
		Desc:     "Fixture for Nearby Share's CB -> CB tests. Each DUT is signed in with a real GAIA account that are in each other's contacts. Configured with 'Data Usage' set to 'Online' and 'Visibility' set to 'All Contacts'",
		Impl:     NewNearbyShareFixture(nearbycommon.DataUsageOnline, nearbycommon.VisibilityAllContacts /*skipReceiverOnboarding=*/, true /*enabledFeatures=*/, []string{} /*disabledFeatures=*/, []string{}),
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
		Name:     "nearbyShareRemoteDataUsageOnlineSomeContacts",
		Desc:     "Fixture for Nearby Share's CB -> CB tests. Each DUT is signed in with a real GAIA account that are in each other's contacts. Configured with 'Data Usage' set to 'Online' and 'Visibility' set to 'Some Contacts' with the sender selected as a contact on the receiver side",
		Impl:     NewNearbyShareFixture(nearbycommon.DataUsageOnline, nearbycommon.VisibilitySelectedContacts /*skipReceiverOnboarding=*/, true /*enabledFeatures=*/, []string{} /*disabledFeatures=*/, []string{}),
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
		Name:     "nearbyShareRemoteDataUsageOnlineNoOne",
		Desc:     "Fixture for Nearby Share's CB -> CB tests. Each DUT is signed in with a real GAIA account that are in each other's contacts. Configured with 'Data Usage' set to 'Online' and 'Visibility' set to 'No One'",
		Impl:     NewNearbyShareFixture(nearbycommon.DataUsageOnline, nearbycommon.VisibilityNoOne /*skipReceiverOnboarding=*/, true /*enabledFeatures=*/, []string{} /*disabledFeatures=*/, []string{}),
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
		Name:     "nearbyShareRemoteDataUsageOnlineSelfShare",
		Desc:     "Fixture for Nearby Share's CB -> CB tests. Each DUT is signed in to the same real GAIA account. Configured with 'Data Usage' set to 'Online' and 'Visibility' set to 'All Contacts'",
		Impl:     NewNearbyShareSelfShareFixture(nearbycommon.DataUsageOnline, nearbycommon.VisibilityAllContacts /*skipReceiverOnboarding=*/, true /*enabledFeatures=*/, []string{} /*disabledFeatures=*/, []string{}),
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

	// Fixtures for BLE V2 tests.
	// TODO(b/333602803): Remove fixtures after BLE V2 is launched.
	testing.AddFixture(&testing.Fixture{
		Name:     "nearbyShareAllContactsBleV2",
		Desc:     "Fixture for Nearby Share's CB -> CB tests. 'Visibility' set to 'All Contacts', BLE V2 flag enabled.",
		Impl:     NewNearbyShareFixture(nearbycommon.DataUsageOnline, nearbycommon.VisibilityAllContacts /*skipReceiverOnboarding=*/, true /*enabledFeatures=*/, []string{"EnableNearbyBleV2"} /*disabledFeatures=*/, []string{}),
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
		Name:     "nearbyShareNoOneBleV2",
		Desc:     "Fixture for Nearby Share's CB -> CB tests. 'Visibility' set to 'No One', BLE V2 flag enabled.",
		Impl:     NewNearbyShareFixture(nearbycommon.DataUsageOnline, nearbycommon.VisibilityNoOne /*skipReceiverOnboarding=*/, true /*enabledFeatures=*/, []string{"EnableNearbyBleV2"} /*disabledFeatures=*/, []string{}),
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
