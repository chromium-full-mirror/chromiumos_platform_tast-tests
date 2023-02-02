// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"chromiumos/tast/common/servo"
	"chromiumos/tast/common/testexec"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/ossettings"
	"chromiumos/tast/local/chrome/uiauto/quicksettings"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/mlservice"
	"chromiumos/tast/local/power"
	"chromiumos/tast/local/power/charge"
	"chromiumos/tast/local/upstart"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AdaptiveCharging,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that Adaptive Charging functionality works correctly",
		BugComponent: "b:167191",
		Contacts: []string{
			"dbasehore@google.com",               // test author
			"chromeos-platform-power@google.com", // CrOS platform power developers
		},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(
			hwdep.Battery(),  // Test doesn't run on ChromeOS devices without a battery.
			hwdep.ChromeEC(), // Test requires Chrome EC to set battery sustainer.
			hwdep.ECFeatureChargeControlV2(),
		),
		Vars:    []string{"servo"},
		Timeout: time.Hour, // We only need up to an hour if the battery is low. Otherwise, the test should finish in about 10 minutes.
	})
}

type adaptiveChargingTestFunc = func(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn) error

func AdaptiveCharging(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// After enabled the AdaptiveCharging feature flag, the setting to enable
	// the feature should be enabled by default.
	cr, err := chrome.New(ctx,
		chrome.EnableFeatures("AdaptiveCharging"),
		chrome.ARCDisabled())
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	srvo, err := servo.NewDirect(ctx, s.RequiredVar("servo"))
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer srvo.Close(cleanupCtx)

	// Putting battery within testable range where the Adaptive Charging
	// notification will show.
	if err := charge.EnsureBatteryWithinRange(ctx, cr, srvo, 80.0, 95.0); err != nil {
		s.Fatalf("Failed to ensure battery percentage within %d%% to %d%%: %v", 80, 95, err)
	}

	// Stop powerd while we're changing directories that it touches.
	if err := upstart.StopJob(ctx, "powerd"); err != nil {
		s.Fatal("Failed to stop powerd: ", err)
	}
	defer upstart.RestartJob(ctx, "powerd")

	// Create fake charge history to make sure the Adaptive Charging heuristic
	// doesn't disable the feature.
	timeFullDir := "/var/lib/power_manager/charge_history/time_full_on_ac/"
	timeAcDir := "/var/lib/power_manager/charge_history/time_on_ac/"
	chargeEventsDir := "/var/lib/power_manager/charge_history/charge_events/"
	storedTimeFullDir := createFakeChargeHistory(s, timeFullDir)
	defer os.Rename(storedTimeFullDir, timeFullDir)
	defer os.RemoveAll(timeFullDir)
	storedTimeAcDir := createFakeChargeHistory(s, timeAcDir)
	defer os.Rename(storedTimeAcDir, timeAcDir)
	defer os.RemoveAll(timeAcDir)
	storedChargeEventsDir := createFakeChargeHistory(s, chargeEventsDir)
	defer os.Rename(storedChargeEventsDir, chargeEventsDir)
	defer os.RemoveAll(chargeEventsDir)

	if err := upstart.EnsureJobRunning(ctx, "powerd"); err != nil {
		s.Fatal("Failed to restart powerd: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Chrome Test API Connection: ", err)
	}

	for _, param := range []struct {
		// the subtest name
		name string
		// the callback to run the subtest
		testFunc   adaptiveChargingTestFunc
		prediction []float64
	}{
		{
			name:       "slowcharging",
			testFunc:   testSlowCharging,
			prediction: []float64{0.0, 0.0, 0.0, 1.0, 0.0, 0.0, 0.0, 0.0, 0.0},
		}, {
			name:       "charge_now",
			testFunc:   testChargeNow,
			prediction: []float64{0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 1.0},
		}, {
			name:       "settings",
			testFunc:   testSettings,
			prediction: []float64{0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 1.0},
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			f, err := mlservice.StartFakeAdaptiveChargingMLService(ctx, param.prediction)
			if err != nil {
				s.Fatal("Failed to start fake Adaptive Charging ML service: ", err)
			}
			defer f.StopService()

			if err := upstart.RestartJob(ctx, "powerd"); err != nil {
				s.Fatal("Failed to restart powerd: ", err)
			}

			s.Logf("Running subtest: %s", param.name)
			if err := param.testFunc(ctx, cr, tconn); err != nil {
				s.Fatalf("Failed subtest %s with error: %v", param.name, err)
			}
			defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)
		})
	}
}

