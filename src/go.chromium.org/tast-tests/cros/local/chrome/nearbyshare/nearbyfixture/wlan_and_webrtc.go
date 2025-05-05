// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package nearbyfixture

import (
	"time"

	nearbycommon "go.chromium.org/tast-tests/cros/common/cros/nearbyshare"
	"go.chromium.org/tast-tests/cros/local/chrome/crossdevice"
	"go.chromium.org/tast-tests/cros/local/chrome/nearbyshare/nearbysnippet"
	"go.chromium.org/tast/core/testing"
)

// addWebRTCAndWLANFixtures registers fixtures for tests using WebRTC and WLAN transfer mediums.
func addWebRTCAndWLANFixtures() {
	testing.AddFixture(&testing.Fixture{
		Name:   "nearbyShareDataUsageOnlineNoOneWebRTCOnly",
		Desc:   "Nearby Share enabled on CrOS and Android configured with 'Data Usage' set to 'Online' and 'Visibility' set to 'No One'. CrOS feature flags configured such that WebRTC is the only upgrade medium",
		Parent: "nearbyShareGAIALoginWebRTCOnly",
		Impl: NewNearbyShareFixture(fixtureOptions{
			crosDataUsage:              nearbycommon.DataUsageOnline,
			crosVisibility:             nearbycommon.VisibilityNoOne,
			androidDataUsage:           nearbysnippet.NearbySharingDataUsage_DATA_USAGE_ONLINE,
			androidVisibility:          nearbysnippet.NearbySharingVisibility_VISIBILITY_HIDDEN,
			crosSelectAndroidAsContact: false,
		}),
		Contacts: []string{
			"chromeos-cross-device-eng@google.com",
			"hansenmichael@google.com",
			"joaquinmarquez@google.com",
		},
		BugComponent:    "b:1131838", // ChromeOS > Software > System Services > Cross Device > Nearby Share
		SetUpTimeout:    3*time.Minute + crossdevice.BugReportDuration,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: postTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:   "nearbyShareDataUsageOnlineNoOneWLANOnly",
		Desc:   "Nearby Share enabled on CrOS and Android configured with 'Data Usage' set to 'Online' and 'Visibility' set to 'No One'. CrOS feature flags configured such that WLAN is the only upgrade medium",
		Parent: "nearbyShareGAIALoginWLANOnly",
		Impl: NewNearbyShareFixture(
			fixtureOptions{
				crosDataUsage:              nearbycommon.DataUsageOnline,
				crosVisibility:             nearbycommon.VisibilityNoOne,
				androidDataUsage:           nearbysnippet.NearbySharingDataUsage_DATA_USAGE_ONLINE,
				androidVisibility:          nearbysnippet.NearbySharingVisibility_VISIBILITY_HIDDEN,
				crosSelectAndroidAsContact: false,
			}),
		Contacts: []string{
			"chromeos-cross-device-eng@google.com",
			"hansenmichael@google.com",
			"joaquinmarquez@google.com",
		},
		BugComponent:    "b:1131838", // ChromeOS > Software > System Services > Cross Device > Nearby Share
		SetUpTimeout:    3*time.Minute + crossdevice.BugReportDuration,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: postTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:   "nearbyShareDataUsageOnlineNoOnePhoneVisibilityEveryoneWebRTCOnly",
		Desc:   "Nearby Share enabled on CrOS and Android configured with 'Data Usage' set to 'Online'. 'Visibility' set to 'No One' on CrOS and 'Everyone' on Android. CrOS feature flags configured such that WebRTC is the only upgrade medium",
		Parent: "nearbyShareGAIALoginWebRTCOnly",
		Impl: NewNearbyShareFixture(fixtureOptions{
			crosDataUsage:              nearbycommon.DataUsageOnline,
			crosVisibility:             nearbycommon.VisibilityNoOne,
			androidDataUsage:           nearbysnippet.NearbySharingDataUsage_DATA_USAGE_ONLINE,
			androidVisibility:          nearbysnippet.NearbySharingVisibility_VISIBILITY_EVERYONE,
			crosSelectAndroidAsContact: false,
		}),
		Contacts: []string{
			"chromeos-cross-device-eng@google.com",
			"hansenmichael@google.com",
			"joaquinmarquez@google.com",
		},
		BugComponent:    "b:1131838", // ChromeOS > Software > System Services > Cross Device > Nearby Share
		SetUpTimeout:    3*time.Minute + crossdevice.BugReportDuration,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: postTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:   "nearbyShareDataUsageOnlineNoOnePhoneVisibilityEveryoneWLANOnly",
		Desc:   "Nearby Share enabled on CrOS and Android configured with 'Data Usage' set to 'Online'. 'Visibility' set to 'No One' on CrOS and 'Everyone' on Android. CrOS feature flags configured such that WLAN is the only upgrade medium",
		Parent: "nearbyShareGAIALoginWLANOnly",
		Impl: NewNearbyShareFixture(
			fixtureOptions{
				crosDataUsage:              nearbycommon.DataUsageOnline,
				crosVisibility:             nearbycommon.VisibilityNoOne,
				androidDataUsage:           nearbysnippet.NearbySharingDataUsage_DATA_USAGE_ONLINE,
				androidVisibility:          nearbysnippet.NearbySharingVisibility_VISIBILITY_EVERYONE,
				crosSelectAndroidAsContact: false,
			}),
		Contacts: []string{
			"chromeos-cross-device-eng@google.com",
			"hansenmichael@google.com",
			"joaquinmarquez@google.com",
		},
		BugComponent:    "b:1131838", // ChromeOS > Software > System Services > Cross Device > Nearby Share
		SetUpTimeout:    3*time.Minute + crossdevice.BugReportDuration,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: postTestTimeout,
	})
}
