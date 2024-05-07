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

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/mlservice"
	"go.chromium.org/tast-tests/cros/local/power/charge"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AdaptiveCharging,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that Adaptive Charging functionality works correctly",
		BugComponent: "b:1361410",
		Contacts: []string{
			"chromeos-platform-power@google.com", // CrOS platform power developers
			"yanyeli@google.com",
		},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(
			hwdep.Battery(),  // Test doesn't run on ChromeOS devices without a battery.
			hwdep.ChromeEC(), // Test requires Chrome EC to set battery sustainer.
			hwdep.ECFeatureChargeControlV2(),
			hwdep.ForceDischarge(),
		),
		Fixture: setup.PowerAshAdaptiveCharging,
		Timeout: 90 * time.Minute, // We only need up to 90 minutes if the battery is low or the system discharges slowly. Otherwise, the test should finish in about 10 minutes or less.
	})
}

type adaptiveChargingTestFunc = func(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn) error

const powerdChargeHistoryDir = "/var/lib/power_manager/charge_history/"

func AdaptiveCharging(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr

	// Putting battery within testable range where the Adaptive Charging
	// notification will show. Adaptive Charging will only trigger when the
	// battery is at or below 95%, so discharge down to at least 93%.
	if err := charge.EnsureBatteryWithinRange(ctx, cr, 80.0, 93.0); err != nil {
		s.Fatalf("Failed to ensure battery percentage within %d%% to %d%%: %v", 80, 93, err)
	}

	// Ensure that charging is turned on.
	if err := setup.AllowBatteryCharging(ctx); err != nil {
		s.Fatal("Failed to enable charging for DUT: ", err)
	}
	if err := setup.WaitUntilPowerSourceChanges(ctx, true); err != nil {
		s.Fatal("Timed out waiting for DUT to start charging: ", err)
	}

	// Stop powerd while we're changing directories that it touches.
	if err := upstart.StopJob(ctx, "powerd"); err != nil {
		s.Fatal("Failed to stop powerd: ", err)
	}
	defer upstart.RestartJob(cleanupCtx, "powerd")

	fakeHistoryDir, err := os.MkdirTemp("/tmp", "charge_history")
	if err != nil {
		s.Fatal("Failed to create fake charge history directory: ", err)
	}

	if err := testexec.CommandContext(ctx, "mount", "--bind", fakeHistoryDir, powerdChargeHistoryDir).Run(); err != nil {
		s.Fatalf("Failed to mount fake charge history directory %s on powerd charge history directory %s: %v", fakeHistoryDir, powerdChargeHistoryDir, err)
	}
	defer func(c context.Context) {
		if err := testexec.CommandContext(c, "umount", powerdChargeHistoryDir).Run(); err != nil {
			testing.ContextLogf(c, "Failed to unmount fake charge history directory from %s: %v", powerdChargeHistoryDir, err)
		}
	}(cleanupCtx)

	// Create fake charge history to make sure the Adaptive Charging heuristic
	// doesn't disable the feature.
	holdTimeDir := filepath.Join(powerdChargeHistoryDir, "hold_time_on_ac/")
	timeFullDir := filepath.Join(powerdChargeHistoryDir, "time_full_on_ac/")
	timeAcDir := filepath.Join(powerdChargeHistoryDir, "time_on_ac/")
	chargeEventsDir := filepath.Join(powerdChargeHistoryDir, "charge_events/")
	createFakeChargeHistory(s, holdTimeDir)
	createFakeChargeHistory(s, timeFullDir)
	createFakeChargeHistory(s, timeAcDir)
	createFakeChargeHistory(s, chargeEventsDir)

	if err := upstart.EnsureJobRunning(ctx, "powerd"); err != nil {
		s.Fatal("Failed to restart powerd: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Chrome Test API Connection: ", err)
	}

	f, err := mlservice.StartFakeAdaptiveChargingMLService(ctx)
	if err != nil {
		s.Fatal("Failed to start fake Adaptive Charging ML service: ", err)
	}
	defer f.StopService()

	for _, param := range []struct {
		// the subtest name
		name string
		// the callback to run the subtest
		testFunc adaptiveChargingTestFunc
	}{
		{
			name:     "charge_now",
			testFunc: testChargeNow,
		}, {
			name:     "settings",
			testFunc: testSettings,
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			if err := upstart.RestartJob(ctx, "powerd"); err != nil {
				s.Fatal("Failed to restart powerd: ", err)
			}

			s.Logf("Running subtest: %s", param.name)
			if err := param.testFunc(ctx, cr, tconn); err != nil {
				s.Fatalf("Failed subtest %s with error: %v", param.name, err)
			}
			defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)
		})
	}
}

// testChargeNow will verify that clicking the "Fully Charge Now" button that
// shows up via notification when Adaptive Charging starts to delay charge
// successfully cancels Adaptive Charging.
func testChargeNow(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn) error {
	// Battery sustained will be enabled before the notification shows up.
	if err := pollUntilBatterySustainingState(ctx, true); err != nil {
		return err
	}

	// Wait for the notification center icon to become visible. It's not
	// visible until a notification appears.
	ui := uiauto.New(tconn)
	notificationCenterIcon := nodewith.HasClass("NotificationCenterTray")
	if err := ui.WithTimeout(time.Minute).WaitUntilExists(notificationCenterIcon)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for the notification center icon to exist")
	}

	// Open the notification center to ensure the notification is visible.
	if err := ui.LeftClick(notificationCenterIcon)(ctx); err != nil {
		return errors.Wrap(err, "failed to click notification center icon")
	}

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

// createFakeChargeHistory creates and populates `dir` with fake charge history values.
func createFakeChargeHistory(s *testing.State, dir string) {
	if err := os.Mkdir(dir, 0700); err != nil {
		s.Fatalf("Failed to create fake charge history subdir, %s: %v", dir, err)
	}

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	duration := 24 * time.Hour
	contents := []byte("\"" + strconv.FormatInt(duration.Microseconds(), 10) + "\"")
	for i := 0; i < 15; i++ {
		dateMicroseconds := today.Add(time.Duration(-i) * duration).UnixMicro()
		// powerd uses Windows Epoch for Adaptive Charging timestamps, since that's what libchrome uses for timestamp
		// serialization... for this reason, we need to add this number to our microseconds to get a correct number of
		// microseconds, as commented for libchrome/base/time/time.h:Time::kTimeTToMicrosecondsOffset.
		// Windows Epoch is 1601-01-01 and Unix Epoch is 1970-01-01.
		dateMicroseconds += 11644473600000000
		if err := ioutil.WriteFile(filepath.Join(dir, strconv.FormatInt(dateMicroseconds, 10)), contents, 0600); err != nil {
			s.Fatal("Failed to write charge history file: ", err)
		}
	}
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