// testSlowCharging will verify that slow charging occurs in Adaptive Charging.
func testSlowCharging(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn) error {
	// The battery percentage is between 80% and 95%, and the predicted unplug
	// time is 3 hours away which provides sufficient time for slow charging. We
	// expect powerd to disable the battery sustain and set a charge current
	// limit in the EC for slow charging,
	//
	// Verify that the battery charge current limit is set for slow charging.
	if err := pollUntilSlowChargingState(ctx); err != nil {
		return err
	}

	// Verify that battery sustain is disabled and Adaptive Charging is not
	// delaying charge.
	if err := pollUntilBatterySustainingState(ctx, false); err != nil {
		return err
	}

	return nil
}

// testChargeNow will verify that clicking the "Fully Charge Now" button that
// shows up via notification when Adaptive Charging starts to delay charge
// successfully cancels Adaptive Charging.
func testChargeNow(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn) error {
	// Battery sustained will be enabled before the notification shows up.
	if err := pollUntilBatterySustainingState(ctx, true); err != nil {
		return err
	}

	// Show quicksettings in its collapsed state, which will ensure that the
	// Adaptive Charging notification is fully visible. If quicksettings is in
	// the expanded state, it may truncate the button, preventing it from being
	// clicked.
	if err := quicksettings.Collapse(ctx, tconn); err != nil {
		return err
	}
	defer quicksettings.Hide(ctx, tconn)

	ui := uiauto.New(tconn)
	chargeNowButton := nodewith.Name("Fully charge now").Role(role.Button)
	if err := ui.WithTimeout(time.Minute).WaitUntilExists(chargeNowButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for the Charge Now button to exist")
	}

	if err := ui.LeftClick(chargeNowButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to click Full Charge Now button")
	}

	// Verify that Adaptive Charging stopped delaying charge.
	if err := pollUntilBatterySustainingState(ctx, false); err != nil {
		return err
	}

	return nil
}

// testSettings will disable the re-enable Adaptive Charging via the Settings
// app.
func testSettings(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)
	toggleAdaptiveCharging := nodewith.Name("Adaptive charging").Role(role.ToggleButton)
	settings, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, "power", ui.WaitUntilExists(toggleAdaptiveCharging))
	if err != nil {
		return err
	}
	defer settings.Close(ctx)

	if err := ui.LeftClick(toggleAdaptiveCharging)(ctx); err != nil {
		return errors.Wrap(err, "failed to toggle Adaptive Charging off")
	}

	if err := pollUntilBatterySustainingState(ctx, false); err != nil {
		return err
	}

	if err := ui.LeftClick(toggleAdaptiveCharging)(ctx); err != nil {
		return errors.Wrap(err, "failed to toggle Adaptive Charging on")
	}

	if err := pollUntilBatterySustainingState(ctx, true); err != nil {
		return err
	}

	return nil
}

// createFakeChargeHistory populates `dir` with fake charge history and stores
// the existing contents in the directory specified by the return value.
func createFakeChargeHistory(s *testing.State, dir string) string {
	storedDir, err := ioutil.TempFile("/tmp", "stored_charge_history.*")
	if err != nil {
		s.Fatal("Failed to copy existing charge history to temporary location: ", err)
	}

	os.Rename(dir, storedDir.Name())

	os.Mkdir(dir, 0666)
	if err != nil {
		s.Fatal("Failed to create directory for temporary charge history: ", err)
	}

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	duration := 24 * time.Hour
	contents := []byte("\"" + strconv.FormatInt(duration.Microseconds(), 10) + "\"")
	for i := 0; i < 15; i++ {
		ioutil.WriteFile(filepath.Join(dir, strconv.FormatInt(10*today.Add(time.Duration(-i)*duration).UnixMicro(), 10)), contents, 0666)
	}

	return storedDir.Name()
}

