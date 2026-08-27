// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/utils"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// Cr50 max ticks is 0xffffffff. It counts in uSecs, so the wrap time is
	// ~4295 seconds
	// 0xffffffff / 1000000 = 4294.967295
	cr50WrapTime = 4295 * time.Second
	// wake 2 minutes before wrap in case the GSC time drifts
	wakeEarly = 2 * time.Minute
	// wait up to 2 minutes for the timer wrap
	waitForWrap = wakeEarly * 2
	// wake up early to monitor the wrap point
	wakeBeforeWrapDelay = cr50WrapTime - wakeEarly
	// Poll gettime every 5 seconds waiting for wrap
	monitorWrapDelay = 5 * time.Second
	monitorWrapCount = int(waitForWrap / monitorWrapDelay)
	// Verify the timer can wrap twice
	wrapIterations = 2
	wrapTestTime   = wrapIterations*cr50WrapTime + 10*time.Minute
)

type testGSCWrapTimeConfig struct {
	// Use console input to wake the GSC
	consoleWake bool
	// Enter deep sleep
	deepSleep bool
	// Expected chip specific reset flags
	originalResetFlags uint32
	// Expected standardized reset flags
	resetFlags uint32
	// Time to wait before reading gettime
	delay time.Duration
	// Number of times to check the GSC time
	pollCount int
}

type allowedDrift struct {
	ds        time.Duration
	coldReset time.Duration
}

// The amount of time to monitor around the wrap point is pretty short. The
// change in GSC time should be more accurate than the hour long wait to get
// to the wrap point.
var monitorWrapDrift = allowedDrift{
	ds: time.Second,
	// cold reset time is rounded to the nearest second on cr50. It may
	// be more out of sync with the test time depending on rounding.
	coldReset: 2 * time.Second,
}

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCWrapTime,
		Desc:    "Verify GSC time wraps correctly",
		Timeout: wrapTestTime,
		Contacts: []string{
			"cros-hwsec@google.com", // CrOS GSC Developers
			"mruthven@chromium.org", // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_h1_shield",
			"gsc_image_ti50"},
		Fixture: fixture.GSCOpenCCD,
		Params: []testing.Param{{
			// Use the console to wake Cr50 right before the wrap
			// point. Verify the timer still wraps successfully
			Name: "cr50_console",
			Val: testGSCWrapTimeConfig{
				consoleWake:        true,
				deepSleep:          true,
				originalResetFlags: uint32(ti50.Cr50ResetFlagHibernate | ti50.Cr50ResetFlagWakePin),
				resetFlags:         ti50.GscResetFlagHibernate,
				delay:              wakeBeforeWrapDelay,
				pollCount:          monitorWrapCount,
			},
		}, {
			// Verify the timer can wrap while Cr50 is awake
			Name: "cr50_standard",
			Val: testGSCWrapTimeConfig{
				deepSleep: false,
				delay:     wakeBeforeWrapDelay,
				pollCount: monitorWrapCount,
			},
		}, {
			// Verify the timer wraps when the rtc-alarm expires
			// in deep sleep
			Name: "cr50_rtc_alarm",
			Val: testGSCWrapTimeConfig{
				consoleWake:        false,
				deepSleep:          true,
				originalResetFlags: uint32(ti50.Cr50ResetFlagHibernate | ti50.Cr50ResetFlagRTCAlarm),
				resetFlags:         ti50.GscResetFlagHibernate,
				delay:              wakeBeforeWrapDelay,
				pollCount:          monitorWrapCount,
			},
		}, {

			// This test doesn't wait long enough for the GSC timer
			// to wrap. This just enters deep sleep and verifies
			// the DS and cold reset timers work through deep sleep.
			Name: "short",
			Val: testGSCWrapTimeConfig{
				consoleWake: true,
				deepSleep:   true,
				resetFlags:  ti50.GscResetFlagHibernate,
				delay:       100 * time.Second,
				pollCount:   5,
			},
		}},
	})
}

