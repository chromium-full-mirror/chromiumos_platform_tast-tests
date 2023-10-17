// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"bytes"
	"context"
	"crypto/rand"
	"time"

	"github.com/google/go-tpm/legacy/tpm2"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"

	"go.chromium.org/tast/core/testing"
)

var (
	efs2Preamble                = []byte{0xec, 0xec, 0xec, 0xec, 0xec}
	efs2ReturnSuccess           = []byte{0x00, 0xec}
	efs2ReturnErrorUnknown      = []byte{0x01, 0xec}
	efs2ReturnErrorMagic        = []byte{0x02, 0xec}
	efs2ReturnErrorCrc          = []byte{0x03, 0xec}
	efs2ReturnErrorSize         = []byte{0x04, 0xec}
	efs2ReturnErrorTimeout      = []byte{0x05, 0xec}
	efs2ReturnErrorUndefinedCmd = []byte{0x06, 0xec}
	efs2ReturnErrorBadPayload   = []byte{0x07, 0xec}
	efs2ReturnErrorVersion      = []byte{0x08, 0xec}
	efs2ReturnErrorNvmem        = []byte{0x09, 0xec}
	efs2ReturnErrorBadParm      = []byte{0x0A, 0xec}
)

type efs2Cmd uint

const (
	efs2CmdSetBootMode efs2Cmd = 1
	efs2CmdVerifyHash  efs2Cmd = 2
)

const (
	efs2BootModeVerified  byte = 0
	efs2BootModeNoBoot    byte = 1
	efs2BootModeTrustedRo byte = 2
)

func bootModeToString(mode byte) string {
	switch mode {
	case efs2BootModeVerified:
		return "VERIFIED_RW"
	case efs2BootModeNoBoot:
		return "NO_BOOT"
	case efs2BootModeTrustedRo:
		return "TRUSTED_RO"
	default:
		return "UNKNOWN"
	}
}

