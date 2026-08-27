// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"strings"
	"time"

	"github.com/google/go-tpm/tpm2"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/utils"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type expectedResetState int

const (
	// Verification succeeded and system reset is allowed.
	verificationSuccessAllowReset expectedResetState = iota
	// Verification failed and system reset is allowed (through latch, bypass,
	// CCD, etc.).
	verificationFailedAllowReset
	// Verification failed and system is held in reset.
	verificationFailedForcedReset
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    Ti50APROVerification,
		Desc:    "Verify AP RO verification feature with valid and invalid settings",
		Timeout: 15 * time.Minute,
		Contacts: []string{
			"cros-hwsec@google.com", // CrOS GSC Developers
			"kupiakos@google.com",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_dt_shield", "gsc_ot_shield", "gsc_ot_fpga_cw310",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCInitialFactory,
		Data:    []string{string(utils.ValidSPIImage), string(utils.BadGBBSPIImage)},
	})
}

// Ti50APROVerification tests AP RO verification succeeds against a production
// image.
func Ti50APROVerification(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	b.Reset(ctx)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")
	flashInfo := b.ProbeSPIFlashChip(ctx, i)

	s.Log("(Re)starting GSC with clamshell straps and no CCD")
	b.ResetWithStraps(ctx, ti50.FfClamshell, ti50.CCDModeOff)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	s.Log("Provisioning AP SPI settings")
	bidSet, err := i.Command(ctx, "bid ZZCR 0x7fffffff")
	th.MustSucceed(err, "Set BID")
	if strings.Contains(bidSet, "failed") {
		s.Fatal("Could not set BID: ", bidSet)
	}

	wpsrSet, err := i.Command(ctx, flashInfo.WpsrCmd)
	th.MustSucceed(err, "Set wpsr")
	if strings.Contains(wpsrSet, "failed") {
		s.Fatal("Could not set wpsr: ", wpsrSet)
	}

	// Ensure testlab mode is enabled before testing, otherwise we can get locked
	// out of ccd open when we verify that FWMP prevents bypass key sequence.
	fixture.EnsureTestLabEnabled(ctx, s, b.DUTControlAndreiboard, i)
	th.MustSucceed(i.TestlabOpen(ctx), "testlab open")
	th.MustSucceed(i.CCDResetFactory(ctx), "reset factory")
	s.Log("Setting AllowUnverifiedRo to never")
	th.MustSucceed(i.SetCCDCapability(ctx, ti50.AllowUnverifiedRO, ti50.CapDefault), "Set AllowUnverifiedRo to never")

	// Before setting the addrmode, which is required to get a passing result,
	// update the SPI flash to a good image. Use this intermediate state to verify
	// that changing the addrmode while the AP is on and then restarting the AP
	// will force GCS to perform another AP RO verification and that the result
	// will be success.
	s.Log("Verifying that initial failure before latch allows system out of reset")
	b.FlashSPIImage(ctx, i, utils.ValidSPIImage, s.DataPath(string(utils.ValidSPIImage)), flashInfo.FlashSize)
	b.VerifyVerificationResultOnReboot(ctx, i, utils.VerificationResultBadSettingsNotProvisioned)
	verifyResetState(ctx, s, b, i, verificationFailedAllowReset, "AP RO failed but not latched")

	s.Log("Toggle AP on then update addr mode")
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
	expectNoGscReboot(ctx, s, i, "AP turned on")

	// Use 4 byte addressing since this is a 32 MiB chip.
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")
	modeSet, err := i.Command(ctx, flashInfo.AddrModeCmd)
	th.MustSucceed(err, "Set addrmode")
	if strings.Contains(modeSet, "failed") {
		s.Fatal("Could not set address mode: ", modeSet)
	}
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
	expectGscReboot(ctx, s, b, i, "AP turned off")
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	s.Log("GSC should now have a passing AP RO verification which flips the latch")
	b.VerifyVerificationResultOnReboot(ctx, i, utils.VerificationResultSuccess)
	verifyResetState(ctx, s, b, i, verificationSuccessAllowReset, "AP RO verification passes")

	// Before trying the bad image, verify that WP monitoring is working
	verifyWPMonitoring(ctx, s, b, i)

	// Now all failed verification should hold system in reset when
	// AllowUnverifiedRO is false
	verifyBadImage(ctx, s, b, i, utils.BadGBBSPIImage, flashInfo.FlashSize, utils.VerificationResultBadGBB)
}

