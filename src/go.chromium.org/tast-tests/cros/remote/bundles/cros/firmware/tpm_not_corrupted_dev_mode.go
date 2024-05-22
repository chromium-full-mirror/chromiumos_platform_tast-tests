// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/4lon/crc8"
	"github.com/golang/protobuf/ptypes/empty"
	"golang.org/x/exp/slices"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	// The definitions of KERNEL_ANTIROLLBACK_SPACE_BYTES, KERNEL_NV_INDEX, and
	// TPM_NVRAM_EXPECTED_VALUES are located in third_party/coreboot/src/security/vboot/antirollback.h.
	// These definitions were originally part of the rollback_index module in the platform/vboot_reference,
	// which has been split into a bottom half and top half. The top half contains generic code which hides
	// the underlying storage implementation, and the bottom half implements the storage abstraction.
	// With this change, the bottom half is moved to coreboot, while the top half stays in vboot_reference.
	// (https://chromium-review.googlesource.com/c/chromiumos/third_party/coreboot/+/206065)
	kernelAntirollbackSpaceBytes    = 0xd
	kernelAntirollbackSpaceBytesV10 = 0x28
	kernelNvIndex                   = 0x1008

	// TPM kernel data format version 1.0
	//   byte[0]: version. bit[7:4]=Major, bit[3:0]=Minor.
	//   byte[1]: byte size. 0x28, 40(0x28) bytes
	//   byte[2]: crc8
	//   byte[3]: flag
	//   byte[7-4]: kernel version. 0x00010001
	//   byte[39:8]: ECFW hash in sha256.
	tpmNVRAMV10Header    = "10 28 "
	tpmNVRAMV10CRCOffset = 2
)

// TODO(seancarpenter): This test intends to check whether the kernel version
// stored in the TPM's NVRAM is corrupted during a transition from normal
// mode to dev mode (according to it's objective in an internal tool named
// test tracker).  Figure out why a specific kernel version is checked for,
// and take a look at not requiring a specific kernel version. Examine
// transitioning to crossystem for the normal mode read of the kernel
// version (e.g. look at things like whether a successful exit code from
// a crossystem reading of the kernel version can establish whether the
// kernel version starts in a valid state).
var tpmNVRAMExpectedValues = []string{
	"1 4c 57 52 47 1 0 1 0 0 0 0 0", "2 4c 57 52 47 1 0 1 0 0 0 0 55",
	"2 4c 57 52 47 0 0 0 0 0 0 0 e8",
}

func init() {
	testing.AddTest(&testing.Test{
		Func: TPMNotCorruptedDevMode,
		Desc: "Ensure kernel and fw version in TPM isn't corrupted in dev mode",
		Contacts: []string{
			"chromeos-faft@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:firmware", "firmware_unstable", "firmware_usb"},
		Timeout:      2 * time.Hour,
		SoftwareDeps: []string{"crossystem"},
		ServiceDeps:  []string{"tast.cros.firmware.TPMService"},
		Vars:         []string{"firmware.skipFlashUSB"},
		Fixture:      fixture.DevMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
	})
}

