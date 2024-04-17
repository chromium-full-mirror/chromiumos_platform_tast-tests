// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-tpm/tpm2"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type spiImage string

const (
	validSPIImage  spiImage = "valid-32M_20231101.bin"
	badGBBSPIImage spiImage = "bad-gbb-32MB_20231101.bin"

	verificationResultSuccess                   = 0xfffff000
	verificationResultBadGBB                    = 0x11000000
	verificationResultBadSettingsNotProvisioned = 0x10000000
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

// todoWpSenseCausesReset tracks if WP monitoring is causing GSC reset. This
// is currently be rolled out slowly through UMA tracking. Once tracking looks
// good, we will enable this and can remove all of the false branches.
// See b/254309086
const todoWpSenseCausesReset = false

var (
	gsctoolApRoPassed  = regexp.MustCompile(`apro result\s*\(20\)\s*:\s*pass`)
	verificationResult = regexp.MustCompile(`AP RO verification result: [^(]+ \(0x(\w+)\)`)
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    Ti50APROVerification,
		Desc:    "Verify AP RO verification feature with valid and invalid settings",
		Timeout: 15 * time.Minute,
		Contacts: []string{
			"gsc-sheriff@google.com", // CrOS GSC Developers
			"kupiakos@google.com",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_shield"},
		Fixture:      fixture.GSCInitialFactory,
		Data:         []string{string(validSPIImage), string(badGBBSPIImage)},
	})
}

func getSPIImageContents(s *testing.State, image spiImage) []byte {
	contents, err := os.ReadFile(s.DataPath(string(image)))
	if err != nil {
		s.Fatalf("Could not read image %q: %s", string(image), err)
	}
	return contents
}

// Ti50APROVerification tests AP RO verification succeeds against a production
// image.
func Ti50APROVerification(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s)
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	s.Log("(Re)starting GSC with clamshell straps and no CCD")
	b.ResetWithStraps(ctx, ti50.FfClamshell, ti50.CcdDisconnected)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	s.Log("Provisioning AP SPI settings")
	bidSet, err := i.Command(ctx, "bid ZZCR 0x7fffffff")
	th.MustSucceed(err, "Set BID")
	if strings.Contains(bidSet, "failed") {
		s.Fatal("Could not set BID: ", bidSet)
	}

	// Found with the `src/third_party/ap_wpsr` tool with
	// `./ap_wpsr --name W25Q256JV_M --start 0 --length 0x00100000`
	wpsrSet, err := i.Command(ctx, "ap_ro_verify wpsr d4 fc 0 41")
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
	flashSPIImage(ctx, s, b, i, validSPIImage)
	verifyVerificationResultOnReboot(ctx, s, b, i, verificationResultBadSettingsNotProvisioned)
	verifyResetState(ctx, s, b, i, verificationFailedAllowReset, "AP RO failed but not latched")

	s.Log("Toggle AP on then update addr mode")
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
	expectNoGscReboot(ctx, s, i, "AP turned on")

	// Use 4 byte addressing since this is a 32 MiB chip.
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")
	modeSet, err := i.Command(ctx, "ap_ro_verify addrmode 4byte")
	th.MustSucceed(err, "Set addrmode")
	if strings.Contains(modeSet, "failed") {
		s.Fatal("Could not set address mode: ", modeSet)
	}
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
	if todoWpSenseCausesReset {
		expectGscReboot(ctx, s, i, "AP turned off")
	} else {
		b.Reset(ctx)
	}
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	s.Log("GSC should now have a passing AP RO verification which flips the latch")
	verifyVerificationResultOnReboot(ctx, s, b, i, verificationResultSuccess)
	verifyResetState(ctx, s, b, i, verificationSuccessAllowReset, "AP RO verification passes")

	// Before trying the bad image, verify that WP monitoring is working
	if todoWpSenseCausesReset {
		verifyWPMonitoring(ctx, s, b, i)
	}

	// Now all failed verification should hold system in reset when
	// AllowUnverifiedRO is false
	verifyBadImage(ctx, s, b, i, badGBBSPIImage, verificationResultBadGBB)
}

