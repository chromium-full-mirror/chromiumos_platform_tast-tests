// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// testParameters contains all the data needed to run a single test iteration.
type testParameters struct {
	apBootRegexp string
	apBootState  string
	apBootMax    time.Duration
}

func init() {
	testing.AddTest(&testing.Test{
		Func: BootTime,
		Desc: "Measures EC boot time",
		Contacts: []string{
			"chromeos-faft@google.com",
			"jbettis@chromium.org",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Attr:         []string{"group:firmware", "firmware_ec", "firmware_smoke", "firmware_bringup", "firmware_stressed", "firmware_meets_kpi", "firmware_enabled", "firmware_ec_ro", "firmware_ec_rw"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Params: []testing.Param{
			{
				Name: "x86",
				ExtraHardwareDeps: hwdep.D(hwdep.X86(), hwdep.SkipOnModel(
					"berknip",
					"dirinboz",
					"ezkinil",
					"gumboz",
					"jelboz",
					"jelboz360",
					"morphius",
					"vilboz",
					"vilboz14",
					"vilboz360",
					"woomax",
				)),
				Val: testParameters{
					// Same as default, with 1.5 seconds
					apBootRegexp: `power state \d+ = S0,`,
					apBootState:  "S0",
					apBootMax:    1500 * time.Millisecond,
				},
			},
			{
				// Zork machines have a waiver for the boot-to-kernel time. go/zork-waiver-b2k
				Name: "zork",
				ExtraHardwareDeps: hwdep.D(hwdep.Model(
					"berknip",
					"dirinboz",
					"ezkinil",
					"gumboz",
					"jelboz",
					"jelboz360",
					"morphius",
					"vilboz",
					"vilboz14",
					"vilboz360",
					"woomax",
				)),
				Val: testParameters{
					apBootRegexp: `HC 0x|Port 80|ACPI query|Executing host reboot command`,
					apBootMax:    2500 * time.Millisecond,
				},
			},
			{
				Name:              "default",
				ExtraHardwareDeps: hwdep.D(hwdep.NoX86()),
				Val: testParameters{
					apBootRegexp: `power state \d+ = S0,`,
					apBootState:  "S0",
					apBootMax:    1 * time.Second,
				},
			},
		},
	})
}

const (
	maxWaitTime time.Duration = 60 * time.Second
)

// BootTime measures the time from EC boot to the first signal that the AP is booting.
// This test is not a hard rule, what really matters is the platform.BootPerfServerCrosPerf test.
// See go/chromeos-boottime for a proposal to revisit this.
// It appears that this test is not measuring the same thing as "Boot-to-Kernel" or "Boot to chromeball", but should catch a regression in that time.
func BootTime(ctx context.Context, s *testing.State) {
	param := s.Param().(testParameters)

	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to local config: ", err)
	}

	useBootTimeCommand := isECSupportBootTimeCommand(ctx, h.Servo)

	var coldBootTime, apBootTime time.Duration
	if useBootTimeCommand {
		coldBootTime, apBootTime = measureBootTimeViaECBootTimeCommand(ctx, s, param.apBootState, h.Servo)
	} else {
		coldBootTime, apBootTime = measureBootTimeViaFollowingECLog(ctx, s, param.apBootRegexp, h.Servo)
	}

	s.Logf("EC cold boot time: %s", coldBootTime)
	s.Logf("AP Boot time: %s", apBootTime)
	var coldBootMax time.Duration = h.Config.ECColdBootTime
	if coldBootTime > coldBootMax {
		s.Errorf("EC boot time = %s; want <=%s", coldBootTime, coldBootMax)
	}
	if apBootTime > param.apBootMax {
		s.Errorf("AP Boot time = %s; want <=%s", apBootTime, param.apBootMax)
	}
	if s.HasError() {
		s.Log("To debug, check the log in $LOGDIR/autoserv_test/servod_*/ec.txt")
	}
}

func isECSupportBootTimeCommand(ctx context.Context, ser *servo.Servo) bool {
	_, err := runECBootTimeCommand(ctx, ser, "S5")
	return err == nil
}

func measureBootTimeViaECBootTimeCommand(ctx context.Context, s *testing.State, apBootState string, ser *servo.Servo) (time.Duration, time.Duration) {
	s.Log("Rebooting EC")
	if err := ser.RunECCommand(ctx, "reboot"); err != nil {
		s.Fatal("Failed to send reboot command: ", err)
	}

	// GoBigSleepLint: Sleep for the first few seconds, then we can safely disable chan then send console command to get boot times
	if err := testing.Sleep(ctx, time.Second*5); err != nil {
		s.Fatal("Failed to sleep for skipping boot ec console jamming: ", err)
	}
	if err := ser.RunECCommand(ctx, "chan save"); err != nil {
		s.Fatal("Failed to save chan: ", err)
	}
	defer func() {
		if err := ser.RunECCommand(ctx, "chan restore"); err != nil {
			s.Fatal("Failed to restore chan: ", err)
		}
	}()
	if err := ser.RunECCommand(ctx, "chan 0"); err != nil {
		s.Fatal("Failed to save chan: ", err)
	}

	elapsedColdBootTime, err := runECBootTimeCommand(ctx, ser, "S5")
	if err != nil {
		s.Fatal("Failed to get cold boot time: ", err)
	}

	elapsedAPBootTime, err := runECBootTimeCommand(ctx, ser, apBootState)
	if err != nil {
		s.Fatal("Failed to get AP boot time: ", err)
	}

	return elapsedColdBootTime, elapsedAPBootTime - elapsedColdBootTime
}

func runECBootTimeCommand(ctx context.Context, ser *servo.Servo, powerState string) (time.Duration, error) {
	cmd := fmt.Sprintf("boottime %s", powerState)
	timePattern := fmt.Sprintf("first %s: (-?\\d+ms)", powerState)
	timeMatches, err := ser.RunECCommandGetOutput(ctx, cmd, []string{timePattern})
	if err != nil {
		return 0, errors.Wrap(err, "failed to run boottime command")
	}

	if len(timeMatches) == 1 && len(timeMatches[0]) == 2 {
		return time.ParseDuration(timeMatches[0][1])
	}
	return 0, errors.Errorf("unexpected boottime output pattern matches: %v", timeMatches)
}

func measureBootTimeViaFollowingECLog(ctx context.Context, s *testing.State, apBootRegexp string, ser *servo.Servo) (time.Duration, time.Duration) {
	rebootingStarted := regexp.MustCompile(`Rebooting!`)
	coldBootFinished := regexp.MustCompile(`power state \d+ = S5,`)
	// This means the AP is initialized, but does not mean ChromeOS is booted.
	apBootFinished := regexp.MustCompile(apBootRegexp)
	// YY-mm-dd HH:MM:SS.sss, but only looking at the MM:SS.sss here
	// See HOST_STRFTIME in src/platform/ec/util/ec3po/console.py
	uartAbsoluteTime := regexp.MustCompile(`^\d+-\d+-\d+ \d+:(\d+):(\d+)\.(\d+)`)

	timestampState, err := ser.GetOnOff(ctx, servo.ECUARTTimestamp)
	if err != nil {
		s.Fatal("Failed to get EC UART timestamping: ", err)
	}
	if err := ser.SetOnOff(ctx, servo.ECUARTTimestamp, servo.On); err != nil {
		s.Fatal("Failed to enable EC UART timestamping: ", err)
	}
	defer func() {
		var onoff servo.OnOffValue

		if timestampState {
			onoff = servo.On
		} else {
			onoff = servo.Off
		}
		if err := ser.SetOnOff(ctx, servo.ECUARTTimestamp, onoff); err != nil {
			s.Fatal("Failed to restore EC UART timestamping: ", err)
		}
	}()

	cancel, err := ser.EnableUARTCapture(ctx, servo.ECUARTCapture)
	if err != nil {
		s.Fatal("Failed to capture EC UART: ", err)
	}

	defer func() {
		if err := cancel(ctx); err != nil {
			s.Fatal("Failed to cancel capture EC UART: ", err)
		}
	}()

	s.Log("Rebooting EC")
	if err := ser.RunECCommand(ctx, "reboot"); err != nil {
		s.Fatal("Failed to send reboot command: ", err)
	}

	// Set times to invalid values to start.
	var (
		startTime      time.Duration = -1
		coldBootTime   time.Duration = -1
		apBootTime     time.Duration = -1
		uartTime       time.Duration = -1
		rolloverOffset time.Duration
		leftoverLines  string
		isStarted      bool
		priorMinute    = -1
	)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if lines, err := ser.GetQuotedString(ctx, servo.ECUARTStream); err != nil {
			s.Fatal("Failed to read UART: ", err)
		} else if lines != "" {
			// It is possible to read partial lines, so save the part after newline for later
			lines = leftoverLines + lines
			if crlfIdx := strings.LastIndex(lines, "\r\n"); crlfIdx < 0 {
				leftoverLines = lines
				lines = ""
			} else {
				leftoverLines = lines[crlfIdx+2:]
				lines = lines[:crlfIdx+2]
			}

			for _, l := range strings.Split(lines, "\r\n") {
				// If the line starts with a timestamp, capture it. Only look at the
				// seconds & millis to make things easier.
				if match := uartAbsoluteTime.FindStringSubmatch(l); match != nil {
					minute, err := strconv.Atoi(match[1])
					if err != nil {
						s.Fatal("Could not parse uart time mins: ", err)
					}
					secs, err := strconv.Atoi(match[2])
					if err != nil {
						s.Fatal("Could not parse uart time secs: ", err)
					}
					millis, err := strconv.Atoi(match[3])
					if err != nil {
						s.Fatal("Could not parse uart time millis: ", err)
					}
					if minute < priorMinute {
						rolloverOffset += time.Hour
					}
					priorMinute = minute
					uartTime = time.Duration(minute)*time.Minute + time.Duration(secs)*time.Second + time.Duration(millis)*time.Millisecond + rolloverOffset
				}
				if uartTime >= 0 {
					s.Logf("%s: %q", uartTime, l)
				} else {
					s.Logf("%q", l)
				}
				if match := rebootingStarted.FindString(l); match != "" {
					isStarted = true
					s.Logf("Reboot detected = %q", match)
				}
				if startTime < 0 && uartTime >= 0 && isStarted {
					startTime = uartTime
				}
				if coldBootTime < 0 {
					if match := coldBootFinished.FindString(l); match != "" {
						if startTime >= 0 {
							coldBootTime = uartTime - startTime
						} else {
							s.Log("Could not determine coldBootTime")
						}
						s.Logf("Cold Boot = %q", match)
					}
				}
				if coldBootTime >= 0 && apBootTime < 0 {
					if match := apBootFinished.FindString(l); match != "" {
						apBootTime = uartTime - startTime - coldBootTime
						s.Logf("AP Boot = %q", match)
					}
				}
			}
		}
		if coldBootTime < 0 {
			return errors.New("failed to find ColdBootTime in EC Log")
		}
		if apBootTime < 0 {
			return errors.New("failed to find BootTime in EC Log")
		}
		return nil
	}, &testing.PollOptions{Interval: time.Millisecond * 200, Timeout: maxWaitTime}); err != nil {
		s.Error("EC output parsing failed: ", err)
	}
	return coldBootTime, apBootTime
}
