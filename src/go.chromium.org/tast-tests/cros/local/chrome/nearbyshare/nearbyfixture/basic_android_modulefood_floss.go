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

// addFlossModulefoodAndroidFixtures registers fixtures for basic CrOS<->Android sharing tests with Floss enabled.
// The Android phone will be signed in with a GAIA account that is a member of the Nearby modulefood group,
// so it will receive the modulefood pre-release version of Android Nearby Share.
// These are the primary fixtures for the Nearby tests that we run across all CrOS channels.
func addFlossModulefoodAndroidFixtures() {
	// Fixtures for in-contacts sharing tests.
	testing.AddFixture(&testing.Fixture{
		Name: "nearbyShareDataUsageOfflineAllContactsFloss",
		Desc: "Nearby Share enabled on CrOS and Android configured with 'Data Usage' set to 'Offline',  'Visibility' set to 'All Contacts'",
		Impl: NewNearbyShareFixture(fixtureOptions{
			crosDataUsage:              nearbycommon.DataUsageOffline,
			crosVisibility:             nearbycommon.VisibilityAllContacts,
			androidDataUsage:           nearbysnippet.NearbySharingDataUsage_DATA_USAGE_OFFLINE,
			androidVisibility:          nearbysnippet.NearbySharingVisibility_VISIBILITY_ALL_CONTACTS,
			crosSelectAndroidAsContact: false,
		}),
		Contacts: []string{
			"chromeos-sw-engprod@google.com",
		},
		Parent:          "nearbyShareGAIALoginFloss",
		SetUpTimeout:    3*time.Minute + crossdevice.BugReportDuration,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: postTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:   "nearbyShareDataUsageOnlineAllContactsFloss",
		Desc:   "Nearby Share enabled on CrOS and Android configured with 'Data Usage' set to 'Online',  'Visibility' set to 'All Contacts'",
		Parent: "nearbyShareGAIALoginFloss",
		Impl: NewNearbyShareFixture(fixtureOptions{
			crosDataUsage:              nearbycommon.DataUsageOnline,
			crosVisibility:             nearbycommon.VisibilityAllContacts,
			androidDataUsage:           nearbysnippet.NearbySharingDataUsage_DATA_USAGE_ONLINE,
			androidVisibility:          nearbysnippet.NearbySharingVisibility_VISIBILITY_ALL_CONTACTS,
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

	// Fixtures for high-visibility sharing tests.
	testing.AddFixture(&testing.Fixture{
		Name:   "nearbyShareDataUsageOnlineNoOneFloss",
		Desc:   "Nearby Share enabled on CrOS and Android configured with 'Data Usage' set to 'Online' and 'Visibility' set to 'No One'",
		Parent: "nearbyShareGAIALoginFloss",
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
		Name: "nearbyShareDataUsageOfflineNoOneFloss",
		Desc: "Nearby Share enabled on CrOS and Android configured with 'Data Usage' set to 'Offline' and 'Visibility' set to 'No One'",
		Impl: NewNearbyShareFixture(fixtureOptions{
			crosDataUsage:              nearbycommon.DataUsageOffline,
			crosVisibility:             nearbycommon.VisibilityNoOne,
			androidDataUsage:           nearbysnippet.NearbySharingDataUsage_DATA_USAGE_OFFLINE,
			androidVisibility:          nearbysnippet.NearbySharingVisibility_VISIBILITY_HIDDEN,
			crosSelectAndroidAsContact: false,
		}),
		Contacts: []string{
			"chromeos-sw-engprod@google.com",
		},
		Parent:          "nearbyShareGAIALoginFloss",
		SetUpTimeout:    3*time.Minute + crossdevice.BugReportDuration,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: postTestTimeout,
	})

	// Fixtures for "Some contacts" visibility tests, that select which contacts to be visible to.
	testing.AddFixture(&testing.Fixture{
		Name: "nearbyShareDataUsageOfflineSomeContactsAndroidSelectedContactFloss",
		Desc: "Nearby Share enabled on CrOS and Android with 'Data Usage' set to 'Offline' on both. The Android device 'Visibility' is 'All Contacts'. The CrOS device 'Visibility' is 'Some contacts' with the Android user set as an allowed contact so it will be visible to the Android device. The CrOS device is logged in with a GAIA account which is mutual contacts with the Android GAIA account",
		Impl: NewNearbyShareFixture(fixtureOptions{
			crosDataUsage:              nearbycommon.DataUsageOffline,
			crosVisibility:             nearbycommon.VisibilitySelectedContacts,
			androidDataUsage:           nearbysnippet.NearbySharingDataUsage_DATA_USAGE_OFFLINE,
			androidVisibility:          nearbysnippet.NearbySharingVisibility_VISIBILITY_ALL_CONTACTS,
			crosSelectAndroidAsContact: true,
		}),
		Contacts: []string{
			"chromeos-sw-engprod@google.com",
		},
		Parent: "nearbyShareGAIALoginFloss",
		Vars: []string{
			customAndroidUsername,
		},
		SetUpTimeout:    3*time.Minute + crossdevice.BugReportDuration,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: postTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:   "nearbyShareDataUsageOnlineSomeContactsAndroidSelectedContactFloss",
		Desc:   "Nearby Share enabled on CrOS and Android with 'Data Usage' set to 'Online' on both. The Android device 'Visibility' is 'All Contacts'. The CrOS device 'Visibility' is 'Some contacts' with the Android user set as an allowed contact so it will be visible to the Android device. The CrOS device is logged in with a GAIA account which is mutual contacts with the Android GAIA account",
		Parent: "nearbyShareGAIALoginFloss",
		Impl: NewNearbyShareFixture(fixtureOptions{
			crosDataUsage:              nearbycommon.DataUsageOnline,
			crosVisibility:             nearbycommon.VisibilitySelectedContacts,
			androidDataUsage:           nearbysnippet.NearbySharingDataUsage_DATA_USAGE_ONLINE,
			androidVisibility:          nearbysnippet.NearbySharingVisibility_VISIBILITY_ALL_CONTACTS,
			crosSelectAndroidAsContact: true,
		}),
		Contacts: []string{
			"chromeos-sw-engprod@google.com",
		},
		Vars: []string{
			customAndroidUsername,
		},
		SetUpTimeout:    3*time.Minute + crossdevice.BugReportDuration,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: postTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:   "nearbyShareDataUsageOfflineSomeContactsAndroidNotSelectedContactFloss",
		Desc:   "Nearby Share enabled on CrOS and Android with 'Data Usage' set to 'Offline' on both. The Android device 'Visibility' is 'All Contacts'. The CrOS device 'Visibility' is 'Some contacts' with no contacts selected, so it will not be visible to the Android device. The CrOS device is logged in with a GAIA account which is mutual contacts with the Android GAIA account",
		Parent: "nearbyShareGAIALoginFloss",
		Impl: NewNearbyShareFixture(fixtureOptions{
			crosDataUsage:              nearbycommon.DataUsageOffline,
			crosVisibility:             nearbycommon.VisibilitySelectedContacts,
			androidDataUsage:           nearbysnippet.NearbySharingDataUsage_DATA_USAGE_OFFLINE,
			androidVisibility:          nearbysnippet.NearbySharingVisibility_VISIBILITY_ALL_CONTACTS,
			crosSelectAndroidAsContact: false,
		}),
		Vars: []string{
			customAndroidUsername,
		},
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
