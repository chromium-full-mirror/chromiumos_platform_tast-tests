// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/firmware/fwupd"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FwupdPasit,
		Desc: "Tests on firmware updates on a given peripheral device",
		// ChromeOS > Platform > Services > Peripherals > Firmware Update - fwupd
		BugComponent: "b:857851",
		Contacts: []string{
			"cros-fwupd-eng@google.com", // Owning team mailing list
			"rishabhagr@google.com",     // Test author
		},
		Attr:         []string{},
		SoftwareDeps: []string{"fwupd"},
		VarDeps: []string{
			"fwupd.deviceGuid",
			"fwupd.baseFwVersion",
			"fwupd.newFwVersion",
		},
		Vars: []string{
			"fwupd.activateRequired",
		},
		Fixture: "prepareFwupd",
		Timeout: 30 * time.Minute,
	})
}

func FwupdPasit(ctx context.Context, s *testing.State) {
	fwd := s.FixtValue().(*fwupd.FixtData).Fwupd

	var activateRequired = false
	if activate, ok := s.Var("fwupd.activateRequired"); ok {
		if activate != "true" && activate != "false" {
			s.Fatal("fwupd.activateRequired variable only accepts 'true' or 'false' values")
		}
		if activate == "true" {
			activateRequired = true
		}
	}

	var deviceGUID = s.RequiredVar("fwupd.deviceGuid")
	device, err := fwd.DeviceByGUID(ctx, deviceGUID)
	if err != nil {
		s.Fatalf("Failed to detect device with GUID: %q Error: %s", deviceGUID, err)
	}

	if device.Problems != 0 {
		s.Fatal("Unable to use '" + device.Name + "' due to detected problems: " + device.UpdateError)
	}

	// Set install options to allow older version and re-install
	installOptions :=
		map[string]dbus.Variant{
			"allow-reinstall": dbus.MakeVariant(true),
			"allow-older":     dbus.MakeVariant(true),
		}
	// Install base version on device
	err = fwd.InstallDeviceByVersion(ctx, device, s.RequiredVar("fwupd.baseFwVersion"), installOptions, activateRequired)
	if err != nil {
		s.Fatal("Failed to successfully install base version: ", err)
	}

	newFwVersion := s.RequiredVar("fwupd.newFwVersion")

	// Check if signed reports exist for the target firmware version
	release, err := fwd.FindReleaseByVersion(ctx, device.DeviceId, newFwVersion)
	if err != nil {
		s.Fatal("Failed to find new version: ", err)
	}
	trustedReportsFlag := release.TrustFlags & fwupd.TrustedReportsReleaseFlagBit
	if trustedReportsFlag != fwupd.TrustedReportsReleaseFlagBit {
		s.Fatalf("Did not find trusted reports for device: %q for version: %s", device.Name, newFwVersion)
	}

	// Install new version on device
	// Don't allow older or re-install. Needs to strictly be an upgrade.
	// This also makes sure that the base version provided is older than the new version.
	emptyInstallOptions := map[string]dbus.Variant{}
	err = fwd.InstallDeviceByVersion(ctx, device, newFwVersion, emptyInstallOptions, activateRequired)
	if err != nil {
		s.Fatal("Failed to successfully install new version: ", err)
	}
}
