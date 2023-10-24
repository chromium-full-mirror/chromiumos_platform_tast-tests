// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"strconv"

	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: TPMKernelVersion,
		Desc: "Check firmware and kernel version stored in TPM",
		Contacts: []string{
			"chromeos-faft@google.com",
			"tij@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_level3"},
		Requirements: []string{"sys-fw-0021-v01", "sys-fw-0024-v01", "sys-fw-0025-v01"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{
			{
				Name:    "normal",
				Val:     fixture.NormalMode,
				Fixture: fixture.NormalMode,
			},
			{
				Name:    "dev",
				Val:     fixture.DevModeGBB,
				Fixture: fixture.DevModeGBB,
			},
			{
				Name:      "dev_usb",
				Val:       fixture.USBDevModeGBBNoServices,
				Fixture:   fixture.USBDevModeGBBNoServices,
				ExtraAttr: []string{"firmware_usb"},
			},
		},
	})
}

// TPMKernelVersion verifies that kernel and firmware version stored
// in TPM are read properly and not containing invalid values.
func TPMKernelVersion(ctx context.Context, s *testing.State) {

	h := s.FixtValue().(*fixture.Value).Helper
	r := reporters.New(h.DUT)

	crossystemTPMVer := func(ctx context.Context) (fwVersion, kernVersion int64) {
		fwVersionStr, err := r.CrossystemParam(ctx, reporters.CrossystemParamTpmFwVer)
		if err != nil {
			s.Fatalf("Failed to get crossystem %v value: %v", reporters.CrossystemParamTpmFwVer, err)
		}

		kernVersionStr, err := r.CrossystemParam(ctx, reporters.CrossystemParamTpmKernelVer)
		if err != nil {
			s.Fatalf("Failed to get crossystem %v value: %v", reporters.CrossystemParamTpmKernelVer, err)
		}

		fwVersion, err = strconv.ParseInt(fwVersionStr, 0, 64)
		if err != nil {
			s.Fatal("Failed to parse firmware version stored in TPM as HEX string: ", err)
		}

		kernVersion, err = strconv.ParseInt(kernVersionStr, 0, 64)
		if err != nil {
			s.Fatal("Failed to parse kernel version stored in TPM as string: ", err)
		}

		return fwVersion, kernVersion
	}

	switch s.Param().(string) {
	case fixture.NormalMode, fixture.DevModeGBB:
		const expVersion int64 = 0x00010001

		fwVersion, kernVersion := crossystemTPMVer(ctx)

		s.Logf("Firmware version in TPM: 0x%08x", fwVersion)
		s.Logf("Kernel version in TPM: 0x%08x", kernVersion)

		if fwVersion != expVersion {
			s.Fatalf("Invalid tpm_fwver version found in crossystem, expected %v, got %v", expVersion, fwVersion)
		}
		if kernVersion != expVersion {
			s.Fatalf("Invalid tpm_kernver version found in crossystem, expected %v, got %v", expVersion, kernVersion)
		}

	case fixture.USBDevModeGBBNoServices:
		kernKeyVfy, err := r.CrossystemParam(ctx, reporters.CrossystemParamKernkeyVfy)
		if err != nil {
			s.Fatalf("Failed to get crossystem %v value: %v", reporters.CrossystemParamKernkeyVfy, err)
		}
		devBootUsb, err := r.CrossystemParam(ctx, reporters.CrossystemParamDevBootUsb)
		if err != nil {
			s.Fatalf("Failed to get crossystem %v value: %v", reporters.CrossystemParamDevBootUsb, err)
		}

		s.Logf("crossystem kernkey_vfy = %q, dev_boot_usb = %q", kernKeyVfy, devBootUsb)

		if kernKeyVfy != "hash" {
			s.Fatalf("Expected kernkey_vfy to be 'hash', got %q instead", kernKeyVfy)
		}
		if devBootUsb != "1" {
			s.Fatalf("Expected dev_boot_usb to be '1', got %q instead", devBootUsb)
		}

		fwVersion, kernVersion := crossystemTPMVer(ctx)

		s.Logf("Firmware version in TPM: 0x%08x", fwVersion)
		s.Logf("Kernel version in TPM: 0x%08x", kernVersion)

		if fwVersion == 0xFFFFFFFF {
			s.Fatal("Invalid tpm_fwver version found in crossystem, got 0xFFFFFFFF")
		}
		if kernVersion == 0xFFFFFFFF {
			s.Fatal("Invalid tpm_kernver version found in crossystem, got 0xFFFFFFFF")
		}
	}

}
