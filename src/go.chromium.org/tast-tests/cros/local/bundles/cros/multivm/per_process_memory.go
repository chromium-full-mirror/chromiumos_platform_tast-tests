// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package multivm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/memory"
	memoryarc "go.chromium.org/tast-tests/cros/local/memory/arc"
	"go.chromium.org/tast-tests/cros/local/multivm"
	"go.chromium.org/tast-tests/cros/local/session"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"golang.org/x/exp/slices"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PerProcessMemory,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Starts session and records per-process memory usage",
		Contacts: []string{
			"arcvm-eng-team@google.com",
			"yixie@google.com",
		},
		BugComponent: "b:168382",
		Attr:         []string{"group:crosbolt", "crosbolt_nightly"},
		Timeout:      20 * time.Minute,
		SoftwareDeps: []string{"chrome", "android_vm"},
		Params: []testing.Param{{
			Name: "arc",
			Val: &stateManagerOptions{
				chromeOptions: multivm.DefaultChromeOptions,
				vmOptions:     []multivm.VMOptions{multivm.DefaultARCOptions},
			},
		}, {
			Name:              "arc_lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val: &stateManagerOptions{
				chromeOptions: multivm.LacrosChromeOptions,
				vmOptions:     []multivm.VMOptions{multivm.DefaultARCOptions},
			},
		}},
	})
}

// stateManagerOptions stores options to create multivm.StateManager instance.
type stateManagerOptions struct {
	chromeOptions multivm.ChromeOptions
	vmOptions     []multivm.VMOptions
}

// perProcessMemoryResult stores results from the test to convert to json.
type perProcessMemoryResult struct {
	SmapsRollups memory.FullSmapsRollup
	ExtraMetrics map[string]uint64
}

const (
	// Times to repeat the test and measure memory usage, in order to observe
	// the fluctuation between multiple test runs.
	totalRuns = 3
	// Time to wait before measuring memory usage after login.
	waitTimeBeforeMeasure = 60 * time.Second
)

func PerProcessMemory(ctx context.Context, s *testing.State) {
	opts := s.Param().(*stateManagerOptions)

	// Fetching smaps_rollup inside ARCVM requires adb root.
	if err := arc.AppendToArcvmDevConf(ctx, "--params=androidboot.verifiedbootstate=orange"); err != nil {
		s.Fatal("Failed to enable adb root: ", err)
	}
	defer arc.RestoreArcvmDevConf(ctx)

	m := multivm.NewStateManager(opts.chromeOptions, opts.vmOptions...).SetForceActivate(true)
	defer m.Deactivate(ctx)

	lastSessionManagerPid := 0

	var resultFiles []string
	for run := 1; run <= totalRuns; run++ {
		s.Logf("Starting run %d/%d", run, totalRuns)

		if err := m.Activate(ctx, s); err != nil {
			s.Fatal("Could not activate state: ", err)
		}

		if err := m.Chrome().Responded(ctx); err != nil {
			s.Fatal("Chrome did not respond: ", err)
		}
		a := arcFromStateManager(m)

		// Ensure the session was restarted by checking session manager PID.
		pid, err := session.GetSessionManagerPID()
		if err != nil {
			s.Fatal("Failed to get session manager PID: ", err)
		}
		if pid == lastSessionManagerPid {
			s.Fatal("Session manager was not restarted")
		}
		lastSessionManagerPid = pid

		if a != nil {
			// Ensure package manager service is running by checking the
			// existence of the "android" package.
			pkgs, err := a.InstalledPackages(ctx)
			if err != nil {
				s.Fatal("Getting installed packages failed: ", err)
			}

			if _, ok := pkgs["android"]; !ok {
				s.Fatal("Android package not found: ", pkgs)
			}
		}

		s.Log("Wait before collecting metrics")
		// GoBigSleepLint: Let the system quiesce for a while and measure its
		// memory consumption.
		if err := testing.Sleep(ctx, waitTimeBeforeMeasure); err != nil {
			s.Fatal("Failed to wait before collecting metrics")
		}
		s.Log("Will now collect idle perf values")

		smapsRollup, err := memory.PerProcessSmapsRollup(ctx, a != nil)
		if err != nil {
			s.Fatal("Failed to get full smaps_rollup: ", err)
		}

		result := &perProcessMemoryResult{
			SmapsRollups: *smapsRollup,
			ExtraMetrics: make(map[string]uint64),
		}

		runSuffix := fmt.Sprintf("_%d", run)

		if a != nil {
			extra, err := extraArcMemoryMetrics(ctx, a, s.OutDir(), runSuffix)
			if err != nil {
				s.Fatal("Failed to get ARCVM extra memory metrics: ", err)
			}
			for k, v := range extra {
				result.ExtraMetrics[k] = v
			}
		}

		extraHostMetrics, err := extraHostMemoryMetrics(ctx, s.OutDir(), runSuffix)
		if err != nil {
			s.Fatal("Failed to fetch extra host metrics: ", err)
		}
		for k, v := range extraHostMetrics {
			result.ExtraMetrics[k] = v
		}

		buf, err := json.Marshal(result)
		if err != nil {
			s.Fatal("Failed to marshal json: ", err)
		}

		resultFile := filepath.Join(s.OutDir(), fmt.Sprintf("smaps_full_%d.json", run))
		if err := os.WriteFile(resultFile, buf, 0644); err != nil {
			s.Fatal("Failed to save json: ", err)
		}
		resultFiles = append(resultFiles, resultFile)

		if err := memory.SaveRawSmaps(ctx, s.OutDir(), runSuffix, smapsRollup, a != nil); err != nil {
			s.Fatal("Failed to dump raw smaps: ", err)
		}

		if err := m.Deactivate(ctx); err != nil {
			s.Fatal("Failed to deactivate state: ", err)
		}
	}

	if err := generateResultsChart(resultFiles, s.OutDir()); err != nil {
		s.Fatal("Failed to generate results chart for crosbolt: ", err)
	}
}