// GSCWrapTime verifies the deep sleep and cold reset timers can count past the
// rtc-alarm wrap point.
func GSCWrapTime(ctx context.Context, s *testing.State) {
	config := s.Param().(testGSCWrapTimeConfig)
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}

	drift := allowedDrift{
		coldReset: time.Duration(float64(config.delay) * 0.02),
	}
	if config.deepSleep {
		drift.ds = time.Second * 2
	} else {
		drift.ds = drift.coldReset
	}

	s.Log("(Re)starting GSC")
	b.Reset(ctx)
	coldResetTime := time.Now()
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	gscTime, err := i.Time(ctx)
	th.MustSucceed(err, "failed to get time")
	s.Logf("gsc time: %+v", gscTime)

	for j := 0; j < wrapIterations; j++ {
		// If PLT_RST_L is asserted, GSC will be able to enter deep sleep.
		b.GpioSet(ctx, ti50.GpioTi50PltRstL, !config.deepSleep)
		delay := config.delay

		gscTime, err := i.Time(ctx)
		if err != nil {
			s.Errorf("Failed to gettime: %s", err)
		} else if gscTime.ColdResetTime > cr50WrapTime {
			upTime := (gscTime.ColdResetTime % cr50WrapTime)
			s.Logf("Subtracing upTime (%s) from the delay (%s)", upTime, delay)
			delay = delay - upTime
		}

		if config.deepSleep {
			beforeDS := time.Now()
			err = b.WaitUntilDeepSleep(ctx, i, ti50.WaitForSleepTimeout)
			th.MustSucceed(err, "failed to enter deep sleep")
			s.Log("entered deep sleep")
			// Find out the delay by subtracting the time GSC has
			// already been up and the time it took for GSC to enter
			// deep sleep.
			// If the test waits too long, it won't be able to catch
			// the startup messages.
			delay = delay - gscTime.DeepSleepTime - time.Since(beforeDS)

		}

		delay = max(delay, time.Second)
		s.Logf("Wait for %s", delay)
		testing.Sleep(ctx, delay) // GoBigSleepLint: wait for delay
		s.Log("Done waiting")

		if config.deepSleep {
			if config.consoleWake {
				s.Log("Wake GSC with console")
				th.MustSucceed(i.WriteSerial(ctx, []byte("\r")), "failed to send wake char")
			}
			s.Log("Waiting for GSC to wake up")
			if out, err := i.WaitUntilDeepSleepWake(ctx, waitForWrap); err != nil {
				s.Errorf("run %d: failed to detect deep sleep resume: %s", j, err)
			} else {
				s.Logf("run %d: woke from sleep: %s", j, out)
			}

			if err := i.WaitUntilBooted(ctx); err != nil {
				s.Errorf("run %d: failed to wait until boot: %s", j, err)
			}
			if sysinfo, err := i.Sysinfo(ctx); err != nil {
				s.Errorf("run %d: failed to get sysinfo output: %s", j, err)
			} else {
				s.Logf("run %d: sysinfo:", j)
				s.Logf("Original GSC Reset Flags: %x", sysinfo.OriginalResetFlags)
				s.Logf("         GSC Reset Flags: %x", sysinfo.ResetFlags)

				// Check that any expected Reset Flags are set
				if config.resetFlags != 0 &&
					config.resetFlags != (sysinfo.ResetFlags&config.resetFlags) {
					s.Errorf("run %d: unexpected reset flags: expected %x got %x",
						j, config.resetFlags, sysinfo.ResetFlags)
				}

				// Original reset flags need to match exactly
				if config.originalResetFlags != 0 &&
					config.originalResetFlags != sysinfo.OriginalResetFlags {
					s.Errorf("run %d: unexpected original reset flags: expected %x got %x",
						j, config.originalResetFlags, sysinfo.OriginalResetFlags)
				}
			}
			// Deassert PLT_RST_L, so GSC won't reenter deep sleep.
			b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
		} else {
			// If GSC didn't enter deep sleep, it's been up and the
			// console has been printing output. Flush the output.
			th.MustSucceed(i.ClearInput(ctx), "flush output")
			th.MustSucceed(i.WaitUntilBooted(ctx), "failed to get prompt")
		}

		expected := ti50.GSCTime{
			ColdResetTime: time.Since(coldResetTime),
		}
		if config.deepSleep {
			// If GSC entered deep sleep, the DS time should be
			// about 0
			expected.DeepSleepTime = 0
		} else {
			// If GSC did not enter deep sleep, the DS time should be
			// about the same as the cold reset time.
			expected.DeepSleepTime = expected.ColdResetTime
		}
		lastGSCTime, err := checkWrappedTime(ctx, i, expected, drift)
		if err != nil {
			s.Fatalf("run %d: before wrap: %s", j, err)
		}
		gotGSCTime := time.Now()

		for k := 0; k < config.pollCount; k++ {
			testing.Sleep(ctx, monitorWrapDelay) // GoBigSleepLint: wait for the wrap to occur

			expected.ColdResetTime = lastGSCTime.ColdResetTime + time.Since(gotGSCTime)
			expected.DeepSleepTime = lastGSCTime.DeepSleepTime + time.Since(gotGSCTime)
			// The amount of time the DS and cold reset time time change after waitForWrap
			// should be more accurate.
			lastGSCTime, err = checkWrappedTime(ctx, i, expected, monitorWrapDrift)
			if err != nil {
				s.Fatalf("run %d:monitor wrap %d: %s", j, k, err)
			}
			gotGSCTime = time.Now()
		}
	}
	// Verify reboot clears DS and cold boot timers
	th.MustSucceed(i.Reboot(ctx), "Rebooted GSC")
	expected := ti50.GSCTime{
		ColdResetTime: time.Second,
		DeepSleepTime: time.Second,
	}
	if _, err = checkWrappedTime(ctx, i, expected, drift); err != nil {
		s.Errorf("time not cleared after GSC reboot: %s", err)
	}
}

