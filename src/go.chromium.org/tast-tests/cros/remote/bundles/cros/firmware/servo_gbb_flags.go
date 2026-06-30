// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"

	common "go.chromium.org/tast-tests/cros/common/firmware"
	commonbios "go.chromium.org/tast-tests/cros/common/firmware/bios"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware/bios"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    ServoGBBFlags,
		Desc:    "Verifies GBB flags state can be obtained and manipulated via the servo CCD interface",
		Timeout: 8 * time.Minute,
		Contacts: []string{
			"chromeos-faft@google.com",
			"jbettis@chromium.org",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		TestBedDeps:  append([]string{tbdep.ServoComponent("ccd_gsc")}, tbdep.ServoPresentAndWorking...),
		Attr:         []string{"group:firmware", "firmware_cr50", "firmware_bios", "firmware_enabled", "firmware_meets_kpi", "firmware_stressed"},
		SoftwareDeps: []string{"flashrom"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.SkipOnModel(
			// b/111215677: CCD servo detection doesn't work on soraka.
			"soraka",
			// b/350805894: Fizz devices don't have functional CCD.
			"jax", "kench", "sion", "talon", "teemo", "twitch", "wukong",
		)),
	})
}

// TODO(b/297577280): Add a CCD variant to ServoGBBFlagsFutility and remove this test.