// TPMNotCorruptedDevMode verifies that the kernel anti-rollback info stored
// in the TPM NVRAM, and then boots to USB and checks the firmware version
// and kernel version via crossystem for corruption.
func TPMNotCorruptedDevMode(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	if err := checkTPMC(ctx, h); err != nil {
		s.Fatal("Failed to check TPMC: ", err)
	}

	s.Log("Setup USB key")
	skipFlashUSB := false
	if skipFlashUSBStr, ok := s.Var("firmware.skipFlashUSB"); ok {
		var err error
		skipFlashUSB, err = strconv.ParseBool(skipFlashUSBStr)
		if err != nil {
			s.Fatalf("Invalid value for var firmware.skipFlashUSB: got %q, want true/false", skipFlashUSBStr)
		}
	}

	var cs *testing.CloudStorage
	if !skipFlashUSB {
		cs = s.CloudStorage()
	}
	if err := h.SetupUSBKey(ctx, cs); err != nil {
		s.Fatal("USBKey not working: ", err)
	}

	if err := h.EnableDevBootUSB(ctx); err != nil {
		s.Fatal("Failed to enable USB boot: ", err)
	}

	s.Log("Removing the USB")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
		s.Fatal("Failed to remove the USB: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 4*time.Minute)
	defer cancel()
	defer func(ctx context.Context) {
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Fatal("Failed to reconnect to dut: ", err)
		}
		if err := h.DisableDevBootUSB(ctx); err != nil {
			s.Fatal("Failed to set crossystem dev_boot_usb to 0: ", err)
		}
		if err := h.DUT.Conn().CommandContext(ctx, "crossystem", "dev_default_boot=disk").Run(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to set crossystem dev_default_boot to disk: ", err)
		}
	}(cleanupCtx)

	s.Log("Rebooting dut by warm reset")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateWarmReset); err != nil {
		s.Fatal("Failed to warm reset dut: ", err)
	}

	s.Logf("Sleeping for %s (FirmwareScreen) ", h.Config.FirmwareScreen)
	// GoBigSleepLint: Delay to wait for the firmware screen during boot-up.
	if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
		s.Fatalf("Failed to sleep for %s: %v", h.Config.FirmwareScreen, err)
	}

	s.Log("Resetting firmware screen timeout")
	if err := h.Servo.PressKey(ctx, " ", servo.DurTab); err != nil {
		s.Fatal("Failed to press space key: ", err)
	}

	s.Log("Setting DFP mode")
	if err := h.Servo.SetDUTPDDataRole(ctx, servo.DFP); err != nil {
		s.Logf("Failed to set pd data role to DFP: %.400s", err)
	}

	s.Log("Inserting a valid USB to DUT")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
		s.Fatal("Failed to insert USB to DUT: ", err)
	}

	// GoBigSleepLint: It may take some time for usb mux state to
	// take effect.
	if err := testing.Sleep(ctx, firmware.UsbVisibleTime); err != nil {
		s.Fatalf("Failed to sleep for %v s: %v", firmware.UsbDisableTime, err)
	}

	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Creating mode switcher: ", err)
	}
	if err := ms.BypassDevBootUSB(ctx); err != nil {
		s.Fatal("Failed to bypass dev boot USB: ", err)
	}

	if err := h.WaitDUTConnectDuringBootFromUSB(ctx, true); err != nil {
		s.Fatal("Failed to boot from USB: ", err)
	}

	if err := readKernFwVer(ctx, h); err != nil {
		s.Fatal("Failed to read kernel and firmware version: ", err)
	}

}

// readTPMC executes a tpmc command using provided arguments and returns the output.
// It also stops the TPM daemon and restarts it after executing the command.
func readTPMC(ctx context.Context, h *firmware.Helper, tpmReadArgs ...string) (out string, reterr error) {
	cleanupCtx := ctx
	// Reserve a longer time to ensure enough time to restore the TPM daemon.
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Minute)
	defer cancel()

	if err := h.RequireTPMServiceClient(ctx); err != nil {
		return "", errors.Wrap(err, "failed to create TPM service client")
	}

	if _, err := h.TPMServiceClient.NewHelper(ctx, &empty.Empty{}); err != nil {
		return "", errors.Wrap(err, "failed to create a TPM service helper")
	}
	defer func(ctx context.Context) {
		if _, err := h.TPMServiceClient.CloseHelper(ctx, &empty.Empty{}); err != nil {
			if reterr != nil {
				testing.ContextLog(ctx, "Failed to close TPM service helper")
			} else {
				reterr = errors.Wrap(err, "failed to close TPM service helper")
			}
		}
	}(cleanupCtx)

	if _, err := h.TPMServiceClient.StopDaemons(ctx, &empty.Empty{}); err != nil {
		return "", errors.Wrap(err, "failed to stop TPM daemons")
	}
	defer func(ctx context.Context) {
		if _, err := h.TPMServiceClient.StartDaemons(ctx, &empty.Empty{}); err != nil {
			if reterr != nil {
				testing.ContextLog(ctx, "Failed to restart TPM daemons")
			} else {
				reterr = errors.Wrap(err, "failed to restart TPM daemons")
			}
		}
	}(cleanupCtx)

	testing.ContextLog(ctx, "Read the kernel anti-rollback space from the TPM")
	cmd := h.DUT.Conn().CommandContext(ctx, "tpmc", tpmReadArgs...)
	kernelRollbackSpace, err := cmd.Output(ssh.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "failed to read kernel anti-rollback info")
	}

	testing.ContextLogf(ctx, "TPMC OUTPUT: %s", kernelRollbackSpace)

	return string(kernelRollbackSpace), nil
}