// checkWrappedTime verifies the GSC time matches the given values
func checkWrappedTime(ctx context.Context, i *ti50.CrOSImage, expected ti50.GSCTime, drift allowedDrift) (ti50.GSCTime, error) {
	// GSC may have entered regular sleep. Use WaitUntilBooted to make sure
	// the console is responsive before sending gettime.
	if err := i.WaitUntilBooted(ctx); err != nil {
		return ti50.GSCTime{}, err
	}
	gscTime, err := i.Time(ctx)
	if err != nil {
		return ti50.GSCTime{}, err
	}
	testing.ContextLogf(ctx, "time: %+v", gscTime)
	testing.ContextLogf(ctx, "Expected time: %+v", expected)
	testing.ContextLogf(ctx, "allowed drift: %+v", drift)

	var errs []string

	maxColdResetTime := expected.ColdResetTime + drift.coldReset
	minColdResetTime := max(0, expected.ColdResetTime-drift.coldReset)
	maxDeepSleepTime := expected.DeepSleepTime + drift.ds
	minDeepSleepTime := max(0, expected.DeepSleepTime-drift.ds)

	if gscTime.ColdResetTime > maxColdResetTime ||
		gscTime.ColdResetTime < minColdResetTime {
		msg := fmt.Sprintf("cold_boot time %s out of range: min %s max %s",
			gscTime.ColdResetTime, minColdResetTime, maxColdResetTime)
		errs = append(errs, msg)

	}
	if gscTime.DeepSleepTime > maxDeepSleepTime || gscTime.DeepSleepTime < minDeepSleepTime {
		msg := fmt.Sprintf("DS time %s not in range: min %s max %s",
			gscTime.DeepSleepTime, minDeepSleepTime, maxDeepSleepTime)
		errs = append(errs, msg)
	}
	if len(errs) != 0 {
		return ti50.GSCTime{}, errors.Errorf("invalid time: %s", errs)
	}
	return gscTime, nil
}
