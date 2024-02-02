// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"crypto/rand"
	"time"

	"github.com/google/go-tpm/tpm2"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    Ti50EFS2,
		Desc:    "Verifies EFS2 communication with EC and AP",
		Timeout: 5 * time.Minute,
		Contacts: []string{
			"chromeos-faft@google.com", // CrOS Firmware Developers
			"jettrink@chromium.org",    // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_dt_ab", "gsc_dt_shield", "gsc_ot_shield",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCOpenCCD,
	})
}

func Ti50EFS2(ctx context.Context, s *testing.State) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	b := utils.NewDevboardHelper(s)
	ecUart := b.PhysicalUart(ti50.UartEC)
	th.MustSucceed(ecUart.Open(ctx), "Open EC UART")
	defer ecUart.Close(ctx)

	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	tpm := b.ResetAndTpmStartup(ctx, i, ti50.CcdSuzyQ, ti50.FfClamshell)

	// Undefine the space to ensure we are in a good state. Not a failure if doesn't work.
	attr := ti50.KernelAttr()
	tpm.NvUndefineSpace(attr)

	// Note if this goes last, it fails with a timeout reading EC console, but if even one of
	// the preceding tests is commented out, then it doesn't. It does not seem to be flaky if
	// run in a loop or anything but last.
	s.Log("Test Preamble Lengths")
	testPreambleLengths(ctx, s, b, ecUart, tpm)

	s.Log("Test Default Boot Mode")
	testDefaultBootMode(ctx, s, b, ecUart, tpm)

	s.Log("Test No Boot Mode")
	testNoBootMode(ctx, s, b, ecUart, tpm)

	s.Log("Test Verified Boot Mode")
	testVerifiedMode(ctx, s, b, ecUart, tpm)

	s.Log("Test Error Cases")
	testErrorCases(ctx, s, b, ecUart, tpm)

	s.Log("Test Kernel File Overwritten")
	testKernelFileOverwritten(ctx, s, b, ecUart, tpm)
}

// makeKernelFile create the 40 bytes kernel file with the specified EC hash and correct crc8.
func makeKernelFile(hash []byte) []byte {
	out := make([]byte, 40)
	out[0] = 0x10                // version 1.0
	out[1] = byte(len(out))      // size
	out[2] = 0                   // crc -- filled in later
	out[3] = 0x00                // flags
	out[4] = 0                   // kernel_version (1 of 4)
	out[5] = 0                   // kernel_version (2 of 4)
	out[6] = 0                   // kernel_version (3 of 4)
	out[7] = 0                   // kernel_version (4 of 4)
	copy(out[8:], hash)          // payload
	out[2] = utils.Crc8(out[3:]) // crc
	return out
}

// checkApBootMode verifies that the boot mode exposed to the AP via TPMV interface returns the
// specified wanted value.
func checkApBootMode(ctx context.Context, s *testing.State, tpm *utils.TpmHelper, wanted byte) {
	s.Logf("AP checks that boot mode is %s", utils.BootModeToString(wanted))
	got, err := tpm.TpmvGetBootMode()
	if err != nil {
		s.Error("Failed TPM command: ", err)
	}
	if wanted != got {
		s.Errorf("Boot Mode incorrect wanted %s, but got %s", utils.BootModeToString(wanted), utils.BootModeToString(got))
	}
}

// resetEc resets the EC via keyboard GSC keycombo, putting the boot most state back into TrustedRo.
func resetEc(ctx context.Context, s *testing.State, b utils.DevboardHelper) {
	s.Log("Toggle EC Reset so GSC resets boot mode to ", utils.BootModeToString(utils.Efs2BootModeTrustedRo))
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, false)
	b.GpioSet(ctx, ti50.GpioTi50KsiRefresh, false)
	testing.Sleep(ctx, 100*time.Millisecond) // GoBigSleepLint: Simulating button press
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)
	b.GpioSet(ctx, ti50.GpioTi50KsiRefresh, true)
}