// arcFromStateManager finds the arc.ARC instance from multivm.StateManager.
func arcFromStateManager(m *multivm.StateManager) *arc.ARC {
	if vm, ok := m.VMs()[multivm.ARCName]; ok {
		return vm.(*arc.ARC)
	}
	return nil
}

// extraArcMemoryMetrics returns a map containing extra memory metrics including
// free memory in ARCVM. Sizes are in kilobytes.
func extraArcMemoryMetrics(ctx context.Context, a *arc.ARC, outdir, suffix string) (map[string]uint64, error) {
	var vmSummary *memoryarc.VMSummary
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var err error
		vmSummary, err = memoryarc.GetDumpsysMeminfoMetrics(ctx, a, outdir, suffix)
		return err
	}, &testing.PollOptions{Timeout: time.Minute, Interval: 5 * time.Second}); err != nil {
		return nil, errors.Wrap(err, "failed to collect ARC dumpsys meminfo metrics")
	}

	extraMetrics := make(map[string]uint64)
	extraMetrics["ARC_Free"] = uint64(vmSummary.FreeRAM)
	extraMetrics["ARC_CachedKernel"] = vmSummary.CachedKernel
	extraMetrics["ARC_CachedPss"] = vmSummary.CachedPss
	extraMetrics["ARC_UserPss"] = vmSummary.UsedPss
	extraMetrics["ARC_KernelPss"] = vmSummary.KernelPss
	extraMetrics["ARC_Lost"] = uint64(vmSummary.LostRAM)

	return extraMetrics, nil
}

// extraHostMemoryMetrics returns a map containing extra memory metrics on the
// host that we are interested in tracking. Sizes are in kilobytes.
func extraHostMemoryMetrics(ctx context.Context, outdir, suffix string) (map[string]uint64, error) {
	const (
		dmaBufInfoFile     = "/sys/kernel/debug/dma_buf/bufinfo"
		i915GemObjectsFile = "/sys/kernel/debug/dri/0/i915_gem_objects"
		messagesFile       = "/var/log/messages"
	)

	metrics := make(map[string]uint64)

	// Get GPU dma_buf memory metrics.
	totalBufBytesRe := regexp.MustCompile(`Total \d+ objects, (\d+) bytes`)
	bufinfoSize, err := extractUint64FromFile(dmaBufInfoFile, outdir, suffix, totalBufBytesRe)
	if err != nil {
		return nil, errors.Wrap(err, "failed to extract bufinfo size")
	}
	// DMA buf size is in bytes, use KB to align with other metrics.
	metrics["Host_GPU_DMABufSize"] = bufinfoSize / 1024

	// Get GPU i915_gem_objects memory metrics if available.
	gemObjectsBytesRe := regexp.MustCompile(`\d+ shrinkable \[\d+ free\] objects, (\d+) bytes`)
	gemObjectsBytes, err := extractUint64FromFile(i915GemObjectsFile, outdir, suffix, gemObjectsBytesRe)
	if err == nil {
		// i915 GEM objects size is in bytes, use KB to align with other metrics
		metrics["Host_GPU_I915GEMObjects"] = gemObjectsBytes / 1024
	} else {
		testing.ContextLog(ctx, "Error extracting i915_gem_objects size, expected on non-Intel devices: ", err)
	}

	// Extract the last balloon size from /var/log/messages if available.
	// Example: BalloonTrace[arcvm,,1045,193,394,1577,523,618]
	//                              ^
	balloonSizeRe := regexp.MustCompile(`BalloonTrace\[arcvm,[^,]*,(\d+),[\d-]*,[\d-]*,[\d-]*,[\d-]*,[\d-]*\]`)
	balloonSize, err := extractUint64FromFile(messagesFile, "", "", balloonSizeRe)
	if err == nil {
		// Extracted balloon size is in MB, use KB to align with other metrics
		metrics["Host_BalloonSize"] = balloonSize * 1024
	} else {
		testing.ContextLog(ctx, "Failed to extract balloon size: ", err)
	}

	return metrics, nil
}