func verifyBadImage(ctx context.Context, s *testing.State, b utils.DevboardHelper, i *ti50.CrOSImage, spi utils.SpiImage, flashSize int, wantVerificationResult uint32) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}

	s.Log("Set AllowUnverifiedRo to always (via ccd factory reset)")
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")
	th.MustSucceed(i.TestlabOpen(ctx), "testlab open")
	th.MustSucceed(i.CCDResetFactory(ctx), "CCD factory reset")

	// Ensure there is no FWMP file
	tpm := b.ResetAndTpmStartup(ctx, i)
	attr := ti50.FwmpAttr()
	tpm.NvUndefineSpace(attr)
	th.MustSucceed(tpm.TpmvCommitNvmem(), "NVCommit")

	// Flash bad image on AP SPI chip. GSC held in reset after done
	b.FlashSPIImage(ctx, i, spi, s.DataPath(string(spi)), flashSize)

	// Verify system not held in reset
	b.VerifyVerificationResultOnReboot(ctx, i, wantVerificationResult)
	verifyResetState(ctx, s, b, i, verificationFailedAllowReset, "AP RO verification failed with AllowUnverifiedRO as always")

	// Ensure that AllowUnverifiedRo is set to never so EC is held in reset
	s.Log("Set AllowUnverifiedRo to never")
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")
	th.MustSucceed(i.TestlabOpen(ctx), "testlab open")
	th.MustSucceed(i.SetCCDCapability(ctx, ti50.AllowUnverifiedRO, ti50.CapDefault), "Set AllowUnverifiedRo to never")

	// Verify system held in reset
	b.VerifyVerificationResultOnReboot(ctx, i, wantVerificationResult)
	verifyResetState(ctx, s, b, i, verificationFailedForcedReset, "AP RO verification failed and AllowUnverifiedRo as never")

	// Verify that the reboot is still in the failed verification state before continuing
	b.VerifyVerificationResultOnReboot(ctx, i, wantVerificationResult)

	s.Log("Perform AP RO bypass key sequence (for clamshell)")
	performAPROBypassKeySequence(ctx, b)

	// Verify that system has been released from reset
	verifyResetState(ctx, s, b, i, verificationFailedAllowReset, "AP RO verification failed after bypass")

	s.Log("Create FWMP file that blocks CCD open (and bypass keycombo)")
	tpm = b.ResetAndTpmStartup(ctx, i)
	writeBlockingFWMPFile(ctx, s, b, tpm)
	defer tpm.NvUndefineSpace(attr)

	// Restart GSC and ensure system held in reset
	b.VerifyVerificationResultOnReboot(ctx, i, wantVerificationResult)
	verifyResetState(ctx, s, b, i, verificationFailedForcedReset, "Verification failed with FWMP before bypass")

	s.Log("Perform AP RO bypass key sequence (for clamshell) -- Should be blocked by FWMP")
	performAPROBypassKeySequence(ctx, b)

	// Verify that system is still in reset because key sequence should be block
	verifyResetState(ctx, s, b, i, verificationFailedForcedReset, "Verification failed, bypassed but blocked by FWMP")
}

