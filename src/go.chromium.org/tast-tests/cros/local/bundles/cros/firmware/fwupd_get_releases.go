// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/firmware/fwupd"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FwupdGetReleases,
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
				Name: "check_trusted_reports_flag",
				Val:  true,
			}, {
				Name: "check_releases",
				Val:  false,
			}},
	})
}

// FwupdGetReleases checks for correct number of releases for the Fake Webcam.
// It also checks if the Trusted Reports flag is set correctly
func FwupdGetReleases(ctx context.Context, s *testing.State) {
	fwupdVersion, err := fwupd.Version(ctx)
	if err != nil {
		s.Fatal("Unable to get FWUPD version: ", err)
	}
	s.Log("FWUPD version detected: ", fwupdVersion)

	var expectedReleases = []fwupd.Release{
		fwupd.Release{
			Name:       fwupd.FakeWebcamReleaseName,
			Version:    "1.2.7",
			TrustFlags: fwupd.TrustedReportsReleaseFlagBit,
		},
		fwupd.Release{
			Name:       fwupd.FakeWebcamReleaseName,
			Version:    "1.2.6",
			TrustFlags: 0,
		},
		fwupd.Release{
			Name:       fwupd.FakeWebcamReleaseName,
			Version:    "1.2.4",
			TrustFlags: fwupd.TrustedReportsReleaseFlagBit,
		},
		fwupd.Release{
			Name:       fwupd.FakeWebcamReleaseName,
			Version:    "1.2.1",
			TrustFlags: 0,
		},
	}
	checkTrustedReports := s.Param().(bool)
	var releases []*fwupd.Release
	releases, err = fwupd.ReleasesForDeviceID(ctx, fwupd.FakeWebcamDeviceID)
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
		if checkTrustedReports {
			trustedReportsFlag := release.TrustFlags & fwupd.TrustedReportsReleaseFlagBit
			if expectedReleases[i].TrustFlags != trustedReportsFlag {
				s.Fatalf("Incorrect release Trusted Reports flag found at index: %v, expected: %v, got: %v", i, expectedReleases[i].TrustFlags, trustedReportsFlag)
			}
		}
	}
}