func testDefaultBootMode(ctx context.Context, s *testing.State, b utils.DevboardHelper, ecUart ti50.SerialChannel, tpm *utils.TpmHelper) {
	resetEc(ctx, s, b)

	s.Log("Start gpio monitoring to ensure EC doesn't reset unexpectedly")
	gpioMonitor := b.GpioMonitorStart(ctx, ti50.GpioTi50EcRstL)
	if gpioMonitor.InitialValues[ti50.GpioTi50EcRstL] != true {
		s.Error("EC_RST_L not de-asserted before when starting monitoring")
	}
	checkApBootMode(ctx, s, tpm, utils.Efs2BootModeTrustedRo)

	s.Log("EC sends Verified as Mode. Should be rejected")
	setVerified := utils.CreateEcPacket(utils.Efs2CmdSetBootMode, []byte{utils.Efs2BootModeVerified})
	if err := utils.CheckSendEcPacket(ctx,
		b,
		ecUart,
		setVerified,
		utils.Efs2ReturnErrorBadParm,
	); err != nil {
		s.Fatal("CheckSendEcPacket failed: ", err)
	}
	checkApBootMode(ctx, s, tpm, utils.Efs2BootModeTrustedRo)

	events := b.GpioMonitorFinish(ctx, gpioMonitor)
	s.Log("Stop gpio monitoring: ", events)
	if len(events.Sorted) != 0 {
		s.Error("There should be no EC_RST_L events")
	}
}

func testNoBootMode(ctx context.Context, s *testing.State, b utils.DevboardHelper, ecUart ti50.SerialChannel, tpm *utils.TpmHelper) {
	resetEc(ctx, s, b)

	s.Log("Start gpio monitoring to ensure EC doesn't reset unexpectedly")
	gpioMonitor := b.GpioMonitorStart(ctx, ti50.GpioTi50EcRstL)
	if gpioMonitor.InitialValues[ti50.GpioTi50EcRstL] != true {
		s.Error("EC_RST_L not de-asserted before when starting monitoring")
	}

	s.Log("EC sends NO_BOOT")
	setNoBoot := utils.CreateEcPacket(utils.Efs2CmdSetBootMode, []byte{utils.Efs2BootModeNoBoot})
	if err := utils.CheckSendEcPacket(ctx,
		b,
		ecUart,
		setNoBoot,
		utils.Efs2ReturnSuccess,
	); err != nil {
		s.Fatal("CheckSendEcPacket failed: ", err)
	}
	checkApBootMode(ctx, s, tpm, utils.Efs2BootModeNoBoot)

	// Sending same command from EC should always succeed to handle failed and retried transactions.
	s.Log("EC sends NO_BOOT again and should succeed")
	if err := utils.CheckSendEcPacket(ctx,
		b,
		ecUart,
		setNoBoot,
		utils.Efs2ReturnSuccess,
	); err != nil {
		s.Fatal("CheckSendEcPacket failed: ", err)
	}
	checkApBootMode(ctx, s, tpm, utils.Efs2BootModeNoBoot)

	s.Log("EC sends Verified as Mode. Should be rejected")
	setVerified := utils.CreateEcPacket(utils.Efs2CmdSetBootMode, []byte{utils.Efs2BootModeVerified})
	if err := utils.CheckSendEcPacket(ctx,
		b,
		ecUart,
		setVerified,
		utils.Efs2ReturnErrorBadParm,
	); err != nil {
		s.Fatal("CheckSendEcPacket failed: ", err)
	}
	checkApBootMode(ctx, s, tpm, utils.Efs2BootModeNoBoot)

	events := b.GpioMonitorFinish(ctx, gpioMonitor)
	s.Log("Stop gpio monitoring: ", events)
	if len(events.Sorted) != 0 {
		s.Error("There should be no EC_RST_L events")
	}

	s.Log("EC sends TrustedRO as Mode. Should be Trigger EC reset")
	setTrustedRo := utils.CreateEcPacket(utils.Efs2CmdSetBootMode, []byte{utils.Efs2BootModeTrustedRo})

	s.Log("Start gpio monitoring to ensure EC resets with trying to set TrustedRO")
	gpioMonitor = b.GpioMonitorStart(ctx, ti50.GpioTi50EcRstL)
	if gpioMonitor.InitialValues[ti50.GpioTi50EcRstL] != true {
		s.Error("EC_RST_L not de-asserted before when starting monitoring")
	}
	if err := utils.SendEcPacketNoResponse(ctx,
		b,
		ecUart,
		setTrustedRo,
	); err != nil {
		s.Fatal("SendEcPacketNoResponse failed: ", err)
	}
	// Wait for gpio monitoring to see EC_RST edge.
	testing.Sleep(ctx, 500*time.Millisecond) // GoBigSleepLint: No good way to poll for EC_RST
	events = b.GpioMonitorFinish(ctx, gpioMonitor)
	s.Log("Stop gpio monitoring: ", events)
	ecReset := events.FindFirst(ti50.GpioTi50EcRstL, utils.GpioEdgeFalling)
	if ecReset == nil {
		s.Error("EC did not reset after invalid call to set TrustedRO")
	} else {
		if events.FindFirstAfter(*ecReset, ti50.GpioTi50EcRstL) == nil {
			s.Error("EC did not release after invalid call to set TrustedRO")
		}
	}
	checkApBootMode(ctx, s, tpm, utils.Efs2BootModeTrustedRo)
}

