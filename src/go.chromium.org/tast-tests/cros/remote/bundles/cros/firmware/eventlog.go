// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"regexp"
	"strconv"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// eventLogParams contains all the data needed to run a single test iteration.
type eventLogParams struct {
	resetType        firmware.ResetType
	bootToMode       fwCommon.BootMode
	suspendResume    bool
	hardwareWatchdog bool
	// All of the regexes in one of the sets must be present. Ex.
	// [][]string{[]string{`Case 1A`, `Case 1B`}, []string{`Case 2A`, `Case 2[BC]`}}
	// Any of these events would pass:
	// Case 1A, Case 1B
	// Case 2A, Case 2B
	// Case 2A, Case 2C
	requiredEventSets [][]string
	prohibitedEvents  string
	allowedEvents     string
}

func init() {
	testing.AddTest(&testing.Test{
		Func: Eventlog,
		Desc: "Ensure that eventlog is written on boot and suspend/resume",
		Contacts: []string{
			"chromeos-faft@google.com",
			"jbettis@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_level4"},
		HardwareDeps: hwdep.D(
			// Eventlog is broken/wontfix on veyron devices.
			// See http://b/35585376#comment14 for more info.
			hwdep.SkipOnPlatform("veyron_fievel"),
			hwdep.SkipOnPlatform("veyron_tiger"),
		),
		Requirements: []string{"sys-fw-0021-v01", "sys-fw-0024-v01"},
		SoftwareDeps: []string{"crossystem", "flashrom", "chrome"},
		ServiceDeps:  []string{"tast.cros.firmware.UtilsService"},
		Vars:         []string{"firmware.skipFlashUSB"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{
			// Test eventlog upon normal->normal reboot.
			{
				Name: "normal",
				// Disable on rammus (b/184778308) and coral (b/250684696)
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel("leona", "shyvana", "astronaut", "babymega", "babytiger", "blacktiplte", "nasher", "robo360")),
				Fixture:           fixture.NormalMode,
				Val: eventLogParams{
					resetType:         firmware.WarmReset,
					requiredEventSets: [][]string{{`System boot`}},
					prohibitedEvents:  `Developer Mode|Recovery Mode|Sleep| Wake`,
				},
				ExtraRequirements: []string{"sys-fw-0025-v01"},
			},
			{
				// Allow some normally disallowed events on rammus. b/184778308
				Name:              "rammus_normal",
				ExtraHardwareDeps: hwdep.D(hwdep.Model("leona", "shyvana")),
				Fixture:           fixture.NormalMode,
				Val: eventLogParams{
					resetType:         firmware.WarmReset,
					requiredEventSets: [][]string{{`System boot`}},
					prohibitedEvents:  `Developer Mode|Recovery Mode|Sleep| Wake`,
					allowedEvents:     `^ACPI Wake \| Deep S5$`,
				},
				ExtraRequirements: []string{"sys-fw-0025-v01"},
			},
			// Test eventlog upon dev->dev reboot.
			{
				Name:              "dev",
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel("leona", "shyvana")),
				Fixture:           fixture.DevModeGBB,
				Val: eventLogParams{
					resetType:         firmware.WarmReset,
					requiredEventSets: [][]string{{`System boot`, `Chrome ?OS Developer Mode|boot_mode=Developer`}},
					prohibitedEvents:  `Recovery Mode|Sleep| Wake`,
				},
				ExtraRequirements: []string{"sys-fw-0025-v01"},
			},
			// Allow some normally disallowed events on rammus. b/184778308
			{
				Name:              "rammus_dev",
				ExtraHardwareDeps: hwdep.D(hwdep.Model("leona", "shyvana")),
				Fixture:           fixture.DevModeGBB,
				Val: eventLogParams{
					resetType:         firmware.WarmReset,
					requiredEventSets: [][]string{{`System boot`, `Chrome ?OS Developer Mode|boot_mode=Developer`}},
					prohibitedEvents:  `Recovery Mode|Sleep| Wake`,
					allowedEvents:     `^ACPI Wake \| Deep S5$`,
				},
				ExtraRequirements: []string{"sys-fw-0025-v01"},
			},
			// Test eventlog upon normal->rec reboot.
			{
				Name:      "normal_rec",
				ExtraAttr: []string{"firmware_usb", "firmware_ro"},
				Fixture:   fixture.NormalMode,
				Val: eventLogParams{
					bootToMode:        fwCommon.BootModeRecovery,
					requiredEventSets: [][]string{{`System boot`, `(?i)Chrome ?OS Recovery Mode \| Recovery Button|boot_mode=Manual recovery`}},
					prohibitedEvents:  `Developer Mode|Sleep|FW Wake|ACPI Wake \| S3`,
				},
				Timeout: 60 * time.Minute,
			},
			// Test eventlog upon rec->normal reboot.
			{
				Name:      "rec_normal",
				ExtraAttr: []string{"firmware_usb", "firmware_ro"},
				Fixture:   fixture.RecModeNoServices,
				Val: eventLogParams{
					bootToMode:        fwCommon.BootModeNormal,
					requiredEventSets: [][]string{{`System boot`}},
					prohibitedEvents:  `Developer Mode|Recovery Mode|Sleep`,
				},
				Timeout: 6 * time.Minute,
			},
			// Test eventlog upon suspend/resume w/ default value of suspend_to_idle.
			// treeya: ACPI Enter | S3, EC Event | Power Button, ACPI Wake | S3, Wake Source | Power Button | 0
			// kindred: S0ix Enter, S0ix Exit, Wake Source | Power Button | 0, EC Event | Power Button
			// leona: S0ix Enter, S0ix Exit, Wake Source | Power Button | 0, EC Event | Power Button
			// eldrid: S0ix Enter, S0ix Exit, Wake Source | Power Button | 0, EC Event | Power Button
			// hayato: Sleep, Wake
			{
				Name:    "suspend_resume",
				Fixture: fixture.NormalMode,
				Val: eventLogParams{
					suspendResume: true,
					requiredEventSets: [][]string{
						{`Sleep`, `^Wake`},
						{`ACPI Enter \| S3`, `ACPI Wake \| S3`},
						{`S0ix Enter`, `S0ix Exit`},
					},
					prohibitedEvents: `System |Developer Mode|Recovery Mode`,
				},
				Timeout:           6 * time.Minute,
				ExtraRequirements: []string{"sys-fw-0025-v01"},
			},
			// Test eventlog with hardware watchdog.
			{
				Name:              "watchdog",
				Fixture:           fixture.NormalMode,
				ExtraSoftwareDeps: []string{"watchdog"},
				Val: eventLogParams{
					hardwareWatchdog: true,
					requiredEventSets: [][]string{
						{`System boot|Hardware watchdog reset`},
					},
				},
				ExtraRequirements: []string{"sys-fw-0025-v01"},
			},
		},
	})
}