// extractUint64FromFile extracts a single uint64 value from contents of a file.
// Only 1 capture group is expected in the regexp.
// A copy of the file is saved to outdir with suffix appended to file name for
// debug purpose if outdir is not empty. If multiple matches are found, use the
// last one.
func extractUint64FromFile(file, outdir, suffix string, re *regexp.Regexp) (uint64, error) {
	buf, err := os.ReadFile(file)
	if err != nil {
		return 0, errors.Wrap(err, "failed to read "+file)
	}

	if outdir != "" {
		baseName := filepath.Base(file)
		if err := os.WriteFile(path.Join(outdir, baseName+suffix), buf, 0644); err != nil {
			return 0, errors.Wrap(err, "failed to save copy of "+baseName)
		}
	}

	matches := re.FindAllStringSubmatch(string(buf), -1)
	if len(matches) == 0 {
		return 0, errors.New("failed to parse " + file)
	}
	group := matches[len(matches)-1]
	if len(group) != 2 {
		return 0, errors.Errorf("expected group size to be 2, but is %d", len(group))
	}
	val, err := strconv.ParseUint(group[1], 10, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to parse value: %s", group[1])
	}

	return val, nil
}

// generateResultsChart calculates the median value of smaps_rollup for every
// process, then generates results compatible with crosbolt dashboard.
func generateResultsChart(resultFiles []string, outdir string) error {
	biggerIsBetterKeys := []string{"ARC_Free", "ARC_Lost"}

	allResultsMap := make(map[string][]uint64)
	illegalChars := regexp.MustCompile(`[^a-zA-Z0-9._-]`)

	fillResultsMap := func(rollups []*memory.NamedSmapsRollup, prefix string) {
		// Multiple processes can have the same name, so we add them up first.
		runResultsMap := make(map[string]uint64)
		for _, r := range rollups {
			processName := r.Command
			// Android HAL process names contain HAL version after `@` in
			// suffix which is not allowed in crosbolt metric name
			// (e.g. `@6.0-service-arc`). Let's remove the version.
			processName = strings.Split(processName, "@")[0]
			// Replace other illegal metric name characters with `_`.
			processName = illegalChars.ReplaceAllString(processName, "_")

			for k, v := range r.Rollup {
				// Only output Pss values to crosbolt.
				if k != "Pss" && k != "SwapPss" {
					continue
				}

				// Adding the median of Pss and the median of SwapPss will not
				// result in the same value as the median of Pss+SwapPss.
				// Therefore we calculate the sum of them here to get accurate
				// median TotalPss.
				name := fmt.Sprintf("%s%s_TotalPss", prefix, processName)
				runResultsMap[name] += v

				// In addition to TotalPss, also output Pss to crosbolt.
				if k != "Pss" {
					continue
				}

				name = fmt.Sprintf("%s%s_Pss", prefix, processName)
				runResultsMap[name] += v
			}
		}

		for k, v := range runResultsMap {
			allResultsMap[k] = append(allResultsMap[k], v)
		}
	}

	for _, resultFile := range resultFiles {
		buf, err := os.ReadFile(resultFile)
		if err != nil {
			return errors.Wrap(err, "unable to read "+resultFile)
		}

		result := &perProcessMemoryResult{}
		if err := json.Unmarshal(buf, result); err != nil {
			return errors.Wrap(err, "unable to parse "+resultFile)
		}

		fillResultsMap(result.SmapsRollups.HostRollups, "Host_")
		for _, r := range result.SmapsRollups.CrosVMRollups {
			fillResultsMap(r.Rollups, r.Tag+"_")
		}
		fillResultsMap(result.SmapsRollups.ARCVMRollups, "ARC_")

		for k, v := range result.ExtraMetrics {
			allResultsMap[k] = append(allResultsMap[k], v)
		}
	}

	values := perf.NewValues()
	for k, v := range allResultsMap {
		slices.Sort(v)
		median := (v[(len(v)-1)/2] + v[len(v)/2]) / 2
		direction := perf.SmallerIsBetter
		if slices.Contains(biggerIsBetterKeys, k) {
			direction = perf.BiggerIsBetter
		}
		values.Set(perf.Metric{
			Name:      k,
			Unit:      "KiB",
			Direction: direction,
			Multiple:  false,
		}, float64(median))
	}

	return values.Save(outdir)
}
