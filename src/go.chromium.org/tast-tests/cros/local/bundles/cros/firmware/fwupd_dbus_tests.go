// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/firmware/fwupd"
	"go.chromium.org/tast/core/testing"
)

const (
	paramTrustedReports = iota
	paramReleases
	paramUpdates
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FwupdDbusTests,
		Desc: "Checks that fwupd can detect the right releases",
		// ChromeOS > Platform > Services > Peripherals > Firmware Update - fwupd
		BugComponent: "b:857851",
		Contacts: []string{
			"chromeos-fwupd@google.com", // CrOS FWUPD
			"rishabhagr@chromium.org",
		},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"fwupd"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{
			{
				Name: "trusted_reports_flag",
				Val:  paramTrustedReports,
			}, {
				Name: "releases",
				Val:  paramReleases,
			}, {
				Name: "updates",
				Val:  paramUpdates,
			},
		},
		Fixture: "prepareFwupd",
	})
}

// FwupdDbusTests checks for correct number of releases for the Fake Webcam.
// It also checks if the Trusted Reports flag is set correctly
func FwupdDbusTests(ctx context.Context, s *testing.State) {
	fwd := s.FixtValue().(*fwupd.FixtData).Fwupd

	fwupdVersion, err := fwd.Version()
	if err != nil {
		s.Fatal("Unable to get FWUPD version: ", err)
	}
	s.Log("FWUPD version detected: ", fwupdVersion)

	var expectedReleases = []fwupd.Release{
		fwupd.Release{
			Name:       fwupd.FakeWebcamReleaseName,
			Version:    "1.2.4",
			TrustFlags: fwupd.TrustedReportsReleaseFlagBit,
			RemoteId:   "lvfs",
		},
		fwupd.Release{
			Name:       fwupd.FakeWebcamReleaseName,
			Version:    "1.2.2",
			TrustFlags: 0,
			RemoteId:   "fwupd-tests",
		},
		fwupd.Release{
			Name:       fwupd.FakeWebcamReleaseName,
			Version:    "1.2.1",
			TrustFlags: 0,
			RemoteId:   "fwupd-tests",
		},
		fwupd.Release{
			Name:       fwupd.FakeWebcamReleaseName,
			Version:    "1.2.0",
			TrustFlags: fwupd.TrustedReportsReleaseFlagBit,
			RemoteId:   "fwupd-tests",
		},
	}

	// Get device using GUID
	var device *fwupd.Device
	device, err = fwd.DeviceByGUID(ctx, fwupd.FakeWebcamGUID)
	if err != nil {
		s.Fatal("Failed to detect expected device using GUID: ", err)
	}
	var testDeviceID = device.DeviceId
	s.Log("FakeWebcam device id found: ", testDeviceID)
	action := s.Param().(int)
	var releases []*fwupd.Release
	if action == paramUpdates {
		releases, err = fwd.UpdatesForDeviceID(ctx, testDeviceID)
		expectedReleases = expectedReleases[:1]
	} else {
		releases, err = fwd.ReleasesForDeviceID(ctx, testDeviceID)
	}
	if err != nil {
		s.Fatal("Failed to get releases: ", err)
	}
	if len(expectedReleases) != len(releases) {
		s.Fatalf("Incorrect number of releases found, expected: %v, got: %v", len(expectedReleases), len(releases))
	}
	for i, release := range releases {
		if expectedReleases[i].Name != release.Name {
			s.Fatalf("Incorrect release name found at index %v, expected: %s, got: %s", i, expectedReleases[i].Name, release.Name)
		}
		if expectedReleases[i].Version != release.Version {
			s.Fatalf("Incorrect release Version found at index: %v, expected: %s, got: %s", i, expectedReleases[i].Version, release.Version)
		}
		if action == paramTrustedReports {
			trustedReportsFlag := release.TrustFlags & fwupd.TrustedReportsReleaseFlagBit
			if expectedReleases[i].TrustFlags != trustedReportsFlag {
				s.Fatalf("Incorrect release Trusted Reports flag found at index: %v, expected: %v, got: %v", i, expectedReleases[i].TrustFlags, trustedReportsFlag)
			}
		}
	}
}
