// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	ps "go.chromium.org/tast-tests/cros/common/power/powerpb"
	sp "go.chromium.org/tast-tests/cros/services/cros/power"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/meta/tastrun"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

/**
Wrapper around other tast tests which (dis)charges the battery and disconnects the
charger for the test duration.

Example:
tast run -var "meta.BatteryTestWrapper.minCharge=10" \
         -var "meta.BatteryTestWrapper.maxCharge=15" \
		 -var "meta.BatteryTestWrapper.dischargeAfterTest=true" \
		 -var "meta.BatteryTestWrapper.subtest=platform.BootPerf" \
		 -var "meta.BatteryTestWrapper.subtestVars=platform.BootPerf.iterations:10;subtest.platform.BootPerf.skipRootfsCheck:false" \
		 $DUT_IP meta.BatteryTestWrapper

*/

const (
	minChargeDefaultValue          = "0"
	maxChargeDefaultValue          = "0"
	dischargeAfterTestDefaultValue = "false"
	subtestDefaultValue            = ""
	subtestVarsDefaultValue        = ""
)

var minChargeVar = testing.RegisterVarString(
	"meta.BatteryTestWrapper.minCharge",
	minChargeDefaultValue,
	"Minimum charge percentage to (dis)charge the battery to",
)
var maxChargeVar = testing.RegisterVarString(
	"meta.BatteryTestWrapper.maxCharge",
	maxChargeDefaultValue,
	"Maximum charge percentage to (dis)charge the battery to",
)
var dischargeAfterTestVar = testing.RegisterVarString(
	"meta.BatteryTestWrapper.dischargeAfterTest",
	dischargeAfterTestDefaultValue,
	"If true, battery will not be re-charged to the initial level after the test",
)
var subtestVar = testing.RegisterVarString(
	"meta.BatteryTestWrapper.subtest",
	subtestDefaultValue,
	"Test to run after preparing the battery Overridden by subtest",
)
var subtestVarsVar = testing.RegisterVarString(
	"meta.BatteryTestWrapper.subtestVars",
	subtestVarsDefaultValue,
	"List of variables to pass to the subtest in form of 'var1:val1;var2:val2'",
)

type batteryTestWrapperParams struct {
	// Minimum charge percentage to (dis)charge the battery to.
	// Overridden by meta.BatteryTestWrapper.minCharge.
	minCharge float64
	// Maximum charge percentage to (dis)charge the battery to.
	// Overridden by meta.BatteryTestWrapper.maxCharge.
	maxCharge float64
	// If true, battery will not be re-charged to the initial level after the test.
	// Overridden by meta.BatteryTestWrapper.dischargeAfterTest.
	dischargeAfterTest bool
	// Test to run after preparing the battery.
	// Overridden by meta.BatteryTestWrapper.subtest.
	subtest string
	// List of variables to pass to the subtest.
	// Overridden by meta.BatteryTestWrapper.subtestVars in form of "var1:val1;var2:val2".
	subtestVars []string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         BatteryTestWrapper,
		Desc:         "Runs test after (dis)charging the battery and unplugging the charger",
		LacrosStatus: testing.LacrosVariantUnneeded,
		BugComponent: "b:167191", // ChromeOS > Platform > System > Power
		Contacts:     []string{"cros-pe-pnp@google.com", "skardach@google.com"},
		Timeout:      24 * time.Hour, // Depends on subtest, so set maximum value here.
		Params: []testing.Param{
			{ // By default, work on user-provided vars. Special pre-configured cases go below.
				Name: "",
				Val:  batteryTestWrapperParams{},
			},
			{ // Boot time just above the battery saver conditions.
				Name: "boot_perf_15p",
				Val: batteryTestWrapperParams{
					minCharge:          25.0,
					maxCharge:          28.0,
					dischargeAfterTest: true,
					subtest:            "platform.BootPerf",
					subtestVars:        []string{"platform.BootPerf.iterations=10"},
				},
			},
			{ // Boot time check under 50% (see b/324398956).
				Name: "boot_perf_40p",
				Val: batteryTestWrapperParams{
					minCharge:          35.0,
					maxCharge:          40.0,
					dischargeAfterTest: true,
					subtest:            "platform.BootPerf",
					subtestVars:        []string{"platform.BootPerf.iterations=10"},
				},
			},
			{ // Boot time check on an almost full battery (see b/324398956).
				Name: "boot_perf_80p",
				Val: batteryTestWrapperParams{
					minCharge:          75.0,
					maxCharge:          80.0,
					dischargeAfterTest: true,
					subtest:            "platform.BootPerf",
					subtestVars:        []string{"platform.BootPerf.iterations=10"},
				},
			},
		},
		ServiceDeps: []string{
			"tast.common.power.powerpb.LocalInfoService",
			"tast.cros.power.BatteryService",
		},
	})
}