// checkTPMC verifies that the kernel anti-rollback data from the tpmc output
// is one of the expected values.
func checkTPMC(ctx context.Context, h *firmware.Helper) error {
	kernelRollbackSpace, err := readTPMC(ctx, h, "read", strconv.FormatInt(kernelNvIndex, 16), strconv.FormatInt(kernelAntirollbackSpaceBytes, 16))
	if err != nil {
		return errors.Wrap(err, "failed to read kernel anti-rollback info")
	}

	// Check that there are no extra lines of output beyond the expected single line of TPM data.
	lines := strings.Split(kernelRollbackSpace, "\n")
	if len(lines[1]) != 0 {
		return errors.Errorf("unexpected additional data beyond the first line of TPM output: %s", kernelRollbackSpace)
	}
	tpmcOutput := lines[0]

	// Check if TPM kernel data is in the format of version 1.0.
	if strings.HasPrefix(tpmcOutput, tpmNVRAMV10Header) {
		return checkTPMCV10(ctx, h)
	}

	if !slices.Contains(tpmNVRAMExpectedValues, tpmcOutput) {
		return errors.Errorf("TPM data is not expected: %s", tpmcOutput)
	}

	return nil
}

func checkTPMCV10(ctx context.Context, h *firmware.Helper) error {
	kernelRollbackSpace, err := readTPMC(ctx, h, "read", strconv.FormatInt(kernelNvIndex, 16), strconv.FormatInt(kernelAntirollbackSpaceBytesV10, 16))
	if err != nil {
		return errors.Wrap(err, "failed to read kernel anti-rollback info")
	}

	tpmcOutput := string(kernelRollbackSpace)
	// Check that there are no extra lines of output beyond the expected single line of TPM data.
	if out := strings.Split(tpmcOutput, "\n"); len(out[1]) > 0 {
		return errors.Errorf("unexpected additional data beyond the first line of TPM output: %s", kernelRollbackSpace)
	}
	kernelData := strings.Fields(tpmcOutput)

	// Check if kernel data is long enough to check CRC.
	if len(kernelData) <= tpmNVRAMV10CRCOffset {
		return errors.New("kernel data is too short for CRC check")
	}

	// Convert kernel data to a byte slice.
	kernelDataBytes := make([]byte, len(kernelData))
	for i, k := range kernelData {
		v, err := strconv.ParseUint(k, 16, 8)
		if err != nil {
			return errors.Wrapf(err, "failed to parse kernel data %q as hex", k)
		}
		kernelDataBytes[i] = byte(v)
	}

	// For all the bytes from offset 3, calculate CRC8.
	crc8Calc := checkSum(kernelDataBytes[tpmNVRAMV10CRCOffset+1:])
	if kernelDataBytes[tpmNVRAMV10CRCOffset] != crc8Calc {
		return errors.Wrapf(err, "kernel Data CRC(%x) is not correct: should be %x", kernelDataBytes[tpmNVRAMV10CRCOffset], crc8Calc)
	}
	testing.ContextLog(ctx, "Kernel data v1.0 CRC is checked")

	return nil
}

// readKernFwVer checks the firmware and kernel version reported by crossystem.
func readKernFwVer(ctx context.Context, h *firmware.Helper) error {
	testing.ContextLog(ctx, "Checking kernel and fw version via crossystem")
	checkVals := map[reporters.CrossystemParam]string{
		reporters.CrossystemParamTpmKernelVer: "0xFFFFFFFF",
		reporters.CrossystemParamTpmFwVer:     "0xFFFFFFFF",
	}
	if matched, err := h.Reporter.CrossystemChecker(ctx, checkVals); err != nil {
		return errors.Wrap(err, "failed to verify crossystem params")
	} else if matched {
		return errors.New("found invalid kernel and firmware versions stored in the TPM")
	}
	return nil
}

func checkSum(data []byte) byte {
	table := crc8.MakeTable(crc8.CRC8)
	crc := crc8.Checksum([]byte(data), table)
	return crc
}
