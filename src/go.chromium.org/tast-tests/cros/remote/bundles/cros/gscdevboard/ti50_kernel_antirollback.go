// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"time"

	"github.com/google/go-tpm/legacy/tpm2"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    Ti50KernelAntirollback,
		Desc:    "Tests creating, deleting, and recreating the kernel antirollback space",
		Timeout: 30 * time.Second,
		Contacts: []string{
			"chromeos-faft@google.com", // CrOS Firmware Developers
			"granaghan@google.com",     // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_ab", "gsc_image_ti50", "gsc_nightly", "gsc_dt_shield", "gsc_h1_shield"},
		Fixture:      fixture.GSCOpenCCD,
	})
}

func Ti50KernelAntirollback(ctx context.Context, s *testing.State) {
	// Test that the kernel antirollback space can be created, deleted, and recreated.
	// 1. Create v0 antirollback space.
	// 2. Attempt to verify against a v1 hash and check for failure.
	// 3. Undefine space.
	// 4. Create v1 antirollback space with 0 hash.
	// 5. Verify against 0 hash. Should succeed.
	// 6. Undefine space.
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	b := utils.NewDevboardHelper(s)
	ecUart := b.PhysicalUart(ti50.UartEC, time.Second)
	th.MustSucceed(ecUart.Open(ctx), "Open EC UART")
	defer ecUart.Close(ctx)

	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	tpm := b.ResetAndTpmStartup(ctx, i, ti50.TpmBusSpi, ti50.CcdSuzyQ, ti50.FfClamshell)
	th.MustSucceed(tpm.TpmvCommitNvmem(), "Failed to enable Nvmem writes.")

	// Undefine to ensure we're starting clean.
	tpm2.NVUndefineSpace(tpm, ti50.EmptyPassword, ti50.RootPlatformHandle, ti50.KernelNvIndex)
	// Make sure we clean up if the test fails.
	defer tpm2.NVUndefineSpace(tpm,
		ti50.EmptyPassword,
		ti50.RootPlatformHandle,
		ti50.KernelNvIndex,
	)

	v0Data := []byte{0x02, 0x4c, 0x57, 0x52, 0x47, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00,
		0x55}

	s.Log("Define NV space")
	th.MustSucceed(tpm2.NVDefineSpace(tpm,
		ti50.RootPlatformHandle,
		ti50.KernelNvIndex,
		ti50.EmptyPassword,
		ti50.EmptyPassword,
		nil,
		ti50.KernelFileAttr,
		uint16(len(v0Data)),
	), "NVDefineSpace failed.")

	s.Log("Write NV space")
	th.MustSucceed(tpm2.NVWrite(tpm,
		ti50.RootPlatformHandle,
		ti50.KernelNvIndex,
		ti50.EmptyPassword,
		v0Data,
		0,
	), "NVWrite failed.")

	// Test against a zero hash.
	hash := make([]byte, 32)
	p := utils.CreateEcPacket(utils.Efs2CmdVerifyHash, hash)

	s.Log("Check kernel hash (should fail)")
	th.MustSucceed(utils.CheckSendEcPacket(ctx,
		b,
		ecUart,
		p,
		utils.Efs2ReturnErrorNvmem,
	), "CheckSendEcPacket failed.")

	s.Log("Undefine NV space")
	th.MustSucceed(tpm2.NVUndefineSpace(tpm,
		ti50.EmptyPassword,
		ti50.RootPlatformHandle,
		ti50.KernelNvIndex,
	), "Undefine failed.")

	// Kernel file with 0 hash.
	v1Data := []byte{0x10, 0x28, 0x0c, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

	s.Log("Redefine NV space")
	th.MustSucceed(tpm2.NVDefineSpace(tpm,
		ti50.RootPlatformHandle,
		ti50.KernelNvIndex,
		ti50.EmptyPassword,
		ti50.EmptyPassword,
		nil,
		ti50.KernelFileAttr,
		uint16(len(v1Data)),
	), "NVDefineSpace failed.")

	s.Log("Write NV space")
	th.MustSucceed(tpm2.NVWrite(tpm,
		ti50.RootPlatformHandle,
		ti50.KernelNvIndex,
		ti50.EmptyPassword,
		v1Data,
		0,
	), "NVWrite failed.")

	s.Log("Check kernel hash (should succeed)")
	th.MustSucceed(utils.CheckSendEcPacket(ctx,
		b,
		ecUart,
		p,
		utils.Efs2ReturnSuccess,
	), "CheckSendEcPacket failed.")

	s.Log("Undefine NV space")
	th.MustSucceed(tpm2.NVUndefineSpace(tpm,
		ti50.EmptyPassword,
		ti50.RootPlatformHandle,
		ti50.KernelNvIndex,
	), "Undefine failed.")

	s.Log("Check kernel hash (should fail)")
	th.MustSucceed(utils.CheckSendEcPacket(ctx,
		b,
		ecUart,
		p,
		utils.Efs2ReturnErrorNvmem,
	), "CheckSendEcPacket failed.")
}