func testVerifiedMode(ctx context.Context, s *testing.State, b utils.DevboardHelper, ecUart ti50.SerialChannel, tpm *utils.TpmHelper) {
	resetEc(ctx, s, b)

	s.Log("Start gpio monitoring to ensure EC doesn't reset unexpectedly")
	gpioMonitor := b.GpioMonitorStart(ctx, ti50.GpioTi50EcRstL)
	if gpioMonitor.InitialValues[ti50.GpioTi50EcRstL] != true {
		s.Error("EC_RST_L not de-asserted before when starting monitoring")
	}

	// Create random hash string
	hash := make([]byte, 32)
	_, err := rand.Read(hash)
	if err != nil {
		s.Fatal("Error getting random hash: ", err)
	}

	attr := ti50.KernelAttr()

	s.Log("Setting up kernel file with random ec hash: ", hash)
	kernelFile := makeKernelFile(hash)
	// Define space in NV storage and clean up afterwards or subsequent runs will fail.
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

	// Write the kernel file data to new space.
	write := tpm2.NVWrite{
		AuthHandle: tpm2.TPMRHPlatform,
		NVIndex: tpm2.NamedHandle{
			Handle: attr.NVIndex,
			Name:   *nvName,
		},
		Data: tpm2.TPM2BMaxNVBuffer{
			Buffer: kernelFile,
		},
		Offset: 0,
	}
	if _, err := write.Execute(tpm); err != nil {
		s.Fatal("NVWrite failed: ", err)
	}

	s.Log("EC sends verify hash and expected success")
	verifyEcPacket := utils.CreateEcPacket(utils.Efs2CmdVerifyHash, hash)
	if err := utils.CheckSendEcPacket(ctx,
		b,
		ecUart,
		verifyEcPacket,
		utils.Efs2ReturnSuccess,
	); err != nil {
		s.Fatal("CheckSendEcPacket failed: ", err)
	}
	checkApBootMode(ctx, s, tpm, utils.Efs2BootModeVerified)

	// GSC needs to be able to handle duplicate requests without adverse side affects to handle
	// communication retries.
	s.Log("EC sends same verify hash and expected success with now reboot")
	if err := utils.CheckSendEcPacket(ctx,
		b,
		ecUart,
		verifyEcPacket,
		utils.Efs2ReturnSuccess,
	); err != nil {
		s.Fatal("CheckSendEcPacket failed: ", err)
	}
	checkApBootMode(ctx, s, tpm, utils.Efs2BootModeVerified)

	s.Log("EC sends Verified as Mode. Should be rejected")
	setVerified := utils.CreateEcPacket(utils.Efs2CmdSetBootMode, []byte{utils.Efs2BootModeVerified})
	if err := utils.CheckSendEcPacket(ctx,
		b,
		ecUart,
		setVerified,
		utils.Efs2ReturnErrorBadParm,
	); err != nil {
		s.Fatal("CheckSendEcPacket failed: ", err)
	}
	checkApBootMode(ctx, s, tpm, utils.Efs2BootModeVerified)

	s.Log("EC sends NO_BOOT and should transition from VERIFIED")
	setNoBoot := utils.CreateEcPacket(utils.Efs2CmdSetBootMode, []byte{utils.Efs2BootModeNoBoot})
	if err := utils.CheckSendEcPacket(ctx,
		b,
		ecUart,
		setNoBoot,
		utils.Efs2ReturnSuccess,
	); err != nil {
		s.Fatal("CheckSendEcPacket failed: ", err)
	}
	checkApBootMode(ctx, s, tpm, utils.Efs2BootModeNoBoot)

	events := b.GpioMonitorFinish(ctx, gpioMonitor)
	s.Log("Stop gpio monitoring: ", events)
	if len(events.Sorted) != 0 {
		s.Error("There should be no EC_RST_L events")
	}

	s.Log("EC sends TrustedRO as Mode. Should be Trigger EC reset")
	setTrustedRo := utils.CreateEcPacket(utils.Efs2CmdSetBootMode, []byte{utils.Efs2BootModeTrustedRo})
	gpioMonitor = b.GpioMonitorStart(ctx, ti50.GpioTi50EcRstL)
	if gpioMonitor.InitialValues[ti50.GpioTi50EcRstL] != true {
		s.Error("EC_RST_L not de-asserted before when starting monitoring")
	}
	if err := utils.SendEcPacketNoResponse(ctx,
		b,
		ecUart,
		setTrustedRo,
	); err != nil {
		s.Fatal("SendEcPacketNoResponse failed: ", err)
	}
	// Wait for gpio monitoring to see EC_RST edge.
	testing.Sleep(ctx, 500*time.Millisecond) // GoBigSleepLint: No good way to poll for EC_RST
	events = b.GpioMonitorFinish(ctx, gpioMonitor)
	s.Log("Stop gpio monitoring: ", events)
	ecReset := events.FindFirst(ti50.GpioTi50EcRstL, utils.GpioEdgeFalling)
	if ecReset == nil {
		s.Error("EC did not reset after invalid call to set TrustedRO")
	} else {
		if events.FindFirstAfter(*ecReset, ti50.GpioTi50EcRstL) == nil {
			s.Error("EC did not release after invalid call to set TrustedRO")
		}
	}
	checkApBootMode(ctx, s, tpm, utils.Efs2BootModeTrustedRo)
}

