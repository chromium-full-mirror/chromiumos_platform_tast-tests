// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/usbutils"
	"go.chromium.org/tast-tests/cros/remote/powercontrol"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type peripheralsPowerMode int

const (
	suspendTest peripheralsPowerMode = iota
	coldbootTest
)

type peripheralsTestParams struct {
	powerMode   peripheralsPowerMode
	iter        int
	checkSDCard bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         SystemPeripheralsFunctionalityCheck,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies connected peripherals detection before and after power operations",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291", // ChromeOS > External > Intel
		SoftwareDeps: []string{"chrome", "reboot"},
		ServiceDeps:  []string{"tast.cros.security.BootLockboxService"},
		Vars:         []string{"servo", "power.sd_card_present"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.InternalDisplay()),
		Params: []testing.Param{{
			Name:    "suspend_quick",
			Val:     peripheralsTestParams{powerMode: suspendTest, iter: 1, checkSDCard: true},
			Timeout: 5 * time.Minute,
		}, {
			Name:      "suspend_bronze",
			Val:       peripheralsTestParams{powerMode: suspendTest, iter: 20, checkSDCard: true},
			Timeout:   8 * time.Minute,
			ExtraAttr: []string{"group:intel-stability-bronze"},
		}, {
			Name:      "suspend_silver",
			Val:       peripheralsTestParams{powerMode: suspendTest, iter: 50, checkSDCard: true},
			Timeout:   13 * time.Minute,
			ExtraAttr: []string{"group:intel-stability-silver"},
		}, {
			Name:      "suspend_gold",
			Val:       peripheralsTestParams{powerMode: suspendTest, iter: 100, checkSDCard: true},
			Timeout:   20 * time.Minute,
			ExtraAttr: []string{"group:intel-stability-gold"},
		}, {
			Name:    "coldboot_quick",
			Val:     peripheralsTestParams{powerMode: coldbootTest, iter: 1, checkSDCard: true},
			Timeout: 5 * time.Minute,
		}, {
			Name:      "coldboot_bronze",
			Val:       peripheralsTestParams{powerMode: coldbootTest, iter: 20, checkSDCard: true},
			Timeout:   25 * time.Minute,
			ExtraAttr: []string{"group:intel-stability-bronze"},
		}, {
			Name:      "coldboot_silver",
			Val:       peripheralsTestParams{powerMode: coldbootTest, iter: 50, checkSDCard: true},
			Timeout:   45 * time.Minute,
			ExtraAttr: []string{"group:intel-stability-silver"},
		}, {
			Name:      "coldboot_gold",
			Val:       peripheralsTestParams{powerMode: coldbootTest, iter: 100, checkSDCard: true},
			Timeout:   85 * time.Minute,
			ExtraAttr: []string{"group:intel-stability-gold"},
		},
		}})
}

// SystemPeripheralsFunctionalityCheck verifies connected peripherals is detected
// or not while performing power mode operations.
// Pre-requisite: Below devices need to be connected to DUT before executing test.
// 1. USB2.0
// 2. USB3.0
// 3. SD_CARD
// 4. External HDMI display
func SystemPeripheralsFunctionalityCheck(ctx context.Context, s *testing.State) {
	ctxForCleanUp := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Minute)
	defer cancel()

	dut := s.DUT()
	testParam := s.Param().(peripheralsTestParams)

	sdCardVar := testParam.checkSDCard
	if sdCard, ok := s.Var("power.sd_card_present"); ok {
		checkSDcard, err := strconv.ParseBool(sdCard)
		if err != nil {
			s.Fatalf("Failed to convert value of 'power.sd_card_present' %q to bool: %v", sdCard, err)
		}
		sdCardVar = checkSDcard
	}

	servoSpec := s.RequiredVar("servo")
	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(ctxForCleanUp)

	defer func(ctx context.Context) {
		if !dut.Connected(ctx) {
			if err := powercontrol.PowerOntoDUT(ctx, pxy, dut); err != nil {
				s.Fatal("Failed to power-on DUT at cleanup: ", err)
			}
		}
	}(ctxForCleanUp)

	// Perform initial Chrome login.
	if err := powercontrol.ChromeOSLogin(ctx, dut, s.RPCHint()); err != nil {
		s.Fatal("Failed to log in to Chrome: ", err)
	}

	// Check for all peripheral devices detection before suspend/cold boot.
	if err := connectedPeripheralsDetection(ctx, dut, sdCardVar); err != nil {
		s.Fatal("Failed to detect connected peripherals devices before cold boot: ", err)
	}

	iter := testParam.iter
	switch testParam.powerMode {
	case suspendTest:
		for i := 1; i <= iter; i++ {
			s.Logf("Suspend test iteration: %d/%d", i, iter)
			if err := performDUTSuspend(ctx, pxy, dut); err != nil {
				s.Fatal("Failed to perform suspend and wake DUT: ", err)
			}

			// Check for all peripheral devices detection after suspend.
			if err := connectedPeripheralsDetection(ctx, dut, sdCardVar); err != nil {
				s.Fatal("Failed to detect connected peripherals devices during suspend test: ", err)
			}
		}

	case coldbootTest:
		for i := 1; i <= iter; i++ {
			s.Logf("Cold boot test iteration: %d/%d", i, iter)
			powerState := "S5"
			if err := powercontrol.ShutdownAndWaitForPowerState(ctx, pxy, dut, powerState); err != nil {
				s.Fatalf("Failed to shutdown and wait for %q powerstate: %v", powerState, err)
			}

			if err := powercontrol.PowerOntoDUT(ctx, pxy, dut); err != nil {
				s.Fatal("Failed to power on DUT: ", err)
			}

			// Performing chrome login after powering on DUT from cold boot.
			if err := powercontrol.ChromeOSLogin(ctx, dut, s.RPCHint()); err != nil {
				s.Fatal("Failed to log in to Chrome after cold boot: ", err)
			}

			// Check for all peripheral devices detection after cold boot.
			if err := connectedPeripheralsDetection(ctx, dut, sdCardVar); err != nil {
				s.Fatal("Failed to detect connected peripherals devices after cold boot: ", err)
			}

			valid, err := powercontrol.IsPrevSleepStateAvailable(ctx, dut)

			if err != nil {
				s.Fatal("Failed to determine if prev_sleep_state is available: ", err)
			}

			if valid {
				// Performing prev_sleep_state check.
				expectedPrevSleepState := 5
				if err := powercontrol.ValidatePrevSleepState(ctx, dut, expectedPrevSleepState); err != nil {
					s.Fatal("Failed to validate previous sleep state: ", err)
				}
			}
		}
	}
}

