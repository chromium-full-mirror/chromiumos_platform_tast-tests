// Copyright 2025 The ChromiumOS Authors
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
		Func:    Cr50StrongboxEnable,
		Desc:    "Test strongbox commands",
		Timeout: 1 * time.Minute,
		Contacts: []string{
			"cros-hwsec@google.com",
			"mruthven@google.com",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_h1_shield", "gsc_image_ti50", "gsc_nightly"},
		Fixture:      fixture.GSCOpenCCD,
	})
}

func Cr50StrongboxEnable(ctx context.Context, s *testing.State) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	tpm := b.ResetAndTpmStartupForBus(ctx, i, ti50.TpmBusSpi, ti50.CCDModeOn, ti50.FfClamshell)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	// Commands should be rejected before Strongbox is enabled.
	checkStrongboxEnable(ctx, s, b, i, tpm, ti50.StrongboxUnset, "GSC reset")

	th.MustSucceed(tpm.TpmvSetStrongboxState(true), "Enable Strongbox")
	checkStrongboxEnable(ctx, s, b, i, tpm, ti50.StrongboxEnabled, "Enabled strongbox")

	th.MustSucceed(tpm.TpmvSetStrongboxState(false), "Disable Strongbox")
	checkStrongboxEnable(ctx, s, b, i, tpm, ti50.StrongboxDisabled, "Disabled strongbox")

	err := tpm.TpmvSetStrongboxState(true)
	if err == nil {
		s.Fatal("Enabled strongbox worked after disabling it")
	}
	checkStrongboxEnable(ctx, s, b, i, tpm, ti50.StrongboxDisabled, "Reject strongbox enable")

	// Simulate S3 reset with SUState
	b.SimulateApS3(ctx, tpm)

	err = tpm.TpmvSetStrongboxState(true)
	if err == nil {
		s.Fatal("Enabled strongbox worked after TPM startup with SUState")
	}
	checkStrongboxEnable(ctx, s, b, i, tpm, ti50.StrongboxDisabled, "Reject enable after S3 resume")

	// Send invalid SUClear command
	startup := tpm2.Startup{
		StartupType: tpm2.TPMSUClear,
	}
	if _, err := startup.Execute(tpm); err == nil {
		s.Log("Sent TPM startup SUClear without shutdown")
	}
	// Verify SB enable still can't be sent
	err = tpm.TpmvSetStrongboxState(true)
	if err == nil {
		s.Fatal("Enabled strongbox without valid SUClear")
	}
	checkStrongboxEnable(ctx, s, b, i, tpm, ti50.StrongboxDisabled, "Reject enable after invalid SUClear")

	// Send valid SUClear sequence
	shutdown := tpm2.Shutdown{
		ShutdownType: tpm2.TPMSUClear,
	}
	if _, err := shutdown.Execute(tpm); err != nil {
		s.Fatal("Shutdown SUClear failed: ", err)
	}

	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
	b.WaitForTpm(ctx, tpm)

	// Verify Strongbox is still disabled
	checkStrongboxEnable(ctx, s, b, i, tpm, ti50.StrongboxDisabled, "Disabled after PLT_RST")
	// Send SUClear to reset the strongbox disable bit
	startup = tpm2.Startup{
		StartupType: tpm2.TPMSUClear,
	}
	// Sending SUClear clears strongbox disable
	if _, err := startup.Execute(tpm); err != nil {
		s.Fatal("TPM startup SUClear error: ", err)
	}
	// Verify Strongbox disable is cleared after SUClear
	checkStrongboxEnable(ctx, s, b, i, tpm, ti50.StrongboxUnset, "SUClear")

	// Verify Strongbox can then be enabled
	th.MustSucceed(tpm.TpmvSetStrongboxState(true), "Enable Strongbox")
	checkStrongboxEnable(ctx, s, b, i, tpm, ti50.StrongboxEnabled, "Enabled strongbox after SUClear")

	// Send valid SUClear sequence
	shutdown = tpm2.Shutdown{
		ShutdownType: tpm2.TPMSUClear,
	}
	if _, err := shutdown.Execute(tpm); err != nil {
		s.Fatal("Shutdown SUClear failed: ", err)
	}

	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
	b.WaitForTpm(ctx, tpm)

	// Send SUClear to reset the strongbox disable bit
	startup = tpm2.Startup{
		StartupType: tpm2.TPMSUClear,
	}
	if _, err := startup.Execute(tpm); err != nil {
		s.Fatal("TPM startup SUClear error: ", err)
	}
	// Verify Strongbox enable is still set after SUClear
	checkStrongboxEnable(ctx, s, b, i, tpm, ti50.StrongboxEnabled, "Enabled after SUClear")

	th.MustSucceed(tpm.TpmvSetStrongboxState(false), "Disable strongbox")
	// Verify Strongbox enable is still set after SUClear
	checkStrongboxEnable(ctx, s, b, i, tpm, ti50.StrongboxDisabled, "Disabled strongbox")
}

func checkStrongboxEnable(ctx context.Context, s *testing.State, b utils.DevboardHelper, i *ti50.CrOSImage, tpm *utils.TpmHelper, expectedSbState ti50.StrongboxState, desc string) {
	sbState, err := i.StrongboxState(ctx)
	if err != nil {
		s.Errorf("%s: failed to get Strongbox state %s", desc, err)
	} else if sbState != expectedSbState {
		s.Errorf("%s: unexpected Strongbox state expected %v got %v", desc, expectedSbState, sbState)
	}
	sbErr, response, err := utils.StrongboxCommand(ctx, tpm, utils.DeviceGetHardwareInfo, nil)
	if err != nil {
		s.Errorf("%s: failed to get HardwareInfo: %s", desc, err)
		return
	}
	if expectedSbState == ti50.StrongboxEnabled {
		if sbErr != utils.StrongboxSuccess {
			s.Errorf("%s: Command failed: %v", desc, sbErr)
			return
		}
		s.Log("Got hardware info")
		return
	}
	if sbErr != utils.HardwareNotYetAvailable {
		s.Errorf("%s: unexpected HardwareInfo error %s", desc, err)
	}
	if len(response) != 0 {
		s.Errorf("%s: unexpected response", desc)
	}
}