func verifyResetState(ctx context.Context, s *testing.State, b utils.DevboardHelper, i *ti50.CrOSImage, resetState expectedResetState, scenario string) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}

	inReset := false
	if resetState == verificationFailedForcedReset {
		inReset = true
	}
	expectedEcResetL := !inReset
	expectedEcState := ""
	if !inReset {
		expectedEcState = "not "
	}

	// GoBigSleepLint: It is a true failure if EC isn't in correct after 0.5 sec
	testing.Sleep(ctx, 500*time.Millisecond)
	ecResetL := b.GpioGet(ctx, ti50.GpioTi50EcRstL)
	if ecResetL != expectedEcResetL {
		s.Errorf("EC %sreleased when %s", expectedEcState, scenario)
	}

	// Ensure CCD is open before calling console commands
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC unresponsive in verfyResetState")
	th.MustSucceed(i.TestlabOpen(ctx), "testlab open")

	s.Log("Verify ecrst console commands when ", scenario)
	th.MustSucceed(i.EcrstOff(ctx), "Ecrst off")
	ecResetL = b.GpioGet(ctx, ti50.GpioTi50EcRstL)
	if ecResetL != expectedEcResetL {
		s.Errorf("EC %sreleased when %s after ecrst off", expectedEcState, scenario)
	}

	th.MustSucceed(i.EcrstPulse(ctx), "Ecrst pulse")
	ecResetL = b.GpioGet(ctx, ti50.GpioTi50EcRstL)
	if ecResetL != expectedEcResetL {
		s.Errorf("EC %sreleased when %s after ecrst pulse", expectedEcState, scenario)
	}

	s.Log("Verify EC reset clamshell combo when ", scenario)
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, false)
	tapActiveLowKey(ctx, b, ti50.GpioTi50KsiRefresh)
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)
	ecResetL = b.GpioGet(ctx, ti50.GpioTi50EcRstL)
	if ecResetL != expectedEcResetL {
		s.Errorf("EC %sreleased when %s after ec reset key combo", expectedEcState, scenario)
	}

	s.Log("Verify that CCD state when ", scenario)
	b.GpioApplyStrap(ctx, ti50.CCDModeOff)
	// verificationFailedForcedReset should override CCDModeOff (see CcdModeMask::FailedVerificationMode).
	if inReset {
		b.WaitUntilCCDConnected(ctx)
	} else {
		b.CCDMustNotBeConnected(ctx, 5*time.Second)
	}

	s.Log("Ensure that GSC didn't reset from previous interactions")
	if err := i.WaitUntilRoBoot(ctx, time.Second); !errors.Is(err, context.DeadlineExceeded) {
		s.Errorf("GSC reset unexpectedly reset when %s: %s", scenario, err)
	}

	s.Log("Verify power button and GSC reset when ", scenario)
	tapActiveLowKey(ctx, b, ti50.GpioTi50PowerBtnL)
	err := i.WaitUntilRoBoot(ctx, time.Second)
	if errors.Is(err, context.DeadlineExceeded) == inReset {
		expectedGscState := ""
		if inReset {
			expectedGscState = "did not "
		}
		s.Errorf("Power button %sreset GSC when %s: %s", expectedGscState, scenario, err)
	}
}