// ServoGBBFlags has been tested to pass with Suzy-Q, Servo V4, Servo V4 + ServoMicro in dual V4 mode.
// Verified fail on Servo V4 + ServoMicro w/o dual v4 mode.
// Has not been tested with with C2D2 (assumed to pass).
func ServoGBBFlags(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequirePlatform(ctx); err != nil {
		s.Fatal("Failed to require platform: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to require config: ", err)
	}
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	if err := h.Servo.RequireCCD(ctx); err != nil {
		s.Fatal("Servo does not have CCD: ", err)
	}

	// Reenable the main servo device when the test ends
	defer h.Servo.PreferDebugHeader(ctx)

	if val, err := h.Servo.GetString(ctx, servo.GSCCCDLevel); err != nil {
		s.Fatal("Failed to get gsc_ccd_level")
	} else if val != servo.Open {
		s.Logf("CCD is not open, got %q. Attempting to unlock", val)
		if err := h.Servo.SetString(ctx, servo.GSCTestlab, servo.Open); err != nil {
			s.Fatal("Failed to unlock CCD")
		}
	}

	ccdSerial := ""
	var servoSPIControl servo.OnOffControl
	devices, err := h.Servo.GetStringList(ctx, servo.Devices)
	if err != nil {
		s.Fatal("Failed to get servo devices: ", err)
	}

	for _, device := range devices {
		stringType, err := servo.PropertyToString(device, "type")
		if err != nil {
			s.Fatal("Failed to get device type property: ", err)
		}
		ccdSerial, err = servo.PropertyToString(device, "serial")
		if err != nil {
			s.Fatal("Failed to get device serial property: ", err)
		}
		if strings.HasPrefix(stringType, "ccd") {
			servoSPIControl = servo.OnOffControl(fmt.Sprintf("%s.%s", stringType, servo.CCDCPUFWSPI))
			break
		}
	}
	if ccdSerial == "" || servoSPIControl == "" {
		s.Fatalf("Failed to find ccd in %+v", devices)
	}
	programmer := fmt.Sprintf("raiden_debug_spi:target=AP,custom_rst=true,serial=%s", ccdSerial)
	s.Logf("Programmer is %s", programmer)

	if err = h.Servo.RemoveCCDWatchdogs(ctx); err != nil {
		s.Fatal("Failed to remove ccd watchdog: ", err)
	}

	s.Log("Getting GBB flags")
	old, err := common.GetGBBFlags(ctx, s.DUT())
	if err != nil {
		s.Fatal("initial GetGBBFlags failed: ", err)
	}
	s.Log("Current GBB flags: ", old.Set)

	s.Log("Reading fw image over CCD")
	h.DisconnectDUT(ctx) // Some of the dutControl commands will reboot
	if err := h.Servo.SetOnOffTimeout(ctx, servoSPIControl, servo.On, 30*time.Second); err != nil {
		s.Fatalf("Failed to enable %v: %+v", servoSPIControl, err)
	}
	img, err := bios.NewRemoteImage(ctx, h.ServoProxy, programmer, commonbios.GBBImageSection, nil)
	if err != nil {
		s.Error("Could not read firmware: ", err)
	}
	if err := h.Servo.SetOnOffTimeout(ctx, servoSPIControl, servo.Off, 30*time.Second); err != nil {
		s.Fatalf("Failed to enable %v: %+v", servoSPIControl, err)
	}
	if img == nil {
		s.Fatal("No firmware image")
	}

	cf, sf, err := getFlagsFromImage(img)
	if err != nil {
		s.Fatal("Could not get GBB flags: ", err)
	}
	ret := pb.GBBFlagsState{Clear: cf, Set: sf}
	s.Log("CDD GBB flags: ", ret.Set)

	sortSlice := cmp.Transformer("Sort", func(in []pb.GBBFlag) []pb.GBBFlag {
		out := append([]pb.GBBFlag(nil), in...)
		sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
		return out
	})
	if !cmp.Equal(old.Set, ret.Set, sortSlice) {
		s.Fatal("GBB flags from CDD do not match SSH'd GBB flags ", cmp.Diff(old.Set, ret.Set, sortSlice))
	}
	powerOnAndWait := func(ctx context.Context) {
		// Switch the active dut controller to servo_micro/C2D2 if present for power on.
		h.Servo.PreferDebugHeader(ctx)

		// GoBigSleepLint: Flashrom usually restarts the dut, but on a few platforms it might require a power press to come back on.
		// But you can't press the power button too soon, or it will be ignored.
		if err := testing.Sleep(ctx, 13*time.Second); err != nil {
			s.Fatal("Pre-power button sleep failed: ", err)
		}
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			state, err := h.Servo.GetECSystemPowerState(ctx)
			if err != nil {
				return errors.Wrap(err, "power state failed")
			}
			if state != "S0" {
				if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOn)); err != nil {
					return errors.Wrap(err, "power button press failed")
				}
			}
			s.Log("Waiting for reboot")
			context.WithTimeout(ctx, 10*time.Second)
			if err := h.WaitConnect(ctx); err != nil {
				return errors.Wrap(err, "connect failed")
			}
			return nil
		}, &testing.PollOptions{
			Timeout: time.Minute,
		}); err != nil {
			s.Fatalf("Failed to connect to DUT: %s", err)
		}
	}
	powerOnAndWait(ctx)

	// We need to change some GBB flag, but it doesn't really matter which.
	// Toggle DEV_SCREEN_SHORT_DELAY
	cf = common.GBBToggle(cf, pb.GBBFlag_DEV_SCREEN_SHORT_DELAY)
	sf = common.GBBToggle(sf, pb.GBBFlag_DEV_SCREEN_SHORT_DELAY)
	if err := setFlagsFromImage(img, cf, sf); err != nil {
		s.Fatal("Failed to toggle GBB flag in image: ", err)
	}

	s.Log("Writing fw image over CCD")
	h.DisconnectDUT(ctx) // Some of the dutControl commands will reboot

	// Switch the active dut controller to CCD
	if err := h.Servo.RequireCCD(ctx); err != nil {
		s.Fatal("Servo does not have CCD: ", err)
	}
	if err := h.Servo.SetOnOff(ctx, servoSPIControl, servo.On); err != nil {
		s.Fatalf("Failed to enable %v: %+v", servoSPIControl, err)
	}
	if err = bios.WriteRemoteFlashrom(ctx, h.ServoProxy, programmer, img, commonbios.GBBImageSection, nil); err != nil {
		s.Error("Failed to write flashrom: ", err)
	}
	if err := h.Servo.SetOnOff(ctx, servoSPIControl, servo.Off); err != nil {
		s.Fatalf("Failed to enable %v: %+v", servoSPIControl, err)
	}

	powerOnAndWait(ctx)

	s.Log("Getting GBB flags")
	newFlags, err := common.GetGBBFlags(ctx, s.DUT())
	if err != nil {
		s.Fatal("final GetGBBFlags failed: ", err)
	}
	s.Log("Updated GBB flags: ", newFlags.Set)
	expected := pb.GBBFlagsState{Clear: cf, Set: sf}
	if !cmp.Equal(expected.Set, newFlags.Set, sortSlice) {
		s.Fatal("Updated GBB flags do not match SSH'd GBB flags ", cmp.Diff(expected.Set, newFlags.Set, sortSlice))
	}
}

func getFlagsFromImage(i *commonbios.Image) ([]pb.GBBFlag, []pb.GBBFlag, error) {
	var gbb uint32
	if err := i.ReadSectionData(commonbios.GBBImageSection, commonbios.GbbHeaderOffset, 4, &gbb); err != nil {
		return nil, nil, err
	}
	setFlags := common.CalcGBBFlags(gbb)
	clearFlags := common.CalcGBBFlags(^gbb)
	return clearFlags, setFlags, nil
}

func setFlagsFromImage(i *commonbios.Image, clearFlags, setFlags []pb.GBBFlag) error {
	var currGBB uint32
	if err := i.ReadSectionData(commonbios.GBBImageSection, commonbios.GbbHeaderOffset, 4, &currGBB); err != nil {
		return err
	}
	newGBB := common.CalcGBBBits(currGBB, common.CalcGBBMask(clearFlags), common.CalcGBBMask(setFlags))
	if newGBB == currGBB {
		// No need to write section data if GBB flags are already correct.
		return nil
	}
	return i.WriteSectionData(commonbios.GBBImageSection, commonbios.GbbHeaderOffset, newGBB)
}