// eventMessagesContainReMatch returns true if any event's message matches the regexp.
func eventMessagesContainReMatch(ctx context.Context, events []reporters.Event, re *regexp.Regexp) bool {
	for _, event := range events {
		if re.MatchString(event.Message) {
			return true
		}
	}
	return false
}

func Eventlog(ctx context.Context, s *testing.State) {
	// Create mode-switcher.
	v := s.FixtValue().(*fixture.Value)
	h := v.Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servod")
	}
	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Creating mode switcher: ", err)
	}
	r := h.Reporter
	param := s.Param().(eventLogParams)

	if err := h.Reporter.ClearEventlog(ctx); err != nil {
		s.Fatal("Failed to clear event log: ", err)
	}
	if param.resetType != "" {
		if err := ms.ModeAwareReboot(ctx, param.resetType); err != nil {
			s.Fatal("Error resetting DUT: ", err)
		}
	} else if param.bootToMode != "" {
		// If booting into recovery, check the USB Key.
		if param.bootToMode == fwCommon.BootModeRecovery {
			skipFlashUSB := false
			if skipFlashUSBStr, ok := s.Var("firmware.skipFlashUSB"); ok {
				skipFlashUSB, err = strconv.ParseBool(skipFlashUSBStr)
				if err != nil {
					s.Fatalf("Invalid value for var firmware.skipFlashUSB: got %q, want true/false", skipFlashUSBStr)
				}
			}
			cs := s.CloudStorage()
			if skipFlashUSB {
				cs = nil
			}
			if err := h.SetupUSBKey(ctx, cs); err != nil {
				s.Fatal("USBKey not working: ", err)
			}
		}
		if err := ms.RebootToMode(ctx, param.bootToMode); err != nil {
			s.Fatalf("Error during transition to %s: %+v", param.bootToMode, err)
		}
	} else if param.suspendResume {
		if err := h.RequireConfig(ctx); err != nil {
			s.Fatal("Requiring fw testing configs: ", err)
		}
		if err := h.RequireRPCUtils(ctx); err != nil {
			s.Fatal("Requiring RPC utils: ", err)
		}
		// Create instance of chrome for login so that DUT suspends and does not shut down.
		s.Log("Use Chrome service")
		if _, err := h.RPCUtils.ReuseChrome(ctx, &empty.Empty{}); err != nil {
			s.Fatal("Failed to create instance of chrome: ", err)
		}

		if err := h.Servo.RemoveCCDWatchdogs(ctx); err != nil {
			s.Error("Failed to remove watchdog for ccd: ", err)
		}

		testing.ContextLog(ctx, "Suspending DUT")
		cmd := h.DUT.Conn().CommandContext(ctx, "powerd_dbus_suspend", "--delay=3")
		if err := cmd.Start(); err != nil {
			s.Fatal("Failed to suspend DUT: ", err)
		}

		testing.ContextLog(ctx, "Checking for S0ix or S3 powerstate")
		if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "S0ix", "S3"); err != nil {
			s.Fatal("Failed to get S0ix or S3 powerstate: ", err)
		}

		if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurPress); err != nil {
			s.Fatal("Failed to press power key on DUT: ", err)
		}

		testing.ContextLog(ctx, "Waiting for S0 powerstate")
		err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "S0")
		if err != nil {
			s.Fatal("Failed to get S0 powerstate: ", err)
		}

		s.Log("Reconnecting to DUT")
		shortCtx, cancel := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
		defer cancel()
		if err := h.WaitConnect(shortCtx); err != nil {
			s.Fatal("Failed to reconnect to DUT: ", err)
		}

	} else if param.hardwareWatchdog {
		if err := h.Servo.RemoveCCDWatchdogs(ctx); err != nil {
			s.Error("Failed to remove watchdog for ccd: ", err)
		}
		// Daisydog is the watchdog service.
		cmd := `nohup sh -c 'sleep 2
			sync
			stop daisydog
			sleep 60 > /dev/watchdog' >/dev/null 2>&1 </dev/null &`
		if err := h.DUT.Conn().CommandContext(ctx, "bash", "-c", cmd).Run(); err != nil {
			s.Fatal("Failed to panic DUT: ", err)
		}
		s.Log("Waiting for DUT to become unreachable")
		h.CloseRPCConnection(ctx)

		if err := h.DUT.WaitUnreachable(ctx); err != nil {
			s.Fatal("Failed to wait for DUT to become unreachable: ", err)
		}
		s.Log("DUT became unreachable (as expected)")

		s.Logf("Reconnecting to DUT (%s)", h.Config.DelayRebootToPing)
		shortCtx, cancel := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
		defer cancel()
		if err := h.WaitConnect(shortCtx); err != nil {
			s.Fatal("Failed to reconnect to DUT: ", err)
		}
		s.Log("Reconnected to DUT")
	}
	// Sometimes events are missing if you check too quickly after boot.
	var events []reporters.Event
	if err := testing.Poll(ctx, func(context.Context) error {
		var err error
		events, err = r.EventlogList(ctx)
		if err != nil {
			return testing.PollBreak(err)
		}
		if len(events) == 0 {
			return errors.New("no new events found")
		}
		return nil
	}, &testing.PollOptions{
		Timeout: 1 * time.Minute, Interval: 5 * time.Second,
	}); err != nil {
		s.Fatal("Gathering events: ", err)
	}
	for _, event := range events {
		s.Log("Found event: ", event)
	}

	// Complicated rules here.
	// One of the param.requiredEventSets must be found.
	// Within that event set, all the regexs need to match to be considered found.
	requiredEventsFound := false
	for _, requiredEventSet := range param.requiredEventSets {
		foundAllRequiredEventsInSet := true
		for _, requiredEvent := range requiredEventSet {
			reRequiredEvent := regexp.MustCompile(requiredEvent)
			if !eventMessagesContainReMatch(ctx, events, reRequiredEvent) {
				foundAllRequiredEventsInSet = false
				break
			}
		}
		if foundAllRequiredEventsInSet {
			requiredEventsFound = true
			break
		}
	}
	if !requiredEventsFound {
		s.Error("Required event missing")
	}
	if param.prohibitedEvents != "" {
		reProhibitedEvents := regexp.MustCompile(param.prohibitedEvents)
		var allowedRe *regexp.Regexp
		if param.allowedEvents != "" {
			allowedRe = regexp.MustCompile(param.allowedEvents)
		}
		for _, event := range events {
			if reProhibitedEvents.MatchString(event.Message) && (allowedRe == nil || !allowedRe.MatchString(event.Message)) {
				s.Errorf("Incorrect event logged: %+v", event)
			}
		}
	}
}
