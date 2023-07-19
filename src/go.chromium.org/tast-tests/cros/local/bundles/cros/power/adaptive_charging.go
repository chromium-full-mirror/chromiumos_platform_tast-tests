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
		BugComponent: "b:167191",
		Contacts: []string{
			"chromeos-platform-power@google.com", // CrOS platform power developers
			"dbasehore@google.com",               // test author
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
		Timeout: time.Hour, // We only need up to an hour if the battery is low. Otherwise, the test should finish in about 10 minutes.
	})
}

type adaptiveChargingTestFunc = func(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn) error

func AdaptiveCharging(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr

	// Putting battery within testable range where the Adaptive Charging
	// notification will show.
	if err := charge.EnsureBatteryWithinRange(ctx, cr, 80.0, 95.0); err != nil {
		s.Fatalf("Failed to ensure battery percentage within %d%% to %d%%: %v", 80, 95, err)
	}

	// Stop powerd while we're changing directories that it touches.
	if err := upstart.StopJob(ctx, "powerd"); err != nil {
		s.Fatal("Failed to stop powerd: ", err)
	}
	defer upstart.RestartJob(cleanupCtx, "powerd")

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
