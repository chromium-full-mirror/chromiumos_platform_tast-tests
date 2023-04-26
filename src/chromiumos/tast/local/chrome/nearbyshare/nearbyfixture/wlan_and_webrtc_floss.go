// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package nearbyfixture

import (
	"time"

	nearbycommon "chromiumos/tast/common/cros/nearbyshare"
	"chromiumos/tast/local/chrome/crossdevice"
	"chromiumos/tast/local/chrome/nearbyshare/nearbysnippet"
	"go.chromium.org/tast/core/testing"
)

// addFlossWebRTCAndWLANFixtures registers fixtures for tests using WebRTC and WLAN transfer mediums, and Floss enabled.
func addFlossWebRTCAndWLANFixtures() {
	testing.AddFixture(&testing.Fixture{
		Name:   "nearbyShareDataUsageOnlineNoOneWebRTCAndWLANFloss",
		Desc:   "Nearby Share enabled on CrOS and Android configured with 'Data Usage' set to 'Online' and 'Visibility' set to 'No One'. CrOS feature flags configured such that WebRTC and WLAN are eligible upgrade mediums",
		Parent: "nearbyShareGAIALoginWebRTCAndWLANFloss",
		Impl: NewNearbyShareFixture(fixtureOptions{
			crosDataUsage:              nearbycommon.DataUsageOnline,
			crosVisibility:             nearbycommon.VisibilityNoOne,
			androidDataUsage:           nearbysnippet.NearbySharingDataUsage_DATA_USAGE_ONLINE,
			androidVisibility:          nearbysnippet.NearbySharingVisibility_VISIBILITY_HIDDEN,
			crosSelectAndroidAsContact: false,
		}),
		Contacts: []string{
			"chromeos-sw-engprod@google.com",
		},
		SetUpTimeout:    3*time.Minute + crossdevice.BugReportDuration,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: postTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:   "nearbyShareDataUsageOnlineNoOneWebRTCOnlyFloss",
		Desc:   "Nearby Share enabled on CrOS and Android configured with 'Data Usage' set to 'Online' and 'Visibility' set to 'No One'. CrOS feature flags configured such that WebRTC is the only upgrade medium",
		Parent: "nearbyShareGAIALoginWebRTCOnlyFloss",
		Impl: NewNearbyShareFixture(fixtureOptions{
			crosDataUsage:              nearbycommon.DataUsageOnline,
			crosVisibility:             nearbycommon.VisibilityNoOne,
			androidDataUsage:           nearbysnippet.NearbySharingDataUsage_DATA_USAGE_ONLINE,
			androidVisibility:          nearbysnippet.NearbySharingVisibility_VISIBILITY_HIDDEN,
			crosSelectAndroidAsContact: false,
		}),
		Contacts: []string{
			"chromeos-sw-engprod@google.com",
		},
		SetUpTimeout:    3*time.Minute + crossdevice.BugReportDuration,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: postTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:   "nearbyShareDataUsageOnlineNoOneWLANOnlyFloss",
		Desc:   "Nearby Share enabled on CrOS and Android configured with 'Data Usage' set to 'Online' and 'Visibility' set to 'No One'. CrOS feature flags configured such that WLAN is the only upgrade medium",
		Parent: "nearbyShareGAIALoginWLANOnlyFloss",
		Impl: NewNearbyShareFixture(
			fixtureOptions{
				crosDataUsage:              nearbycommon.DataUsageOnline,
				crosVisibility:             nearbycommon.VisibilityNoOne,
				androidDataUsage:           nearbysnippet.NearbySharingDataUsage_DATA_USAGE_ONLINE,
				androidVisibility:          nearbysnippet.NearbySharingVisibility_VISIBILITY_HIDDEN,
				crosSelectAndroidAsContact: false,
			}),
		Contacts: []string{
			"chromeos-sw-engprod@google.com",
		},
		SetUpTimeout:    3*time.Minute + crossdevice.BugReportDuration,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: postTestTimeout,
	})
}
