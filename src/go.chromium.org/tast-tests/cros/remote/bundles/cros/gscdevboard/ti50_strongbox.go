// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    Ti50Strongbox,
		Desc:    "Test strongbox commands",
		Timeout: 10 * time.Minute,
		Contacts: []string{
			"cros-hwsec@google.com",
			"ecgh@google.com",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{
			"group:gsc",
			"gsc_dt_shield", "gsc_ot_shield",
			"gsc_image_ti50a",
		},
		Fixture: fixture.Ti50ADevboard,
	})
}

func Ti50Strongbox(ctx context.Context, s *testing.State) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	b := utils.NewDevboardHelper(s)
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	tpm := b.ResetAndTpmStartupForBus(ctx, i, ti50.TpmBusI2c, ti50.CCDModeOn, ti50.FfClamshell)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	status, _, err := utils.StrongboxCommand(ctx, tpm, utils.StrongboxDeviceGetHardwareInfo)
	s.Logf("command 0x%x -> status 0x%x", utils.StrongboxDeviceGetHardwareInfo, status)
	if err != nil {
		s.Fatal("Failed StrongboxDeviceGetHardwareInfo: ", err)
	}
	if status != utils.StrongboxSuccess {
		s.Errorf("Wrong status for StrongboxDeviceGetHardwareInfo: 0x%x", status)
	}
	status, _, err = utils.StrongboxCommand(ctx, tpm, utils.StrongboxDeviceAddRngEntropy)
	s.Logf("command 0x%x -> status 0x%x", utils.StrongboxDeviceAddRngEntropy, status)
	if err != nil {
		s.Fatal("Failed StrongboxDeviceAddRngEntropy: ", err)
	}
	if status != utils.StrongboxSuccess {
		s.Errorf("Wrong status for StrongboxDeviceAddRngEntropy: 0x%x", status)
	}
	status, _, err = utils.StrongboxCommand(ctx, tpm, utils.StrongboxDeviceGenerateKey)
	s.Logf("command 0x%x -> status 0x%x", utils.StrongboxDeviceGenerateKey, status)
	if err != nil {
		s.Fatal("Failed StrongboxDeviceGenerateKey: ", err)
	}
	if status != utils.InvalidArgument {
		s.Errorf("Wrong status for StrongboxDeviceGenerateKey: 0x%x", status)
	}
	status, _, err = utils.StrongboxCommand(ctx, tpm, utils.StrongboxDeviceImportKey)
	s.Logf("command 0x%x -> status 0x%x", utils.StrongboxDeviceImportKey, status)
	if err != nil {
		s.Fatal("Failed StrongboxDeviceImportKey: ", err)
	}
	if status != utils.StrongboxUnimplemented {
		s.Errorf("Wrong status for StrongboxDeviceImportKey: 0x%x", status)
	}
}