func verifyWPMonitoring(ctx context.Context, s *testing.State, b utils.DevboardHelper, i *ti50.CrOSImage) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}

	// Set up state we need for test
	s.Log("Set AllowUnverifiedRo to always (via ccd factory reset) and set WP enabled at boot")
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")
	th.MustSucceed(i.TestlabOpen(ctx), "testlab open")
	th.MustSucceed(i.CCDResetFactory(ctx), "CCD factory reset")
	th.MustSucceed(i.SetWpAtBoot(ctx, true), "Enable WP at boot")
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
	b.GpioSet(ctx, ti50.GpioTi50WriteProtectSenseL, false)

	// Reboot so we know that WP is always enabled
	th.MustSucceed(i.SendConsoleRebootCmd(ctx), "Send reboot command")
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	// First, WP_SENSE_L follows WP emulating normal operation

	th.MustSucceed(i.SetWp(ctx, false), "Disable WP")
	b.GpioSet(ctx, ti50.GpioTi50WriteProtectSenseL, true)
	expectNoGscReboot(ctx, s, i, "disabling WP")
	th.MustSucceed(i.SetWp(ctx, true), "Enable WP")
	b.GpioSet(ctx, ti50.GpioTi50WriteProtectSenseL, false)
	expectNoGscReboot(ctx, s, i, "enabling WP")
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
	expectGscReboot(ctx, s, b, i, "AP turned on")
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
	expectNoGscReboot(ctx, s, i, "AP turned off")
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
	expectNoGscReboot(ctx, s, i, "AP turned on")
	th.MustSucceed(i.SetWp(ctx, false), "Disable WP")
	b.GpioSet(ctx, ti50.GpioTi50WriteProtectSenseL, true)
	expectNoGscReboot(ctx, s, i, "enabling WP")
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
	expectNoGscReboot(ctx, s, i, "AP turned off")
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
	expectNoGscReboot(ctx, s, i, "AP turned on")
	th.MustSucceed(i.SetWp(ctx, true), "Enable WP")

	b.GpioSet(ctx, ti50.GpioTi50WriteProtectSenseL, false)
	expectNoGscReboot(ctx, s, i, "disabling WP")
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
	expectGscReboot(ctx, s, b, i, "AP turned off")
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	// Now, WP_SENSE_L doesn't follow WP emulating an external driver

	// Only pulse WP disabled (externally) quickly
	b.GpioSet(ctx, ti50.GpioTi50WriteProtectSenseL, true)
	b.GpioSet(ctx, ti50.GpioTi50WriteProtectSenseL, false)
	expectNoGscReboot(ctx, s, i, "WP is externally disabled")
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
	expectGscReboot(ctx, s, b, i, "AP turned on")
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	b.GpioSet(ctx, ti50.GpioTi50WriteProtectSenseL, true)
	expectNoGscReboot(ctx, s, i, "WP is continually externally disabled")
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
	expectGscReboot(ctx, s, b, i, "AP turns off")
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	// Verify booting with already disabled WP_SENSE_L is detected, and stop externally
	// disabling WP before AP turns on to prevent boot loops
	b.GpioSet(ctx, ti50.GpioTi50WriteProtectSenseL, false)
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
	expectGscReboot(ctx, s, b, i, "AP turned on")
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	// Verify deep sleep WP_SENSE_L detection
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
	b.GpioSet(ctx, ti50.GpioTi50WriteProtectSenseL, false)
	s.Log("Waiting 70 seconds for GSC to go to deep sleep")
	th.MustSucceed(i.WaitUntilDeepSleep(ctx, ti50.WaitForSleepTimeout), "Enter deep sleep")

	// Externally pulse WP disable quickly. Should wake up GSC from deep sleep
	b.GpioSet(ctx, ti50.GpioTi50WriteProtectSenseL, true)
	b.GpioSet(ctx, ti50.GpioTi50WriteProtectSenseL, false)
	if err := i.WaitUntilRoBoot(ctx, time.Second); err != nil {
		s.Error("GSC did not wake from deep sleep with external WP pulse: ", err)
	}
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
	expectGscReboot(ctx, s, b, i, "AP turned on")
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	// Verify normal sleep WP_SENSE_L detection
	s.Log("Waiting 70 seconds for GSC to go to normal sleep")
	th.MustSucceed(i.WaitUntilNormalSleep(ctx, ti50.WaitForSleepTimeout), "Enter normal sleep")

	// Externally pulse WP disable quickly
	b.GpioSet(ctx, ti50.GpioTi50WriteProtectSenseL, true)
	b.GpioSet(ctx, ti50.GpioTi50WriteProtectSenseL, false)
	expectNoGscReboot(ctx, s, i, "pulsing external WP while in normal sleep")
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
	expectGscReboot(ctx, s, b, i, "AP turned off")
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	// Verify chip restores external wp is dirty after deep sleep resume
	// Externally pulse WP disable quickly
	b.GpioSet(ctx, ti50.GpioTi50WriteProtectSenseL, true)
	b.GpioSet(ctx, ti50.GpioTi50WriteProtectSenseL, false)
	s.Log("Waiting 70 seconds for GSC to go to deep sleep")
	th.MustSucceed(i.WaitUntilDeepSleep(ctx, ti50.WaitForSleepTimeout), "Enter deep sleep")
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after deep sleep")
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
	expectGscReboot(ctx, s, b, i, "AP turned on")
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	// Verify chip does restores cooperative wp is dirty state after deep sleep
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
	th.MustSucceed(i.SetWp(ctx, false), "Disable WP")
	expectNoGscReboot(ctx, s, i, "disabling WP")
	s.Log("Waiting 70 seconds for GSC to go to deep sleep")
	th.MustSucceed(i.WaitUntilDeepSleep(ctx, ti50.WaitForSleepTimeout), "Enter deep sleep")
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after deep sleep")
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
	expectGscReboot(ctx, s, b, i, "AP turned on")
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")
}