func pollUntilBatterySustainingState(ctx context.Context, sustaining bool) error {
	if sustaining {
		testing.ContextLog(ctx, "Waiting for battery sustainer to enable")
	} else {
		testing.ContextLog(ctx, "Waiting for battery sustainer to disable")
	}

	return testing.Poll(ctx, func(c context.Context) error {
		out, err := testexec.CommandContext(ctx, "ectool", "chargecontrol").Output()
		if err != nil {
			return errors.Wrap(err, "failed to check battery sustainer")
		}
		testing.ContextLogf(ctx, "chargecontrol status: %s", out)
		sustainDetect := regexp.MustCompile(`Battery sustainer = on`)
		if sustainDetect.MatchString(string(out)) != sustaining {
			if sustaining {
				return errors.New("Battery sustainer is still off")
			}
			return errors.New("Battery sustainer is still on")
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: time.Second})
}

// Expression to extract `chg_current` from the output of the `ectool
// chargestate show` command.
var chargeCurrentRegex = regexp.MustCompile(`chg_current = ([0-9]+)mA`)

// parseChargeCurrent parses the output of `ectool chargestate show` to return
// the value of `chg_current` which is the charge current limit set by the EC.
func parseChargeCurrent(output []byte) (int, error) {
	chargeCurrentMatch := chargeCurrentRegex.FindSubmatch([]byte(output))
	if chargeCurrentMatch == nil {
		return -1, errors.New("no `chg_current` value found")
	}

	return strconv.Atoi(string(chargeCurrentMatch[1]))
}

func pollUntilSlowChargingState(ctx context.Context) error {
	testing.ContextLog(ctx, "Waiting for slow charging to start")

	return testing.Poll(ctx, func(c context.Context) error {
		out, err := testexec.CommandContext(ctx, "ectool", "chargestate", "show").Output()
		if err != nil {
			return errors.Wrap(err, "failed to check charge state")
		}
		testing.ContextLogf(ctx, "chargestate: %s", out)

		// Check the current charge limit set by the EC.
		chargeCurrentLimitMA, err := parseChargeCurrent(out)
		if err != nil {
			return errors.Wrap(err, "failed to obtain set charge current limit")
		}

		status, err := power.GetStatus(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to obtain DUT power status")
		}

		// Check the actual current being supplied to the battery.
		actualCurrentMA := status.BatteryCurrent * 1000

		// We expect the charge current limit to be set to 0.1C (i.e. 10% of the
		// battery design capacity) for slow charging.
		expectedLimitMA := int(status.BatteryChargeFullDesign * 0.1 * 1000)

		testing.ContextLogf(ctx, "Actual current: %vmA; Current charge limit: %vmA; Expected limit: %vmA", actualCurrentMA, chargeCurrentLimitMA, expectedLimitMA)

		// Check if the charge current limit has been set.
		//
		// If `chargeCurrentLimitMA` is 0, it means that the battery is not
		// actively charging, which indicates that battery sustain is enabled or
		// the battery is discharging.
		//
		// Else, if `chargeCurrentLimitMA` is greater than the expected limit,
		// the current limit has not been set. The EC will round down the
		// current limit to the closest supported rate. The exception is a
		// charger with a minimum current limit above 0.1C, which will set a
		// higher current limit. We should add exceptions for such devices if we
		// ever encounter any.
		//
		// Otherwise, if the current limit has been set but the actual current
		// supplied to the battery exceeds the set limit by more than 10% (to
		// allow for a charger circuit tolerance of 5% and potential 5%
		// measurement error), this could indicate a failure in the EC or
		// hardware to ensure the supplied current is limited accordingly.
		if chargeCurrentLimitMA == 0 {
			return errors.New("Battery is not charging")
		} else if chargeCurrentLimitMA > expectedLimitMA {
			return errors.New("Charge current limit is higher than expected slow charging limit")
		} else if actualCurrentMA > float64(chargeCurrentLimitMA)*1.1 {
			return errors.New("Actual charge current supplied to the battery is greater than the limit that has been set by the EC")
		}

		testing.ContextLog(ctx, "Charge current limit has now been set")
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: time.Second})
}