func testErrorCases(ctx context.Context, s *testing.State, b utils.DevboardHelper, ecUart ti50.SerialChannel, tpm *utils.TpmHelper) {
	resetEc(ctx, s, b)

	s.Log("Start gpio monitoring to ensure EC doesn't reset unexpectedly")
	gpioMonitor := b.GpioMonitorStart(ctx, ti50.GpioTi50EcRstL)
	if gpioMonitor.InitialValues[ti50.GpioTi50EcRstL] != true {
		s.Error("EC_RST_L not de-asserted before when starting monitoring")
	}

	// Create random hash string.
	hash := make([]byte, 32)
	_, err := rand.Read(hash)
	if err != nil {
		s.Fatal("Error getting random hash: ", err)
	}

	attr := ti50.KernelAttr()
	tpm.NvUndefineSpace(attr)

	s.Log("Setting up kernel file with random ec hash: ", hash)
	kernelFile := makeKernelFile(hash)
	// Define space in NV storage and clean up afterwards or subsequent runs will fail.
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

	// Write the kernel file data to new space.
	write := tpm2.NVWrite{
		AuthHandle: tpm2.TPMRHPlatform,
		NVIndex:    nvHandle,
		Data: tpm2.TPM2BMaxNVBuffer{
			Buffer: kernelFile,
		},
		Offset: 0,
	}
	if _, err := write.Execute(tpm); err != nil {
		s.Fatal("NVWrite failed: ", err)
	}

	s.Log("EC sends verify hash with wrong hash and expect automatic transition to NO_BOOT")
	// Flip the all the bits in the first hash byte from what is stored in kernel file.
	hash[0] ^= 0xFF
	verifyEcPacket := utils.CreateEcPacket(utils.Efs2CmdVerifyHash, hash)
	if err := utils.CheckSendEcPacket(ctx,
		b,
		ecUart,
		verifyEcPacket,
		utils.Efs2ReturnErrorBadPayload,
	); err != nil {
		s.Fatal("CheckSendEcPacket failed: ", err)
	}
	checkApBootMode(ctx, s, tpm, utils.Efs2BootModeNoBoot)

	// Ensure no EC resets until this point
	events := b.GpioMonitorFinish(ctx, gpioMonitor)
	s.Log("Stop gpio monitoring: ", events)
	if len(events.Sorted) != 0 {
		s.Error("There should be no EC_RST_L events")
	}

	// Reset EC so we can get back to Trusted RO.
	resetEc(ctx, s, b)

	// Restart EC_RST_L monitoring
	gpioMonitor = b.GpioMonitorStart(ctx, ti50.GpioTi50EcRstL)
	if gpioMonitor.InitialValues[ti50.GpioTi50EcRstL] != true {
		s.Error("EC_RST_L not de-asserted before when starting monitoring")
	}

	// Flip the all the bits in the last byte after calculating crc8.
	s.Log("EC sends verify hash with wrong crc8 and expects error")
	badCrc := utils.CreateEcPacket(utils.Efs2CmdVerifyHash, hash)
	badCrc[len(badCrc)-1] ^= 0xFF
	if err := utils.CheckSendEcPacket(ctx,
		b,
		ecUart,
		badCrc,
		utils.Efs2ReturnErrorCrc,
	); err != nil {
		s.Fatal("CheckSendEcPacket failed: ", err)
	}
	checkApBootMode(ctx, s, tpm, utils.Efs2BootModeTrustedRo)

	s.Log("EC sends unknown command and expects error")
	badCmd := utils.CreateEcPacket(utils.Efs2Cmd(78), []byte{})
	if err := utils.CheckSendEcPacket(ctx,
		b,
		ecUart,
		badCmd,
		utils.Efs2ReturnErrorUndefinedCmd,
	); err != nil {
		s.Fatal("CheckSendEcPacket failed: ", err)
	}
	checkApBootMode(ctx, s, tpm, utils.Efs2BootModeTrustedRo)

	s.Log("EC sends bad version and expects error")
	badVersion := utils.CreateEcPacket(utils.Efs2CmdVerifyHash, []byte{})
	// Increment the version field which should be 5 byte from the end.
	badVersion[len(badVersion)-5]++
	if err := utils.CheckSendEcPacket(ctx,
		b,
		ecUart,
		badVersion,
		utils.Efs2ReturnErrorVersion,
	); err != nil {
		s.Fatal("CheckSendEcPacket failed: ", err)
	}
	checkApBootMode(ctx, s, tpm, utils.Efs2BootModeTrustedRo)

	s.Log("EC sends bad magic and expects error")
	badMagic := utils.CreateEcPacket(utils.Efs2CmdVerifyHash, []byte{})
	// Increment the second magic field which should be 6 byte from the end.
	badMagic[len(badMagic)-6]++
	if err := utils.CheckSendEcPacket(ctx,
		b,
		ecUart,
		badMagic,
		utils.Efs2ReturnErrorMagic,
	); err != nil {
		s.Fatal("CheckSendEcPacket failed: ", err)
	}
	checkApBootMode(ctx, s, tpm, utils.Efs2BootModeTrustedRo)

	s.Log("EC sends too small verified hash and expects error")
	badSizeVerified := utils.CreateEcPacket(utils.Efs2CmdVerifyHash, []byte{1, 2})
	if err := utils.CheckSendEcPacket(ctx,
		b,
		ecUart,
		badSizeVerified,
		utils.Efs2ReturnErrorSize,
	); err != nil {
		s.Fatal("CheckSendEcPacket failed: ", err)
	}
	checkApBootMode(ctx, s, tpm, utils.Efs2BootModeTrustedRo)

	s.Log("EC sends too large verified hash size and expects error")
	badSizeVerified2 := utils.CreateEcPacket(utils.Efs2CmdVerifyHash, make([]byte, 30))
	if err := utils.CheckSendEcPacket(ctx,
		b,
		ecUart,
		badSizeVerified2,
		utils.Efs2ReturnErrorSize,
	); err != nil {
		s.Fatal("CheckSendEcPacket failed: ", err)
	}
	checkApBootMode(ctx, s, tpm, utils.Efs2BootModeTrustedRo)

	s.Log("EC sends too small set boot mode and expects error")
	badSizeBootMode := utils.CreateEcPacket(utils.Efs2CmdSetBootMode, []byte{})
	if err := utils.CheckSendEcPacket(ctx,
		b,
		ecUart,
		badSizeBootMode,
		utils.Efs2ReturnErrorSize,
	); err != nil {
		s.Fatal("CheckSendEcPacket failed: ", err)
	}
	checkApBootMode(ctx, s, tpm, utils.Efs2BootModeTrustedRo)

	s.Log("EC sends too large set boot mode and expects error")
	badSizeBootMode2 := utils.CreateEcPacket(utils.Efs2CmdSetBootMode, make([]byte, 2))
	if err := utils.CheckSendEcPacket(ctx,
		b,
		ecUart,
		badSizeBootMode2,
		utils.Efs2ReturnErrorSize,
	); err != nil {
		s.Fatal("CheckSendEcPacket failed: ", err)
	}
	checkApBootMode(ctx, s, tpm, utils.Efs2BootModeTrustedRo)

	events = b.GpioMonitorFinish(ctx, gpioMonitor)
	s.Log("Stop gpio monitoring: ", events)
	if len(events.Sorted) != 0 {
		s.Error("There should be no EC_RST_L events")
	}
}

