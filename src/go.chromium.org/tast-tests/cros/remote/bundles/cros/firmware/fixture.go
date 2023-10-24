// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	cryptossh "golang.org/x/crypto/ssh"

	common "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

type fixtureParams struct {
	expectedMode        common.BootMode
	isDevModeExpected   bool
	leaveStatefulMarker bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: Fixture,
		Desc: "Verifies firmware fixtures",
		Contacts: []string{
			"chromeos-faft@google.com",
			"jbettis@chromium.org",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		SoftwareDeps: []string{"crossystem"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{{
			Name:      "normal",
			Val:       fixtureParams{expectedMode: common.BootModeNormal},
			Fixture:   fixture.NormalMode,
			ExtraAttr: []string{"group:firmware", "firmware_smoke"},
		}, {
			Name:      "dev",
			Val:       fixtureParams{expectedMode: common.BootModeDev},
			Fixture:   fixture.DevMode,
			ExtraAttr: []string{"group:firmware", "firmware_smoke"},
		}, {
			Name:      "dev_gbb",
			Val:       fixtureParams{expectedMode: common.BootModeDev},
			Fixture:   fixture.DevModeGBB,
			ExtraAttr: []string{"group:firmware", "firmware_smoke"},
		}, {
			Name:      "dev_usb",
			Val:       fixtureParams{expectedMode: common.BootModeUSBDev},
			Fixture:   fixture.USBDevModeNoServices,
			ExtraAttr: []string{"group:firmware", "firmware_smoke", "firmware_usb"},
		}, {
			Name:      "dev_usb_gbb",
			Val:       fixtureParams{expectedMode: common.BootModeUSBDev},
			Fixture:   fixture.USBDevModeGBBNoServices,
			ExtraAttr: []string{"group:firmware", "firmware_smoke", "firmware_usb"},
		}, {
			Name:              "dev_rec",
			Val:               fixtureParams{expectedMode: common.BootModeRecovery, isDevModeExpected: true},
			Fixture:           fixture.DevRecModeNoServices,
			ExtraAttr:         []string{"group:firmware", "firmware_smoke", "firmware_usb", "group:labqual", "firmware_bios", "firmware_level2"},
			ExtraRequirements: []string{"sys-fw-0021-v01", "sys-fw-0024-v01", "sys-fw-0025-v01"},
		}, {
			Name:              "rec",
			Val:               fixtureParams{expectedMode: common.BootModeRecovery},
			Fixture:           fixture.RecModeNoServices,
			ExtraAttr:         []string{"group:firmware", "firmware_smoke", "firmware_usb", "group:labqual", "firmware_bios", "firmware_level2"},
			ExtraRequirements: []string{"sys-fw-0021-v01", "sys-fw-0024-v01", "sys-fw-0025-v01"},
		}, {
			Name:    "devusb_reinstall",
			Val:     fixtureParams{expectedMode: common.BootModeUSBDev, leaveStatefulMarker: true},
			Fixture: fixture.USBDevModeWithReinstall,
			Timeout: 20 * time.Minute,
		}},
	})
}

func Fixture(ctx context.Context, s *testing.State) {
	v := s.FixtValue().(*fixture.Value)
	param := s.Param().(fixtureParams)
	wantMode := param.expectedMode

	if v.BootMode != wantMode {
		s.Errorf("Unexpected fixture boot mode: got %q, want %q", v.BootMode, wantMode)
	}

	h := v.Helper

	curr, err := h.Reporter.CurrentBootMode(ctx)
	if err != nil {
		s.Error("Could not report DUT boot mode: ", err)
	} else if curr != wantMode {
		s.Errorf("Unexpected DUT boot mode: got %q, want %q", curr, v.BootMode)
	}

	if res, err := common.GetGBBFlags(ctx, h.DUT); err != nil {
		s.Error("Failed to get GBB flags: ", err)
	} else if !common.GBBFlagsStatesEqual(v.GBBFlags, res) {
		s.Errorf("GBB flags: got %v, want %v", res.Set, v.GBBFlags)
	}

	if param.isDevModeExpected {
		if devswBoot, err := h.Reporter.CrossystemParam(ctx, reporters.CrossystemParamDevswBoot); err != nil {
			s.Fatal(err, "failed to get crossystem devsw_boot")
		} else if devswBoot != "1" {
			s.Fatalf("expected devsw_boot to be 1, got %s", devswBoot)
		}
	}
	if param.leaveStatefulMarker {
		ms, err := firmware.NewModeSwitcher(ctx, h)
		if err != nil {
			s.Fatal("Creating mode switcher: ", err)
		}
		if err := ms.RebootToMode(ctx, common.BootModeDev, firmware.AllowGBBForce); err != nil {
			s.Fatal("Error booting to internal disk: ", err)
		}
		// Leave a directory that the ChromeOS installer should delete. If this fails, then a previous test failed to clean it up.
		if err := h.DUT.Conn().CommandContext(ctx, "mkdir", "/mnt/stateful_partition/please_delete_me").Run(ssh.DumpLogOnError); err != nil {
			s.Error("Failed to create marker directory: ", err)
		}
	} else {
		err := h.DUT.Conn().CommandContext(ctx, "bash", "-c", "test -d /mnt/stateful_partition/please_delete_me").Run()
		if err == nil {
			s.Error("Marker directory unexpectedly found")
		} else if exitErr, ok := err.(*cryptossh.ExitError); !ok {
			s.Errorf("test -d returned %T; want *cryptossh.ExitError: %v", err, err)
		} else if code := exitErr.ExitStatus(); code != 1 {
			s.Errorf("test -d returned exit code %d; want 1", code)
		}
	}
}
