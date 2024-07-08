// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"time"

	"github.com/google/go-tpm/tpm2"

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
			"cros-hwsec@google.com", // CrOS GSC Developers
			"granaghan@google.com",  // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_dt_ab", "gsc_dt_shield", "gsc_h1_shield", "gsc_ot_shield",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCOpenCCD,
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
	ecUart := b.PhysicalUart(ti50.UartEC)
	th.MustSucceed(ecUart.Open(ctx), "Open EC UART")
	defer ecUart.Close(ctx)

	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	tpm := b.ResetAndTpmStartup(ctx, i, ti50.CcdSuzyQ, ti50.FfClamshell)
	th.MustSucceed(tpm.TpmvCommitNvmem(), "Failed to enable Nvmem writes.")

	// Undefine to ensure we're starting clean.
	attr := ti50.KernelAttr()
	tpm.NvUndefineSpace(attr)
	// Make sure we clean up if the test fails.
	defer tpm.NvUndefineSpace(attr)

	v0Data := []byte{0x02, 0x4c, 0x57, 0x52, 0x47, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00,
		0x55}
	attr.DataSize = uint16(len(v0Data))

	s.Log("Define NV space")
	def := tpm2.NVDefineSpace{
		AuthHandle: tpm2.TPMRHPlatform,
		Auth:       ti50.EmptyPassword(),
		PublicInfo: tpm2.New2B(attr),
	}
	if _, err := def.Execute(tpm); err != nil {
		s.Fatal("NVDefineSpace failed: ")
	}
	defer tpm.NvUndefineSpace(attr)

	nvName, err := tpm2.NVName(&attr)
	if err != nil {
		s.Fatal("Failed to get NV name: ", err)
	}
	nvHandle := tpm2.NamedHandle{
		Handle: attr.NVIndex,
		Name:   *nvName,
	}

	s.Log("Write NV space")
	write := tpm2.NVWrite{
		AuthHandle: tpm2.TPMRHPlatform,
		NVIndex:    nvHandle,
		Data: tpm2.TPM2BMaxNVBuffer{
			Buffer: v0Data,
		},
		Offset: 0,
	}
	if _, err := write.Execute(tpm); err != nil {
		s.Fatal("NVWrite failed: ", err)
	}

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
	if err := tpm.NvUndefineSpace(attr); err != nil {
		s.Fatal("Undefine failed: ")
	}

	// Kernel file with 0 hash.
	v1Data := []byte{0x10, 0x28, 0x0c, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	attr.DataSize = uint16(len(v1Data))

	s.Log("Redefine NV space")
	def = tpm2.NVDefineSpace{
		AuthHandle: tpm2.TPMRHPlatform,
		Auth:       ti50.EmptyPassword(),
		PublicInfo: tpm2.New2B(attr),
	}
	if _, err := def.Execute(tpm); err != nil {
		s.Fatal("NVDefineSpace failed: ")
	}

	s.Log("Write NV space")
	write = tpm2.NVWrite{
		AuthHandle: tpm2.TPMRHPlatform,
		NVIndex:    nvHandle,
		Data: tpm2.TPM2BMaxNVBuffer{
			Buffer: v1Data,
		},
		Offset: 0,
	}
	if _, err := write.Execute(tpm); err != nil {
		s.Fatal("NVWrite failed: ", err)
	}

	s.Log("Check kernel hash (should succeed)")
	th.MustSucceed(utils.CheckSendEcPacket(ctx,
		b,
		ecUart,
		p,
		utils.Efs2ReturnSuccess,
	), "CheckSendEcPacket failed.")

	s.Log("Undefine NV space")
	if err := tpm.NvUndefineSpace(attr); err != nil {
		s.Fatal("Undefine failed: ")
	}

	s.Log("Check kernel hash (should fail)")
	th.MustSucceed(utils.CheckSendEcPacket(ctx,
		b,
		ecUart,
		p,
		utils.Efs2ReturnErrorNvmem,
	), "CheckSendEcPacket failed.")
}