// sdCardDetection performs SD card detection validation.
func sdCardDetection(ctx context.Context, dut *dut.DUT) error {
	const sdMmcSpecFile = "/sys/kernel/debug/mmc0/ios"
	sdCardSpecRe := regexp.MustCompile(`timing spec:.[0-9]+.\((?:sd|mmc).*`)
	return testing.Poll(ctx, func(ctx context.Context) error {
		isSDCardConnected := sdCardConnected(ctx, dut)
		if !isSDCardConnected {
			return errors.New("failed to find SD card")
		}
		sdCardSpecOut, err := linuxssh.ReadFile(ctx, dut.Conn(), sdMmcSpecFile)
		if err != nil {
			return errors.Wrap(err, "failed to execute sd card /sys/kernel' command")
		}
		if got := string(sdCardSpecOut); !sdCardSpecRe.MatchString(got) {
			return errors.Errorf("failed to get MMC spec info in /sys/kernel/ = got %q, want match %q", got, sdCardSpecRe)
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second})
}

// usbStorageDevicesDetection verifies whether connected USB storage device
// detected or not.
func usbStorageDevicesDetection(ctx context.Context, dut *dut.DUT) error {
	usbStorageClassName := "Mass Storage"
	usb2DeviceSpeed := "480M"
	usb3DeviceSpeed := "5000M"

	// Check for USB device(s) detection after cold boot.
	usbDevicesList, err := usbutils.ListDevicesInfo(ctx, dut)
	if err != nil {
		return errors.Wrap(err, "failed to get USB devices list")
	}

	usbDevicesSpeed := []string{usb2DeviceSpeed, usb3DeviceSpeed}
	for _, deviceSpeeed := range usbDevicesSpeed {
		got := usbutils.NumberOfUSBDevicesConnected(usbDevicesList, usbStorageClassName, deviceSpeeed)
		if want := 1; got != want {
			return errors.Errorf("unexpected number of USB devices connected with %q speed: got %d, want %d", deviceSpeeed, got, want)
		}
	}
	return nil
}

// connectedPeripheralsDetection verified whether all connected peripheral devices
// detected or not.
func connectedPeripheralsDetection(ctx context.Context, dut *dut.DUT, sdCardVar bool) error {
	if err := usbStorageDevicesDetection(ctx, dut); err != nil {
		return errors.Wrap(err, "failed to detect connected USB storage devices")
	}
	if sdCardVar {
		if err := sdCardDetection(ctx, dut); err != nil {
			return errors.Wrap(err, "failed to detect connected SD Card")
		}
	}
	numberOfDisplay := 1
	spec := usbutils.DisplaySpec{
		NumberOfDisplays: &numberOfDisplay,
		DisplayType:      usbutils.NativeHDMI,
	}
	if err := usbutils.ExternalDisplayDetectionForRemote(ctx, dut, spec); err != nil {
		spec := usbutils.DisplaySpec{
			NumberOfDisplays: &numberOfDisplay,
			DisplayType:      "TypeCHDMI",
		}
		if err := usbutils.ExternalDisplayDetectionForRemote(ctx, dut, spec); err != nil {
			return errors.Wrap(err, "failed to detect external HDMI display")
		}
	}
	return nil
}

// sdCardConnected return SD card detection status.
func sdCardConnected(ctx context.Context, dut *dut.DUT) bool {
	sdFound := false
	sysOut, err := dut.Conn().CommandContext(ctx, "ls", "/sys/block").Output()
	if err != nil {
		return sdFound
	}
	stringOut := strings.TrimSpace(string(sysOut))
	sc := bufio.NewScanner(strings.NewReader(stringOut))
	for sc.Scan() {
		sysBlockFile := fmt.Sprintf("/sys/block/%s/device/type", sc.Text())
		sdOut, err := dut.Conn().CommandContext(ctx, "cat", sysBlockFile).Output()
		if err == nil {
			if strings.TrimSpace(string(sdOut)) == "SD" {
				sdFound = true
				break
			}
		}
	}
	return sdFound
}

// performDUTSuspend performs DUT suspend with 'powerd_dbus_suspend' command and
// wakes DUT with servo power key press.
func performDUTSuspend(ctx context.Context, pxy *servo.Proxy, dut *dut.DUT) error {
	powerOffCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := dut.Conn().CommandContext(powerOffCtx, "powerd_dbus_suspend").Run(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return errors.Wrap(err, "failed to power off DUT")
	}

	sdCtx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	if err := dut.WaitUnreachable(sdCtx); err != nil {
		return errors.Wrap(err, "failed to wait for unreachable")
	}

	if err := powercontrol.PowerOntoDUT(ctx, pxy, dut); err != nil {
		return errors.Wrap(err, "failed to power on DUT")
	}
	return nil
}