func init() {
	testing.AddTest(&testing.Test{
		Func:    Ti50Efs2,
		Desc:    "Verifies EFS2 communication with EC and AP",
		Timeout: 5 * time.Minute,
		Contacts: []string{
			"chromeos-faft@google.com", // CrOS Firmware Developers
			"jettrink@chromium.org",    // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_ab", "gsc_dt_shield", "gsc_image_ti50", "gsc_nightly"},
		Fixture:      fixture.Ti50CcdOpen,
	})
}

func Ti50Efs2(ctx context.Context, s *testing.State) {
	f := s.FixtValue().(*fixture.Value)
	b := utils.NewDevboardHelper(f, s)
	i := ti50.NewCrOSImage(b)
	ecUart := b.PhysicalUart(ti50.UartEC, time.Second)

	tpm := b.ResetAndTpmStartup(ctx, i, ti50.TpmBusSpi, ti50.CcdSuzyQ, ti50.FfClamshell)

	// Undefine the space to ensure we are in a good state. Not a failure if doesn't work/
	tpm2.NVUndefineSpace(tpm, ti50.EmptyPassword, ti50.RootPlatformHandle, ti50.KernelFileID)

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

// createEcPacket creates an EFS2 packet with the correct formatting and crc8 data with the
// specified command and payload.
func createEcPacket(cmd efs2Cmd, payload []byte) []byte {
	data := make([]byte, 7+len(payload))
	data[0] = 'E'                  // magic
	data[1] = 'C'                  // magic
	data[2] = 0                    // version
	data[3] = 0                    // crc -- filled in later
	data[4] = byte(cmd)            // cmd low byte (no command is more than 255)
	data[5] = 0                    // cmd high byte
	data[6] = byte(len(payload))   // size
	copy(data[7:], payload)        // payload
	data[3] = utils.Crc8(data[4:]) // crc
	return append(efs2Preamble, data...)
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

// checkSendEcPacket sends the specified EC EFS2 packet and tests whether the returned EFS2 code
// is the specified wanted value.
func checkSendEcPacket(ctx context.Context, s *testing.State, b utils.DevboardHelper, ecUart ti50.SerialChannel, packet, want []byte) {
	b.GpioSet(ctx, ti50.GpioTi50EcPacketMode, true)
	defer b.GpioSet(ctx, ti50.GpioTi50EcPacketMode, false)

	if err := ecUart.ClearInput(ctx); err != nil {
		s.Error("Could not clear EC serial: ", err)
	}
	if err := ecUart.WriteSerial(ctx, packet); err != nil {
		s.Error("Could not write EC serial: ", err)
	}
	got, err := ecUart.ReadSerialBytes(ctx, 2)
	if err != nil {
		s.Error("Could not read EC serial: ", err)
	} else if !bytes.Equal(got, want) {
		s.Errorf("GSC did not respond to EC packet correctly. Got %v, wanted %v", got, want)
	}
}

// sendEcPacketNoResponse sends the specified EC EFS2 packet, but does not try to read any response.
// This is useful when the GSC is expected to reset the EC and does not respond with any value.
func sendEcPacketNoResponse(ctx context.Context, s *testing.State, b utils.DevboardHelper, ecUart ti50.SerialChannel, packet []byte) {
	b.GpioSet(ctx, ti50.GpioTi50EcPacketMode, true)
	defer b.GpioSet(ctx, ti50.GpioTi50EcPacketMode, false)

	if err := ecUart.ClearInput(ctx); err != nil {
		s.Error("Could not clear EC serial: ", err)
	}
	if err := ecUart.WriteSerial(ctx, packet); err != nil {
		s.Error("Could not write EC serial: ", err)
	}
}

// checkApBootMode verifies that the boot mode exposed to the AP via TPMV interface returns the
// specified wanted value.
func checkApBootMode(ctx context.Context, s *testing.State, tpm *utils.TpmHelper, wanted byte) {
	s.Logf("AP checks that boot mode is %s", bootModeToString(wanted))
	got, err := tpm.TpmvGetBootMode()
	if err != nil {
		s.Error("Failed TPM command: ", err)
	}
	if wanted != got {
		s.Errorf("Boot Mode incorrect wanted %s, but got %s", bootModeToString(wanted), bootModeToString(got))
	}
}

// resetEc resets the EC via keyboard GSC keycombo, putting the boot most state back into TrustedRo.
func resetEc(ctx context.Context, s *testing.State, b utils.DevboardHelper) {
	s.Log("Toggle EC Reset so GSC resets boot mode to ", bootModeToString(efs2BootModeTrustedRo))
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
	checkApBootMode(ctx, s, tpm, efs2BootModeTrustedRo)

	s.Log("EC sends Verified as Mode. Should be rejected")
	setVerified := createEcPacket(efs2CmdSetBootMode, []byte{efs2BootModeVerified})
	checkSendEcPacket(ctx, s, b, ecUart, setVerified, efs2ReturnErrorBadParm)
	checkApBootMode(ctx, s, tpm, efs2BootModeTrustedRo)

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
	setNoBoot := createEcPacket(efs2CmdSetBootMode, []byte{efs2BootModeNoBoot})
	checkSendEcPacket(ctx, s, b, ecUart, setNoBoot, efs2ReturnSuccess)
	checkApBootMode(ctx, s, tpm, efs2BootModeNoBoot)

	// Sending same command from EC should always succeed to handle failed and retried transactions.
	s.Log("EC sends NO_BOOT again and should succeed")
	checkSendEcPacket(ctx, s, b, ecUart, setNoBoot, efs2ReturnSuccess)
	checkApBootMode(ctx, s, tpm, efs2BootModeNoBoot)

	s.Log("EC sends Verified as Mode. Should be rejected")
	setVerified := createEcPacket(efs2CmdSetBootMode, []byte{efs2BootModeVerified})
	checkSendEcPacket(ctx, s, b, ecUart, setVerified, efs2ReturnErrorBadParm)
	checkApBootMode(ctx, s, tpm, efs2BootModeNoBoot)

	events := b.GpioMonitorFinish(ctx, gpioMonitor)
	s.Log("Stop gpio monitoring: ", events)
	if len(events.Sorted) != 0 {
		s.Error("There should be no EC_RST_L events")
	}

	s.Log("EC sends TrustedRO as Mode. Should be Trigger EC reset")
	setTrustedRo := createEcPacket(efs2CmdSetBootMode, []byte{efs2BootModeTrustedRo})

	s.Log("Start gpio monitoring to ensure EC resets with trying to set TrustedRO")
	gpioMonitor = b.GpioMonitorStart(ctx, ti50.GpioTi50EcRstL)
	if gpioMonitor.InitialValues[ti50.GpioTi50EcRstL] != true {
		s.Error("EC_RST_L not de-asserted before when starting monitoring")
	}
	sendEcPacketNoResponse(ctx, s, b, ecUart, setTrustedRo)
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
	checkApBootMode(ctx, s, tpm, efs2BootModeTrustedRo)
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

	s.Log("Setting up kernel file with random ec hash: ", hash)
	kernelFile := makeKernelFile(hash)
	// Define space in NV storage and clean up afterwards or subsequent runs will fail.
	if err := tpm2.NVDefineSpace(tpm,
		ti50.RootPlatformHandle,
		ti50.KernelFileID,
		ti50.EmptyPassword,
		ti50.EmptyPassword,
		nil,
		ti50.KernelFileAttr,
		uint16(len(kernelFile)),
	); err != nil {
		s.Fatal("NVDefineSpace failed: ", err)
	}
	defer tpm2.NVUndefineSpace(tpm, ti50.EmptyPassword, ti50.RootPlatformHandle, ti50.KernelFileID)

	// Write the kernel file data to new space.
	if err := tpm2.NVWrite(tpm,
		ti50.RootPlatformHandle,
		ti50.KernelFileID,
		ti50.EmptyPassword,
		kernelFile,
		0,
	); err != nil {
		s.Fatal("NVWrite failed: ", err)
	}

	s.Log("EC sends verify hash and expected success")
	verifyEcPacket := createEcPacket(efs2CmdVerifyHash, hash)
	checkSendEcPacket(ctx, s, b, ecUart, verifyEcPacket, efs2ReturnSuccess)
	checkApBootMode(ctx, s, tpm, efs2BootModeVerified)

	// GSC needs to be able to handle duplicate requests without adverse side affects to handle
	// communication retries.
	s.Log("EC sends same verify hash and expected success with now reboot")
	checkSendEcPacket(ctx, s, b, ecUart, verifyEcPacket, efs2ReturnSuccess)
	checkApBootMode(ctx, s, tpm, efs2BootModeVerified)

	s.Log("EC sends Verified as Mode. Should be rejected")
	setVerified := createEcPacket(efs2CmdSetBootMode, []byte{efs2BootModeVerified})
	checkSendEcPacket(ctx, s, b, ecUart, setVerified, efs2ReturnErrorBadParm)
	checkApBootMode(ctx, s, tpm, efs2BootModeVerified)

	s.Log("EC sends NO_BOOT and should transition from VERIFIED")
	setNoBoot := createEcPacket(efs2CmdSetBootMode, []byte{efs2BootModeNoBoot})
	checkSendEcPacket(ctx, s, b, ecUart, setNoBoot, efs2ReturnSuccess)
	checkApBootMode(ctx, s, tpm, efs2BootModeNoBoot)

	events := b.GpioMonitorFinish(ctx, gpioMonitor)
	s.Log("Stop gpio monitoring: ", events)
	if len(events.Sorted) != 0 {
		s.Error("There should be no EC_RST_L events")
	}

	s.Log("EC sends TrustedRO as Mode. Should be Trigger EC reset")
	setTrustedRo := createEcPacket(efs2CmdSetBootMode, []byte{efs2BootModeTrustedRo})
	gpioMonitor = b.GpioMonitorStart(ctx, ti50.GpioTi50EcRstL)
	if gpioMonitor.InitialValues[ti50.GpioTi50EcRstL] != true {
		s.Error("EC_RST_L not de-asserted before when starting monitoring")
	}
	sendEcPacketNoResponse(ctx, s, b, ecUart, setTrustedRo)
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
	checkApBootMode(ctx, s, tpm, efs2BootModeTrustedRo)
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

	s.Log("Setting up kernel file with random ec hash: ", hash)
	kernelFile := makeKernelFile(hash)
	// Define space in NV storage and clean up afterwards or subsequent runs will fail.
	if err := tpm2.NVDefineSpace(tpm,
		ti50.RootPlatformHandle,
		ti50.KernelFileID,
		ti50.EmptyPassword,
		ti50.EmptyPassword,
		nil,
		ti50.KernelFileAttr,
		uint16(len(kernelFile)),
	); err != nil {
		s.Fatal("NVDefineSpace failed: ", err)
	}
	defer tpm2.NVUndefineSpace(tpm, ti50.EmptyPassword, ti50.RootPlatformHandle, ti50.KernelFileID)

	// Write the kernel file data to new space.
	if err := tpm2.NVWrite(tpm,
		ti50.RootPlatformHandle,
		ti50.KernelFileID,
		ti50.EmptyPassword,
		kernelFile,
		0,
	); err != nil {
		s.Fatal("NVWrite failed: ", err)
	}

	s.Log("EC sends verify hash with wrong hash and expect automatic transition to NO_BOOT")
	// Flip the all the bits in the first hash byte from what is stored in kernel file.
	hash[0] ^= 0xFF
	verifyEcPacket := createEcPacket(efs2CmdVerifyHash, hash)
	checkSendEcPacket(ctx, s, b, ecUart, verifyEcPacket, efs2ReturnErrorBadPayload)
	checkApBootMode(ctx, s, tpm, efs2BootModeNoBoot)

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
	badCrc := createEcPacket(efs2CmdVerifyHash, hash)
	badCrc[len(badCrc)-1] ^= 0xFF
	checkSendEcPacket(ctx, s, b, ecUart, badCrc, efs2ReturnErrorCrc)
	checkApBootMode(ctx, s, tpm, efs2BootModeTrustedRo)

	s.Log("EC sends unknown command and expects error")
	badCmd := createEcPacket(efs2Cmd(78), []byte{})
	checkSendEcPacket(ctx, s, b, ecUart, badCmd, efs2ReturnErrorUndefinedCmd)
	checkApBootMode(ctx, s, tpm, efs2BootModeTrustedRo)

	s.Log("EC sends bad version and expects error")
	badVersion := createEcPacket(efs2CmdVerifyHash, []byte{})
	// Increment the version field which should be 5 byte from the end.
	badVersion[len(badVersion)-5]++
	checkSendEcPacket(ctx, s, b, ecUart, badVersion, efs2ReturnErrorVersion)
	checkApBootMode(ctx, s, tpm, efs2BootModeTrustedRo)

	s.Log("EC sends bad magic and expects error")
	badMagic := createEcPacket(efs2CmdVerifyHash, []byte{})
	// Increment the second magic field which should be 6 byte from the end.
	badMagic[len(badMagic)-6]++
	checkSendEcPacket(ctx, s, b, ecUart, badMagic, efs2ReturnErrorMagic)
	checkApBootMode(ctx, s, tpm, efs2BootModeTrustedRo)

	s.Log("EC sends too small verified hash and expects error")
	badSizeVerified := createEcPacket(efs2CmdVerifyHash, []byte{1, 2})
	checkSendEcPacket(ctx, s, b, ecUart, badSizeVerified, efs2ReturnErrorSize)
	checkApBootMode(ctx, s, tpm, efs2BootModeTrustedRo)

	s.Log("EC sends too large verified hash size and expects error")
	badSizeVerified2 := createEcPacket(efs2CmdVerifyHash, make([]byte, 30))
	checkSendEcPacket(ctx, s, b, ecUart, badSizeVerified2, efs2ReturnErrorSize)
	checkApBootMode(ctx, s, tpm, efs2BootModeTrustedRo)

	s.Log("EC sends too small set boot mode and expects error")
	badSizeBootMode := createEcPacket(efs2CmdSetBootMode, []byte{})
	checkSendEcPacket(ctx, s, b, ecUart, badSizeBootMode, efs2ReturnErrorSize)
	checkApBootMode(ctx, s, tpm, efs2BootModeTrustedRo)

	s.Log("EC sends too large set boot mode and expects error")
	badSizeBootMode2 := createEcPacket(efs2CmdSetBootMode, make([]byte, 2))
	checkSendEcPacket(ctx, s, b, ecUart, badSizeBootMode2, efs2ReturnErrorSize)
	checkApBootMode(ctx, s, tpm, efs2BootModeTrustedRo)

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

	s.Log("Setting up kernel file with random ec hash: ", hash)
	kernelFile := makeKernelFile(hash)
	// Define space in NV storage and clean up afterwards or subsequent runs will fail.
	if err := tpm2.NVDefineSpace(tpm,
		ti50.RootPlatformHandle,
		ti50.KernelFileID,
		ti50.EmptyPassword,
		ti50.EmptyPassword,
		nil,
		ti50.KernelFileAttr,
		uint16(len(kernelFile)),
	); err != nil {
		s.Fatal("NVDefineSpace failed: ", err)
	}
	defer tpm2.NVUndefineSpace(tpm, ti50.EmptyPassword, ti50.RootPlatformHandle, ti50.KernelFileID)

	// Write the kernel file data to new space.
	if err := tpm2.NVWrite(tpm,
		ti50.RootPlatformHandle,
		ti50.KernelFileID,
		ti50.EmptyPassword,
		kernelFile,
		0,
	); err != nil {
		s.Fatal("NVWrite failed: ", err)
	}

	s.Log("EC sends verify hash and expects success")
	// Flip the all the bits in the first hash byte from what is stored in kernel file.
	verifyEcPacket := createEcPacket(efs2CmdVerifyHash, hash)
	checkSendEcPacket(ctx, s, b, ecUart, verifyEcPacket, efs2ReturnSuccess)
	checkApBootMode(ctx, s, tpm, efs2BootModeVerified)

	s.Log("AP changes kernel file without EC reset")
	// Change first byte of hash and re-write file.
	hash[0] ^= 0xFF
	kernelFile = makeKernelFile(hash)
	if err := tpm2.NVWrite(tpm,
		ti50.RootPlatformHandle,
		ti50.KernelFileID,
		ti50.EmptyPassword,
		kernelFile,
		0,
	); err != nil {
		s.Fatal("NVWrite failed: ", err)
	}

	s.Log("EC sends verify hash with old hash and expects failure")
	checkSendEcPacket(ctx, s, b, ecUart, verifyEcPacket, efs2ReturnErrorBadPayload)
	checkApBootMode(ctx, s, tpm, efs2BootModeNoBoot)

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

	checkApBootMode(ctx, s, tpm, efs2BootModeTrustedRo)

	s.Log("EC sends packet with too short of preamble to set NO_BOOT. Should have no affect")
	tooShortPreamble := createEcPacket(efs2CmdSetBootMode, []byte{efs2BootModeNoBoot})
	tooShortPreamble = tooShortPreamble[len(efs2Preamble)/2:]
	sendEcPacketNoResponse(ctx, s, b, ecUart, tooShortPreamble)
	checkApBootMode(ctx, s, tpm, efs2BootModeTrustedRo)

	s.Log("EC sends packet with long preamble to set NO_BOOT. Should work")
	longPreamble := createEcPacket(efs2CmdSetBootMode, []byte{efs2BootModeNoBoot})
	longPreamble = append(efs2Preamble, longPreamble...)
	longPreamble = append(efs2Preamble, longPreamble...)
	longPreamble = append(efs2Preamble, longPreamble...)
	checkSendEcPacket(ctx, s, b, ecUart, longPreamble, efs2ReturnSuccess)
	checkApBootMode(ctx, s, tpm, efs2BootModeNoBoot)

	events := b.GpioMonitorFinish(ctx, gpioMonitor)
	s.Log("Stop gpio monitoring: ", events)
	if len(events.Sorted) != 0 {
		s.Error("There should be no EC_RST_L events")
	}
}