func testKernelFileOverwritten(ctx context.Context, s *testing.State, b utils.DevboardHelper, ecUart ti50.SerialChannel, tpm *utils.TpmHelper) {
	resetEc(ctx, s, b)

	s.Log("Start gpio monitoring to ensure EC doesn't reset unexpectedly")
	gpioMonitor := b.GpioMonitorStart(ctx, ti50.GpioTi50EcRstL)
	if gpioMonitor.InitialValues[ti50.GpioTi50EcRstL] != true {
		s.Error("EC_RST_L not de-asserted before when starting monitoring")
	}

	// Create random hash string.
	hash := make([]byte, 32)
	_, err := rand.Read(hash)
	if err != nil {
		s.Fatal("Error getting random hash: ", err)
	}

	attr := ti50.KernelAttr()
	tpm.NvUndefineSpace(attr)

	s.Log("Setting up kernel file with random ec hash: ", hash)
	kernelFile := makeKernelFile(hash)
	// Define space in NV storage and clean up afterwards or subsequent runs will fail.
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

	// Write the kernel file data to new space.
	write := tpm2.NVWrite{
		AuthHandle: tpm2.TPMRHPlatform,
		NVIndex:    nvHandle,
		Data: tpm2.TPM2BMaxNVBuffer{
			Buffer: kernelFile,
		},
		Offset: 0,
	}
	if _, err := write.Execute(tpm); err != nil {
		s.Fatal("NVWrite failed: ", err)
	}

	s.Log("EC sends verify hash and expects success")
	// Flip the all the bits in the first hash byte from what is stored in kernel file.
	verifyEcPacket := utils.CreateEcPacket(utils.Efs2CmdVerifyHash, hash)
	if err := utils.CheckSendEcPacket(ctx,
		b,
		ecUart,
		verifyEcPacket,
		utils.Efs2ReturnSuccess,
	); err != nil {
		s.Fatal("CheckSendEcPacket failed: ", err)
	}
	checkApBootMode(ctx, s, tpm, utils.Efs2BootModeVerified)

	s.Log("AP changes kernel file without EC reset")
	// Change first byte of hash and re-write file.
	hash[0] ^= 0xFF
	kernelFile = makeKernelFile(hash)
	write = tpm2.NVWrite{
		AuthHandle: tpm2.TPMRHPlatform,
		NVIndex:    nvHandle,
		Data: tpm2.TPM2BMaxNVBuffer{
			Buffer: kernelFile,
		},
		Offset: 0,
	}
	if _, err := write.Execute(tpm); err != nil {
		s.Fatal("NVWrite failed: ", err)
	}

	s.Log("EC sends verify hash with old hash and expects failure")
	if err := utils.CheckSendEcPacket(ctx,
		b,
		ecUart,
		verifyEcPacket,
		utils.Efs2ReturnErrorBadPayload,
	); err != nil {
		s.Fatal("CheckSendEcPacket failed: ", err)
	}
	checkApBootMode(ctx, s, tpm, utils.Efs2BootModeNoBoot)

	events := b.GpioMonitorFinish(ctx, gpioMonitor)
	s.Log("Stop gpio monitoring: ", events)
	if len(events.Sorted) != 0 {
		s.Error("There should be no EC_RST_L events")
	}
}

