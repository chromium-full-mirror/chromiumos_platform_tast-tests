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
		Func:    GSCFWMP,
		Desc:    "Verifies various FWMP enforcement for GSC",
		Timeout: 30 * time.Second,
		Contacts: []string{
			"cros-hwsec@google.com", // CrOS GSC Developers
			"jettrink@chromium.org", // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_ab", "gsc_dt_shield", "gsc_h1_shield", "gsc_image_ti50", "gsc_nightly"},
		Fixture:      fixture.GSCOpenCCD,
	})
}

// GSCFWMP verifies the FWMP can force write protect to be enabled
func GSCFWMP(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s)
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	tpm := b.ResetAndTpmStartup(ctx, i, ti50.CcdSuzyQ, ti50.FfClamshell)
	err := tpm.TpmvCommitNvmem()
	if err != nil {
		s.Fatal("Failed to enable Nvmem writes: ", err)
	}

	// Undefine the space to ensure we are in a good state. Not a failure if doesn't work
	attr := ti50.FwmpAttr()
	tpm.NvUndefineSpace(attr)

	verifyWpDisabledWithFwmp(ctx, s, b, i, tpm)
}

func verifyWpDisabledWithFwmp(ctx context.Context, s *testing.State, b utils.DevboardHelper, i *ti50.CrOSImage, tpm *utils.TpmHelper) {
	s.Log("Verify WP works before FWMP unlock disable")
	_, err := i.Command(ctx, "wp disable atboot")
	if err != nil {
		s.Fatal("Error communicating with ti50: ", err)
	}

	// Ensure write protect is disabled
	if b.GpioGet(ctx, ti50.GpioTi50WriteProtectL) != true {
		s.Fatal("WP signal not disable after `wp disable` console command")
	}

	attr := ti50.FwmpAttr()
	nvName, err := tpm2.NVName(&attr)
	if err != nil {
		s.Fatal("Failed to get NV name: ", err)
	}
	nvHandle := tpm2.NamedHandle{
		Handle: attr.NVIndex,
		Name:   *nvName,
	}

	s.Log("Write FWMP file with unlock disabled")
	fwmpFile := utils.MakeFWMPFile(utils.FWMPDisableUnlock)
	// Define space in NV storage and clean up afterwards
	def := tpm2.NVDefineSpace{
		AuthHandle: tpm2.TPMRHPlatform,
		Auth:       ti50.EmptyPassword(),
		PublicInfo: tpm2.New2B(attr),
	}
	if _, err := def.Execute(tpm); err != nil {
		s.Fatal("NVDefineSpace failed: ")
	}
	defer tpm.NvUndefineSpace(attr)

	// Write the fwmp file data to new space.
	write := tpm2.NVWrite{
		AuthHandle: tpm2.TPMRHPlatform,
		NVIndex:    nvHandle,
		Data: tpm2.TPM2BMaxNVBuffer{
			Buffer: fwmpFile,
		},
		Offset: 0,
	}
	if _, err := write.Execute(tpm); err != nil {
		s.Fatal("NVWrite failed: ", err)
	}

	// Ensure Write protect is disabled
	if b.GpioGet(ctx, ti50.GpioTi50WriteProtectL) != false {
		s.Fatal("WP signal not enabled after policy gets written to NVmem with FWMP unlocked disabled")
	}

	// Type the "wp disable" command again; this should have no affect because of FWMP
	_, err = i.Command(ctx, "wp disable atboot")
	if err != nil {
		s.Fatal("Error communicating with ti50: ", err)
	}

	// Ensure Write protect is still enabled since wp command should have been blocked
	if b.GpioGet(ctx, ti50.GpioTi50WriteProtectL) != false {
		s.Fatal("WP signal not enabled after `wp disable` command, but should be blocked")
	}
}
