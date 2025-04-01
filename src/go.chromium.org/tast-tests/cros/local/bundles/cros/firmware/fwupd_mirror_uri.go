// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/firmware/fwupd"
	"go.chromium.org/tast/core/testing"
)

// ReleaseURI contains the release URI of the test webcam device in the
// system. Note that this is not the same as fwupd.ReleaseURI, which
// contains an extra "test" subdirectory.
const releaseURI = "https://storage.googleapis.com/chromeos-localmirror/lvfs/a92d4f433e925ea8e4a10d25dfa58e64ba1e68d07ee963605a2ccbaa2e3185aa-fakedevice124.cab"

func init() {
	testing.AddTest(&testing.Test{
		Func: FwupdMirrorURI,
		Desc: "Test that fwupd uses the CrOS LVFS mirror in release URIs",
		// ChromeOS > Platform > Services > Peripherals > Firmware Update - fwupd
		BugComponent: "b:857851",
		Contacts: []string{
			"chromeos-fwupd@google.com",
			"nicholasbishop@google.com",
		},
		Attr:         []string{"group:mainline", "informational", "group:fwupd"},
		SoftwareDeps: []string{"fwupd"},
		Fixture:      "prepareFwupd",
	})
}

func FwupdMirrorURI(ctx context.Context, s *testing.State) {
	fwd := s.FixtValue().(*fwupd.FixtData).Fwupd

	device, err := fwd.DeviceByGUID(ctx, fwupd.FakeWebcamGUID)
	if err != nil {
		s.Fatal("Failed to detect expected device using GUID: ", err)
	}

	releases, err := fwd.ReleasesForDeviceID(ctx, device.DeviceId)
	if err != nil {
		s.Fatal("Failed to get releases for the device: ", err)
	}

	release := releases[0]
	location := release.Locations[0]

	if location != releaseURI {
		s.Fatalf("Unexpected URI: got %s, want %s", location, releaseURI)
	}
}
