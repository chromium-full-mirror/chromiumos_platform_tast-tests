// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/mlservice"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const powerdChargeHistoryDir = "/var/lib/power_manager/charge_history/"
const (
	devicePowerBatteryChargingOptimizationStandard = 1
	devicePowerBatteryChargingOptimizationAdaptive = 2
	devicePowerBatteryChargingOptimizationLimited  = 3
)

// Putting battery within testable range where both Adaptive Charging and Charge
// Limit can be triggered.
// Adaptive Charging will only trigger when the
// battery is at or below 95%.
// Charge Limit will triggered when the battery is trying to charge over 80%.
var chargeParam = power.ChargeParams{
	MaxBatteryPreparationTime: 80 * time.Minute,
	MinChargePercentage:       80.0,
	MaxChargePercentage:       93.0,
	// Powerd uses display battery percentage for adaptive charging.
	// Set UseDisplayPercentage to true when preparing battery for adaptive
	// charging.
	UseDisplayPercentage:  true,
	DischargeOnCompletion: false,
	IsCustomized:          false,
	IsPowerQual:           false,
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         DevicePowerBatteryChargingOptimization,
		Desc:         "Test the DevicePowerBatteryChargingOptimization policy",
		Contacts:     []string{"chromeos-power-team@google.com", "jingmuli@google.com"},
		BugComponent: "b:1111617",
		Attr:         []string{"group:power", "power_weekly"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(
			hwdep.Battery(),  // Test doesn't run on ChromeOS devices without a battery.
			hwdep.ChromeEC(), // Test requires Chrome EC to set battery sustainer.
			hwdep.ECFeatureChargeControlV2(),
			hwdep.ForceDischarge(),
		),
		Fixture: fixture.FakeDMSEnrolled,
		Params: []testing.Param{
			{
				Name: "standard",
				Val:  devicePowerBatteryChargingOptimizationStandard, // DevicePowerBatteryChargingOptimization = Standard
			},
			{
				Name: "adaptive",
				Val:  devicePowerBatteryChargingOptimizationAdaptive, // DevicePowerBatteryChargingOptimization = Adaptive
			},
			{
				Name: "limited",
				Val:  devicePowerBatteryChargingOptimizationLimited, // DevicePowerBatteryChargingOptimization = Limited
			},
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DevicePowerBatteryChargingOptimization{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.DevicePowerBatteryChargingOptimization{}, pci.VerifiedFunctionalityOS),
		},
		Timeout: 3*time.Minute + chargeParam.MaxBatteryPreparationTime,
	})
}

func DevicePowerBatteryChargingOptimization(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	cr, err := chrome.New(ctx,
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.KeepEnrollment(),
		chrome.ExtraArgs("--disable-policy-key-verification"),
		chrome.EnableFeatures("AdaptiveCharging"),
	)
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}
	defer cr.Close(cleanupCtx)

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	if err := setup.PrepareBattery(ctx, chargeParam); err != nil {
		s.Fatalf("Failed to ensure battery percentage within %.2f%% to %.2f%%: %v", chargeParam.MinChargePercentage, chargeParam.MaxChargePercentage, err)
	}

	// Ensure that charging is turned on.
	if err := setup.AllowBatteryCharging(ctx); err != nil {
		s.Fatal("Failed to enable charging for DUT: ", err)
	}
	if err := setup.WaitUntilPowerSourceChanges(ctx, true); err != nil {
		s.Fatal("Timed out waiting for DUT to start charging: ", err)
	}

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
	createFakeChargeHistoryData(s, holdTimeDir)
	createFakeChargeHistoryData(s, timeFullDir)
	createFakeChargeHistoryData(s, timeAcDir)
	createFakeChargeHistoryData(s, chargeEventsDir)

	if err := upstart.EnsureJobRunning(ctx, "powerd"); err != nil {
		s.Fatal("Failed to restart powerd: ", err)
	}

	f, err := mlservice.StartFakeAdaptiveChargingMLService(ctx)
	if err != nil {
		s.Fatal("Failed to start fake Adaptive Charging ML service: ", err)
	}
	defer f.StopService()

	deviceOptimizationPolicyValue := s.Param().(int)
	// Apply policy.
	chargingOptimization := deviceOptimizationPolicyValue
	policies := []policy.Policy{&policy.DevicePowerBatteryChargingOptimization{Val: chargingOptimization}}

	if err := policyutil.ServeAndVerify(ctx, fdms, cr, policies); err != nil {
		s.Fatal("Failed to serve and verify policies: ", err)
	}

	// Verify battery sustainer state based on policy.
	expectedSustaining := false
	expectedAdaptiveChargingToggle := false

	switch chargingOptimization {
	case 1: // Standard
		expectedSustaining = false
		expectedAdaptiveChargingToggle = false
	case 2: // Adaptive
		expectedSustaining = true
		expectedAdaptiveChargingToggle = true
	case 3: // Limited
		expectedSustaining = true
		expectedAdaptiveChargingToggle = false
	}

	if err := pollUntilBatterySustainingState(ctx, expectedSustaining); err != nil {
		s.Errorf("Battery sustaining state mismatch. Charging Optimization: %d, Expected: %t", chargingOptimization, expectedSustaining)
	}

	// Verify Adaptive charging toggle.
	if err := verifyAdaptiveChargingToggle(ctx, s, cr, tconn, expectedAdaptiveChargingToggle); err != nil {
		s.Errorf("Adaptive charging toggle mismatch. Charging Optimization: %d, Expected: %t", chargingOptimization, expectedAdaptiveChargingToggle)
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

func verifyAdaptiveChargingToggle(ctx context.Context, s *testing.State, cr *chrome.Chrome, tconn *chrome.TestConn, expectedChecked bool) error {
	ui := uiauto.New(tconn)
	toggleAdaptiveCharging := nodewith.Name("Adaptive charging").Role(role.ToggleButton)
	settings, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, "power", ui.WaitUntilCheckedState(toggleAdaptiveCharging, expectedChecked))
	if err != nil {
		return err
	}
	defer settings.Close(ctx)

	return nil
}

// createFakeChargeHistoryData creates and populates `dir` with fake charge history values.
func createFakeChargeHistoryData(s *testing.State, dir string) {
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
		if err := os.WriteFile(filepath.Join(dir, strconv.FormatInt(dateMicroseconds, 10)), contents, 0600); err != nil {
			s.Fatal("Failed to write charge history file: ", err)
		}
	}
}
