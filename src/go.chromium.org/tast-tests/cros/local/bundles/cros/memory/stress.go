// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"context"
	"io/ioutil"
	"math/rand"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/memory/memorystress"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type testParams struct {
	isLacros bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         Stress,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Create heavy memory pressure and check if oom-killer is invoked",
		Contacts:     []string{"chromeos-memory@google.com"},
		BugComponent: "b:167286",
		// This test takes 15-30 minutes to run.
		Timeout:      45 * time.Minute,
		SoftwareDeps: []string{"chrome"},
		Vars: []string{
			"memory.Stress.enableARC",
			"memory.Stress.minFilelistKB",
			"memory.Stress.seed",
			"memory.Stress.useHugePages",
		},
		Params: []testing.Param{{
			ExtraAttr:         []string{"group:crosbolt", "crosbolt_memory_nightly"},
			ExtraSoftwareDeps: []string{"android_container"},
			Val: testParams{
				isLacros: false,
			},
		}, {
			Name:              "vm",
			ExtraAttr:         []string{"group:crosbolt", "crosbolt_memory_nightly"},
			ExtraSoftwareDeps: []string{"android_vm"},
			Val: testParams{
				isLacros: false,
			},
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           "lacros",
			Val: testParams{
				isLacros: true,
			},
		}},
	})

}

func Stress(ctx context.Context, s *testing.State) {
	minFilelistKB := -1
	if val, ok := s.Var("memory.Stress.minFilelistKB"); ok {
		val, err := strconv.Atoi(val)
		if err != nil {
			s.Fatal("Cannot parse argument memory.Stress.minFilelistKB: ", err)
		}
		minFilelistKB = val
	}
	s.Log("minFilelistKB: ", minFilelistKB)

	// The memory pressure is higher when ARC is enabled (without launching Android apps).
	// Checks the ARC enabled case by default.
	enableARC := true
	if val, ok := s.Var("memory.Stress.enableARC"); ok {
		boolVal, err := strconv.ParseBool(val)
		if err != nil {
			s.Fatal("Cannot parse argument memory.Stress.enableARC: ", err)
		}
		enableARC = boolVal
	}
	s.Log("enableARC: ", enableARC)

	useHugePages := false
	if val, ok := s.Var("memory.Stress.useHugePages"); ok {
		boolVal, err := strconv.ParseBool(val)
		if err != nil {
			s.Fatal("Cannot parse argument memory.Stress.useHugePages: ", err)
		}
		useHugePages = boolVal
	}
	s.Log("useHugePages: ", useHugePages)

	seed := time.Now().UTC().UnixNano()
	if val, ok := s.Var("memory.Stress.seed"); ok {
		intVal, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			s.Fatal("Cannot parse argument memory.Stress.seed: ", err)
		}
		seed = intVal
	}
	s.Log("Seed: ", seed)
	localRand := rand.New(rand.NewSource(seed))

	perfValues := perf.NewValues()

	const mbPerTab = 800
	if s.Param().(testParams).isLacros {
		if err := lacrosMain(ctx, s, localRand, mbPerTab, perfValues); err != nil {
			s.Fatal("lacrosMain failed: ", err)
		}
	} else {
		if err := stressMain(ctx, localRand, mbPerTab, minFilelistKB, enableARC, useHugePages, perfValues); err != nil {
			s.Fatal("stressMain failed: ", err)
		}
	}

	if err := perfValues.Save(s.OutDir()); err != nil {
		s.Error("Cannot save perf data: ", err)
	}
}

func stressMain(ctx context.Context, localRand *rand.Rand, mbPerTab, minFilelistKB int, enableARC, useHugePages bool, perfValues *perf.Values) error {
	// Tests both the low compress ratio and high compress ratio cases.
	// When there is more random data (67 percent random), the compress ratio is low,
	// the low memory notification is triggered by low uncompressed anonymous memory.
	// When there is less random data (33 percent random), the compress ratio is high,
	// the low memory notification is triggered by low swap free.
	const switchCount = 150
	result67, err := stressTestCase(ctx, localRand, mbPerTab, switchCount, minFilelistKB, 0.67, enableARC, useHugePages)
	if err != nil {
		return errors.Wrap(err, "67_percent_random test case failed")
	}
	result33, err := stressTestCase(ctx, localRand, mbPerTab, switchCount, minFilelistKB, 0.33, enableARC, useHugePages)
	if err != nil {
		return errors.Wrap(err, "33_percent_random test case failed")
	}

	if err := memorystress.ReportTestCaseResult(ctx, perfValues, result67, "67_percent_random"); err != nil {
		return errors.Wrap(err, "reporting 67_percent_random failed")
	}
	if err := memorystress.ReportTestCaseResult(ctx, perfValues, result33, "33_percent_random"); err != nil {
		return errors.Wrap(err, "reporting 33_percent_random failed")
	}

	return nil
}

func stressTestCase(ctx context.Context, localRand *rand.Rand, mbPerTab, switchCount, minFilelistKB int, compressRatio float32, enableARC, useHugePages bool) (memorystress.TestCaseResult, error) {
	var opts []chrome.Option
	if enableARC {
		opts = append(opts, chrome.ARCEnabled())
	}
	if useHugePages {
		opts = append(opts, chrome.HugePagesEnabled())
	}
	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		return memorystress.TestCaseResult{}, errors.Wrap(err, "cannot start chrome")
	}

	if err := cpu.WaitUntilIdle(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to wait for idle CPU: ", err)
	}

	// Setup min_filelist_kbytes after chrome start.
	if minFilelistKB >= 0 {
		if err := ioutil.WriteFile("/proc/sys/vm/min_filelist_kbytes", []byte(strconv.Itoa(minFilelistKB)), 0644); err != nil {
			return memorystress.TestCaseResult{}, errors.Wrap(err, "could not write to /proc/sys/vm/min_filelist_kbytes")
		}
	}

	return memorystress.TestCase(ctx, cr.Browser(), localRand, mbPerTab, switchCount, compressRatio)
}

func lacrosMain(ctx context.Context, s *testing.State, localRand *rand.Rand, mbPerTab int, perfValues *perf.Values) error {
	// TODO(b/191105438): Tune Lacros variation when Lacros tab discarder is mature.
	tconn, err := s.FixtValue().(chrome.HasChrome).Chrome().TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	lacros, err := lacros.Launch(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to launch lacros-chrome")
	}

	if err := cpu.WaitUntilIdle(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for idle CPU")
	}

	const switchCount = 10
	const compressRatio = 0.67
	result, err := memorystress.TestCase(ctx, lacros.Browser(), localRand, mbPerTab, switchCount, compressRatio)
	if err != nil {
		return errors.Wrap(err, "memorystress test case failed")
	}

	if err := memorystress.ReportTestCaseResult(ctx, perfValues, result, "stress"); err != nil {
		return errors.Wrap(err, "reporting result failed")
	}

	return nil
}