func verifyBadImage(ctx context.Context, s *testing.State, b utils.DevboardHelper, i *ti50.CrOSImage, spi spiImage, wantVerificationResult uint32) {
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
	flashSPIImage(ctx, s, b, i, spi)

	// Verify system not held in reset
	verifyVerificationResultOnReboot(ctx, s, b, i, wantVerificationResult)
	verifyResetState(ctx, s, b, i, verificationFailedAllowReset, "AP RO verification failed with AllowUnverifiedRO as always")

	// Ensure that AllowUnverifiedRo is set to never so EC is held in reset
	s.Log("Set AllowUnverifiedRo to never")
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")
	th.MustSucceed(i.TestlabOpen(ctx), "testlab open")
	th.MustSucceed(i.SetCCDCapability(ctx, ti50.AllowUnverifiedRO, ti50.CapDefault), "Set AllowUnverifiedRo to never")

	// Verify system held in reset
	verifyVerificationResultOnReboot(ctx, s, b, i, wantVerificationResult)
	verifyResetState(ctx, s, b, i, verificationFailedForcedReset, "AP RO verification failed and AllowUnverifiedRo as never")

	// Verify that the reboot is still in the failed verification state before continuing
	verifyVerificationResultOnReboot(ctx, s, b, i, wantVerificationResult)

	s.Log("Perform AP RO bypass key sequence (for clamshell)")
	performAPROBypassKeySequence(ctx, b)

	// Verify that system has been released from reset
	verifyResetState(ctx, s, b, i, verificationFailedAllowReset, "AP RO verification failed after bypass")

	s.Log("Create FWMP file that blocks CCD open (and bypass keycombo)")
	tpm = b.ResetAndTpmStartup(ctx, i)
	writeBlockingFWMPFile(ctx, s, b, tpm)
	defer tpm.NvUndefineSpace(attr)

	// Restart GSC and ensure system held in reset
	verifyVerificationResultOnReboot(ctx, s, b, i, wantVerificationResult)
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
	b.GpioApplyStrap(ctx, ti50.CcdDisconnected)
	// GoBigSleepLint: Allow USB to settle after reboot, then poll
	testing.Sleep(ctx, 5*time.Second)
	if inReset {
		b.WaitUntilCCDConnected(ctx)
	} else {
		b.CCDMustNotBeConnected(ctx, 2*time.Second)
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
	expectGscReboot(ctx, s, i, "AP turned on")
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
	expectGscReboot(ctx, s, i, "AP turned off")
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	// Now, WP_SENSE_L doesn't follow WP emulating an external driver

	// Only pulse WP disabled (externally) quickly
	b.GpioSet(ctx, ti50.GpioTi50WriteProtectSenseL, true)
	b.GpioSet(ctx, ti50.GpioTi50WriteProtectSenseL, false)
	expectNoGscReboot(ctx, s, i, "WP is externally disabled")
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
	expectGscReboot(ctx, s, i, "AP turned on")
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	b.GpioSet(ctx, ti50.GpioTi50WriteProtectSenseL, true)
	expectNoGscReboot(ctx, s, i, "WP is continually externally disabled")
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
	expectGscReboot(ctx, s, i, "AP turns off")
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	// Verify booting with already disabled WP_SENSE_L is detected, and stop externally
	// disabling WP before AP turns on to prevent boot loops
	b.GpioSet(ctx, ti50.GpioTi50WriteProtectSenseL, false)
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
	expectGscReboot(ctx, s, i, "AP turned on")
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	// Verify deep sleep WP_SENSE_L detection
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
	b.GpioSet(ctx, ti50.GpioTi50WriteProtectSenseL, false)
	s.Log("Waiting 70 seconds for GSC to go to deep sleep")
	th.MustSucceed(i.WaitUntilDeepSleep(ctx, ti50.WaitForSleepTimeout), "Enter deep sleep")

	// Externally pulse WP disable quickly. Should wake up GSC
	b.GpioSet(ctx, ti50.GpioTi50WriteProtectSenseL, true)
	b.GpioSet(ctx, ti50.GpioTi50WriteProtectSenseL, false)
	expectGscReboot(ctx, s, i, "pusling external WP while deep sleeping")
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
	expectGscReboot(ctx, s, i, "AP turned on")
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	// Verify normal sleep WP_SENSE_L detection
	s.Log("Waiting 70 seconds for GSC to go to normal sleep")
	th.MustSucceed(i.WaitUntilNormalSleep(ctx, ti50.WaitForSleepTimeout), "Enter normal sleep")

	// Externally pulse WP disable quickly
	b.GpioSet(ctx, ti50.GpioTi50WriteProtectSenseL, true)
	b.GpioSet(ctx, ti50.GpioTi50WriteProtectSenseL, false)
	expectNoGscReboot(ctx, s, i, "pulsing external WP while in normal sleep")
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
	expectGscReboot(ctx, s, i, "AP turned off")
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	// Verify chip restores external wp is dirty after deep sleep resume
	// Externally pulse WP disable quickly
	b.GpioSet(ctx, ti50.GpioTi50WriteProtectSenseL, true)
	b.GpioSet(ctx, ti50.GpioTi50WriteProtectSenseL, false)
	s.Log("Waiting 70 seconds for GSC to go to deep sleep")
	th.MustSucceed(i.WaitUntilDeepSleep(ctx, ti50.WaitForSleepTimeout), "Enter deep sleep")
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after deep sleep")
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
	expectGscReboot(ctx, s, i, "AP turned on")
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	// Verify chip does restores cooperative wp is dirty state after deep sleep
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
	th.MustSucceed(i.SetWp(ctx, false), "Disable WP")
	expectNoGscReboot(ctx, s, i, "disabling WP")
	s.Log("Waiting 70 seconds for GSC to go to deep sleep")
	th.MustSucceed(i.WaitUntilDeepSleep(ctx, ti50.WaitForSleepTimeout), "Enter deep sleep")
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after deep sleep")
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
	expectGscReboot(ctx, s, i, "AP turned on")
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")
}

func expectGscReboot(ctx context.Context, s *testing.State, i *ti50.CrOSImage, scenario string) {
	if err := i.WaitUntilRoBoot(ctx, time.Second); err != nil {
		s.Errorf("GSC did not reset when %s: %s", scenario, err)
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

func flashSPIImage(ctx context.Context, s *testing.State, b utils.DevboardHelper, i *ti50.CrOSImage, spi spiImage) {
	b.WithApFlashAccess(ctx, i, ti50.HoldInReset, func(flash ti50.ApFlash) {
		// Enable SW WP on the AP SPI chip so the status registers are as expected.
		// This range represents the RO section of the AP flash.
		// We are able to modify SW WP because HW WP is disabled due to some
		// previous "ccd reset factory".
		flash.EnableApWriteProtect(ctx, 0, 0x00100000)

		// Write the fresh AP flash image. We only care about the RO section for
		// verification but this will write the whole 32M image. The SW WP is
		// ignored because HW WP is disabled.
		s.Log("Flashing new AP image: ", string(spi))
		flash.WriteApFlash(ctx, getSPIImageContents(s, spi))
	})
}

func verifyVerificationResultOnReboot(ctx context.Context, s *testing.State, b utils.DevboardHelper, i *ti50.CrOSImage, wantVerificationResult uint32) {
	b.Reset(ctx)
	m, err := i.WaitUntilMatch(ctx, verificationResult, 5*time.Second)
	if err != nil {
		s.Fatal("Expected to see Ti50 verification result: ", err)
	}

	verificationResult := mustParseVerificationResult(s, m)
	if verificationResult != wantVerificationResult {
		s.Errorf("Unexpected verification result: got 0x%x, wanted 0x%x", verificationResult, wantVerificationResult)
	}
}

func mustParseVerificationResult(s *testing.State, m [][]byte) uint32 {
	result, err := strconv.ParseUint(string(m[1]), 16, 32)
	if err != nil {
		s.Fatalf("Could not parse verification result of %v: %s", m[1], err)
	}
	return uint32(result)
}
