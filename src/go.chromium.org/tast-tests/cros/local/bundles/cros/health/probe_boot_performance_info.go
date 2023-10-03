// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package health

import (
	"context"
	"math"
	"time"

	"go.chromium.org/tast-tests/cros/local/croshealthd"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type bootPerformanceInfo struct {
	BootUpSeconds              float64 `json:"boot_up_seconds"`
	BootUpTimestamp            float64 `json:"boot_up_timestamp"`
	ShutdownSeconds            float64 `json:"shutdown_seconds"`
	ShutdownTimestamp          float64 `json:"shutdown_timestamp"`
	ShutdownReason             string  `json:"shutdown_reason"`
	TpmInitializationSeconds   float64 `json:"tpm_initialization_seconds"`
	PowerOnToKernelSeconds     float64 `json:"power_on_to_kernel_seconds"`
	KernelToPreStartupSeconds  float64 `json:"kernel_to_pre_startup_seconds"`
	KernelToPostStartupSeconds float64 `json:"kernel_to_post_startup_seconds"`
	StartupToChromeExecSeconds float64 `json:"startup_to_chrome_exec_seconds"`
	ChromeExecToLoginSeconds   float64 `json:"chrome_exec_to_login_seconds"`
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ProbeBootPerformanceInfo,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check that we can probe cros_healthd for boot performance info",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
			"kerker@google.com",
		},
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		Attr:         []string{"group:mainline"},
		// Skip on reven board (ChromeOS Flex) because the boot
		// performance metrics are not supported on it.
		SoftwareDeps: []string{"chrome", "diagnostics", "boot_perf_info"},
		Fixture:      "crosHealthdRunning",
	})
}

func getBootPerformanceData(ctx context.Context, outDir string) (bootPerformanceInfo, error) {
	var bootPerf bootPerformanceInfo
	params := croshealthd.TelemParams{Category: croshealthd.TelemCategoryBootPerformance}
	err := croshealthd.RunAndParseJSONTelem(ctx, params, outDir, &bootPerf)

	return bootPerf, err
}

func validateBootPerformanceData(bootPerf *bootPerformanceInfo) error {
	if bootPerf.BootUpSeconds < 0.5 {
		return errors.New("it is impossible that boot_up_seconds is less than 0.5")
	}
	if bootPerf.BootUpTimestamp < 0.5 {
		return errors.New("it is impossible that boot_up_timestamp is less than 0.5")
	}
	if len(bootPerf.ShutdownReason) == 0 {
		return errors.New("shutdown_reason should not be empty string")
	}
	if bootPerf.TpmInitializationSeconds < 0.000001 {
		return errors.New("it is impossible that tpm_initialization_seconds is less than 0.000001")
	}
	if bootPerf.PowerOnToKernelSeconds < 0.000001 {
		return errors.New("it is impossible power_on_to_kernel_seconds is less than 0.000001")
	}
	if bootPerf.KernelToPreStartupSeconds < 0.000001 {
		return errors.New("it is impossible kernel_to_pre_startup_seconds is less than 0.000001")
	}
	if bootPerf.KernelToPostStartupSeconds < 0.000001 {
		return errors.New("it is impossible kernel_to_post_startup_seconds is less than 0.000001")
	}
	if bootPerf.StartupToChromeExecSeconds < 0.000001 {
		return errors.New("it is impossible startup_to_chrome_exec_seconds is less than 0.000001")
	}
	if bootPerf.ChromeExecToLoginSeconds < 0.000001 {
		return errors.New("it is impossible chrome_exec_to_login_seconds is less than 0.000001")
	}

	return nil
}

func ProbeBootPerformanceInfo(ctx context.Context, s *testing.State) {
	var bootPerf bootPerformanceInfo
	var err error
	if bootPerf, err = getBootPerformanceData(ctx, s.OutDir()); err != nil {
		s.Fatal("Failed to get boot performance telemetry info: ", err)
	}

	if err = validateBootPerformanceData(&bootPerf); err != nil {
		s.Fatal("Failed to validate boot performance data: ", err)
	}

	// Sleep 5 seconds, fetch data again. "boot_up_timestamp" should be the same.
	// GoBigSleepLint: It's used to verify "boot_up_timestamp" doesn't change with time.
	if err = testing.Sleep(ctx, 5*time.Second); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	var bootPerfNew bootPerformanceInfo
	if bootPerfNew, err = getBootPerformanceData(ctx, s.OutDir()); err != nil {
		s.Fatal("Failed to get boot performance telemetry info: ", err)
	}

	if err = validateBootPerformanceData(&bootPerfNew); err != nil {
		s.Fatal("Failed to validate boot performance data: ", err)
	}

	if math.Abs(bootPerf.BootUpTimestamp-bootPerfNew.BootUpTimestamp) > 1 {
		s.Errorf("Failed as difference between boot_up_timestamp (%v) and new boot_up_timestamp (%v) is greater than 1", bootPerf.BootUpTimestamp, bootPerfNew.BootUpTimestamp)
	}
}
