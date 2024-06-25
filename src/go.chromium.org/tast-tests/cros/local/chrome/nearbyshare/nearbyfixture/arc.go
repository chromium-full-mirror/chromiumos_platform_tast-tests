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

// addARCFixtures registers fixtures for tests that initiate sharing from the ARC sharesheet.
func addARCFixtures() {
	testing.AddFixture(&testing.Fixture{
		Name:   "nearbyShareDataUsageOnlineNoOneARCEnabled",
		Desc:   "Nearby Share enabled on CrOS and Android configured with 'Data Usage' set to 'Online', 'Visibility' set to 'No One', and ARC enabled",
		Parent: "nearbyShareGAIALoginARCEnabled",
		Impl: NewNearbyShareFixture(fixtureOptions{
			crosDataUsage:              nearbycommon.DataUsageOnline,
			crosVisibility:             nearbycommon.VisibilityNoOne,
			androidDataUsage:           nearbysnippet.NearbySharingDataUsage_DATA_USAGE_ONLINE,
			androidVisibility:          nearbysnippet.NearbySharingVisibility_VISIBILITY_HIDDEN,
			crosSelectAndroidAsContact: false,
		}),
		Contacts: []string{
			"chromeos-sw-engprod@google.com",
			"arc-app-dev@google.com",
		},
		BugComponent:    "b:537221", // ChromeOS > Software > ARC++ > Framework > Chrome Integration
		SetUpTimeout:    3*time.Minute + crossdevice.BugReportDuration,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: postTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "nearbyShareDataUsageOfflineNoOneARCEnabled",
		Desc: "Nearby Share enabled on CrOS and Android configured with 'Data Usage' set to 'Offline', 'Visibility' set to 'No One', and ARC enabled",
		Impl: NewNearbyShareFixture(fixtureOptions{
			crosDataUsage:              nearbycommon.DataUsageOffline,
			crosVisibility:             nearbycommon.VisibilityNoOne,
			androidDataUsage:           nearbysnippet.NearbySharingDataUsage_DATA_USAGE_OFFLINE,
			androidVisibility:          nearbysnippet.NearbySharingVisibility_VISIBILITY_HIDDEN,
			crosSelectAndroidAsContact: false,
		}),
		Contacts: []string{
			"chromeos-sw-engprod@google.com",
			"arc-app-dev@google.com",
		},
		BugComponent:    "b:537221", // ChromeOS > Software > ARC++ > Framework > Chrome Integration
		Parent:          "nearbyShareGAIALoginARCEnabled",
		SetUpTimeout:    3*time.Minute + crossdevice.BugReportDuration,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: postTestTimeout,
	})
}