func testPreambleLengths(ctx context.Context, s *testing.State, b utils.DevboardHelper, ecUart ti50.SerialChannel, tpm *utils.TpmHelper) {
	resetEc(ctx, s, b)

	s.Log("Start gpio monitoring to ensure EC doesn't reset unexpectedly")
	gpioMonitor := b.GpioMonitorStart(ctx, ti50.GpioTi50EcRstL)
	if gpioMonitor.InitialValues[ti50.GpioTi50EcRstL] != true {
		s.Error("EC_RST_L not de-asserted before when starting monitoring")
	}

	checkApBootMode(ctx, s, tpm, utils.Efs2BootModeTrustedRo)

	s.Log("EC sends packet with too short of preamble to set NO_BOOT. Should have no affect")
	tooShortPreamble := utils.CreateEcPacket(utils.Efs2CmdSetBootMode, []byte{utils.Efs2BootModeNoBoot})
	tooShortPreamble = tooShortPreamble[len(utils.Efs2Preamble)/2:]
	if err := utils.SendEcPacketNoResponse(ctx,
		b,
		ecUart,
		tooShortPreamble,
	); err != nil {
		s.Fatal("SendEcPacketNoResponse failed: ", err)
	}
	checkApBootMode(ctx, s, tpm, utils.Efs2BootModeTrustedRo)

	s.Log("EC sends packet with long preamble to set NO_BOOT. Should work")
	longPreamble := utils.CreateEcPacket(utils.Efs2CmdSetBootMode, []byte{utils.Efs2BootModeNoBoot})
	longPreamble = append(utils.Efs2Preamble, longPreamble...)
	longPreamble = append(utils.Efs2Preamble, longPreamble...)
	longPreamble = append(utils.Efs2Preamble, longPreamble...)
	if err := utils.CheckSendEcPacket(ctx,
		b,
		ecUart,
		longPreamble,
		utils.Efs2ReturnSuccess,
	); err != nil {
		s.Fatal("CheckSendEcPacket failed: ", err)
	}
	checkApBootMode(ctx, s, tpm, utils.Efs2BootModeNoBoot)

	events := b.GpioMonitorFinish(ctx, gpioMonitor)
	s.Log("Stop gpio monitoring: ", events)
	if len(events.Sorted) != 0 {
		s.Error("There should be no EC_RST_L events")
	}
}