func BatteryTestWrapper(ctx context.Context, s *testing.State) {
	param, ok := s.Param().(batteryTestWrapperParams)
	if !ok {
		s.Fatal("Failed to convert test testParams")
	}

	var err error // helper for un-scoped assignments
	// Start with the test variant defaults
	minCharge := param.minCharge
	maxCharge := param.maxCharge
	dischargeAfterTest := param.dischargeAfterTest
	subtest := param.subtest
	subtestVars := param.subtestVars

	// Now get the variable overrides
	// If provided, both min and max charge levels are required to prevent mistakes.
	minChargeStr := minChargeVar.Value()
	maxChargeStr := maxChargeVar.Value()
	if minChargeStr != minChargeDefaultValue && maxChargeStr != maxChargeDefaultValue {
		if minCharge, err = strconv.ParseFloat(strings.TrimSpace(string(minChargeStr)), 32); err != nil {
			s.Fatalf("Failed to parse minCharge percentage from %q", minChargeStr)
		}
		if maxCharge, err = strconv.ParseFloat(strings.TrimSpace(string(maxChargeStr)), 32); err != nil {
			s.Fatalf("Failed to parse maxCharge percentage from %q", maxChargeStr)
		}
	} else {
		s.Fatal("Both minCharge and maxCharge have to be provided")
	}
	if dischargeAfterTestStr := dischargeAfterTestVar.Value(); dischargeAfterTestStr != dischargeAfterTestDefaultValue {
		if dischargeAfterTest, err = strconv.ParseBool(strings.TrimSpace(string(dischargeAfterTestStr))); err != nil {
			s.Fatalf("Failed to parse dischargeAfterTest from %q", dischargeAfterTestStr)
		}
	}
	// Check if we have a subtest override from command line. Make sure we have something to run.
	if subtestStr := subtestVar.Value(); subtestStr != subtestDefaultValue {
		subtest = subtestStr
	}
	if subtest == "" {
		s.Fatal("Please specify a subtest or use a pre-defined subtest variant")
	}
	// Parse subtest variables. Syntax is: var1:val1;var2:val2;...
	if subtestVarsStr := subtestVarsVar.Value(); subtestVarsStr != subtestVarsDefaultValue {
		for _, variable := range strings.Split(subtestVarsStr, ";") {
			subtestVars = append(subtestVars, strings.Replace(variable, ":", "=", 1))
		}
	}
	// Now convert vars into flags for the subtest
	var flags []string
	for _, variable := range subtestVars {
		flags = append(flags, "-var="+variable)
	}

	cleanupTime := 1 * time.Minute
	// If we need to re-charge the battery, it might take longer (up to 3 hours)
	if !dischargeAfterTest {
		cleanupTime += 3 * time.Hour
	}
	cleanupCtx, cancel := ctxutil.Shorten(ctx, cleanupTime)
	defer cancel()

	// commandCtx is used for running subtest.
	commandCtx, commandCancel := ctxutil.Shorten(ctx, cleanupTime)
	defer commandCancel()

	s.Log("Subtest: ", subtest)
	//s.Log("Subtest vars: ", subtestVars)

	// Connect to RPC service clients.
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(cleanupCtx)

	// If necessary, charge/discharge the battery and disconnect the charger.
	infoService := ps.NewLocalInfoServiceClient(cl.Conn)
	var powerStatus *ps.Status
	if powerStatus, err = infoService.GetPowerStatus(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to read the current battery charge level. Error: ", err)
	}
	initialCharge := powerStatus.BatteryPercent
	initiallyDischarging := powerStatus.BatteryDischarging
	chargeParams := sp.BatteryRequest{
		MinPercentage:         float32(minCharge),
		MaxPercentage:         float32(maxCharge),
		DischargeOnCompletion: true,
	}
	batteryService := sp.NewBatteryServiceClient(cl.Conn)
	if _, err = batteryService.PrepareBattery(ctx, &chargeParams); err != nil {
		s.Fatal("Failed to (dis)charge the battery. Error: ", err)
	}

	defer func(ctx context.Context) {
		// Wait just in case the test could break the connectivity (e.g. boot or suspend).
		d := s.DUT()
		if err := d.WaitConnect(ctx); err != nil {
			s.Log("Failed to connect to the DUT: ", err)
			return
		}

		// Connect to RPC service clients.
		cl, err := rpc.Dial(ctx, d, s.RPCHint())
		if err != nil {
			s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
		}
		defer cl.Close(ctx)

		batteryService := sp.NewBatteryServiceClient(cl.Conn)
		// Re-charge the battery...
		if !dischargeAfterTest {
			// Re-charge within 5% of the initial charge to prevent test hanging around 99-100%.
			minCharge := initialCharge - 5
			maxCharge := initialCharge
			// Unless the device was already below critical, then charge up a bit more.
			if initialCharge < 5 {
				minCharge = 10
				maxCharge = 15
			}
			s.Logf("Recharging battery to %f%%-%f%% range", minCharge, maxCharge)
			chargeParams := sp.BatteryRequest{
				MinPercentage:         float32(minCharge),
				MaxPercentage:         float32(maxCharge),
				DischargeOnCompletion: initiallyDischarging,
			}
			if _, err = batteryService.PrepareBattery(ctx, &chargeParams); err != nil {
				s.Fatal("Failed to re-charge the battery to initial state. Error: ", err)
			}
		} else if !initiallyDischarging { // ... or return charging to initial state.
			s.Log("Re-connecting the charger")
			if _, err = batteryService.AllowBatteryCharging(ctx, &empty.Empty{}); err != nil {
				s.Fatal("Failed to re-enable battery charging. Error: ", err)
			}
		}
	}(cleanupCtx)

	s.Log("Starting subtest: ", subtest)
	resultsDir := filepath.Join(s.OutDir(), "subtest_results")
	skippedTests := tastrun.RunAndEvaluate(commandCtx, s, flags, []string{subtest}, resultsDir, tastrun.SkipPolicyDisallowSkipping)
	if len(skippedTests) > 0 {
		s.Fatal("Test is skipped, abort post-processing")
	}

	select {
	case <-commandCtx.Done():
		s.Fatal("Subtest is cancelled")
	default:
		// Not canceled in servo goroutine.
		s.Log("Finished subtest: ", subtest)
	}

}