func expectGscReboot(ctx context.Context, s *testing.State, b utils.DevboardHelper, i *ti50.CrOSImage, scenario string) {
	if err := i.WaitUntilRoBoot(ctx, time.Second); err != nil {
		s.Errorf("GSC did not reset when %s: %s", scenario, err)
	} else {
		// Do pin reset to reset the rollback counter, so rollback logic won't
		// interfere with the test
		b.Reset(ctx)
	}
}

func expectNoGscReboot(ctx context.Context, s *testing.State, i *ti50.CrOSImage, scenario string) {
	if err := i.WaitUntilRoBoot(ctx, time.Second); !errors.Is(err, context.DeadlineExceeded) {
		s.Errorf("GSC unexpectedly reset when %s: %s", scenario, err)
	}
}

func writeBlockingFWMPFile(ctx context.Context, s *testing.State, b utils.DevboardHelper, tpm *utils.TpmHelper) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}

	s.Log("Write FWMP file with unlock disabled")
	fwmpFile := utils.MakeFWMPFile(utils.FWMPDisableUnlock)
	// Define space in NV storage and clean up afterwards
	attr := ti50.FwmpAttr()
	def := tpm2.NVDefineSpace{
		AuthHandle: tpm2.TPMRHPlatform,
		Auth:       ti50.EmptyPassword(),
		PublicInfo: tpm2.New2B(attr),
	}
	if _, err := def.Execute(tpm); err != nil {
		s.Fatal("NVDefineSpace failed: ")
	}

	// Write the fwmp file data to new space and immediate commit to nvmem.
	nvName, err := tpm2.NVName(&attr)
	if err != nil {
		s.Fatal("Failed to get NV name: ", err)
	}
	nvHandle := tpm2.NamedHandle{
		Handle: attr.NVIndex,
		Name:   *nvName,
	}
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

	th.MustSucceed(tpm.TpmvCommitNvmem(), "NVCommit")
}

func performAPROBypassKeySequence(ctx context.Context, b utils.DevboardHelper) {
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, false)
	tapActiveLowKey(ctx, b, ti50.GpioTi50KsiRefresh)
	tapActiveLowKey(ctx, b, ti50.GpioTi50KsiRefresh)
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)

	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, false)
	tapActiveLowKey(ctx, b, ti50.GpioTi50KsiRefresh)
	tapActiveLowKey(ctx, b, ti50.GpioTi50KsiRefresh)
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)
}

func tapActiveLowKey(ctx context.Context, b utils.DevboardHelper, gpio ti50.GpioName) {
	b.GpioSet(ctx, gpio, false)
	testing.Sleep(ctx, 100*time.Millisecond) // GoBigSleepLint: Simulating 100ms button press
	b.GpioSet(ctx, gpio, true)
}
