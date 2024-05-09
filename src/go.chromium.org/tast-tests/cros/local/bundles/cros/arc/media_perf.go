// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/mediaperf"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"

	"go.chromium.org/tast/core/errors"
)

var (
	// mediaPerfBooted is a precondition similar to arc.Booted() with no opt-in and disables some heavy Android activities that use system resources.
	mediaPerfBooted = arc.NewPrecondition("mediaperf_booted", nil /* GAIAVARS */, nil /* GAIALOGINPOOLVARS */, false /* O_DIRECT */)
)

// testParameters contains all the data needed to run a single test iteration.
type mediaPerfTestParameters struct {
	binaryTranslation bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:           MediaPerf,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		LacrosStatus:   testing.LacrosVariantExists,
		Desc:           "Captures set of media performance metrics and uploads them as perf metrics",
		Contacts: []string{
			"arc-performance@google.com",
			"jasondchen@google.com",
			"alanding@google.com",
		},
		// ChromeOS > Software > ARC++ > Performance
		BugComponent: "b:168382",
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{"chrome"},
		Data:         []string{mediaperf.X86ApkName, mediaperf.ArmApkName},
		Timeout:      35 * time.Minute,
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
			Val: mediaPerfTestParameters{
				binaryTranslation: true,
			},
			Pre: mediaPerfBooted,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"android_container", "lacros"},
			Val: mediaPerfTestParameters{
				binaryTranslation: true,
			},
			Pre: mediaPerfBooted,
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
			Val: mediaPerfTestParameters{
				binaryTranslation: true,
			},
			Pre: mediaPerfBooted,
		}, {
			Name:              "vm_lacros",
			ExtraSoftwareDeps: []string{"android_vm", "lacros"},
			Val: mediaPerfTestParameters{
				binaryTranslation: true,
			},
			Pre: mediaPerfBooted,
		}, {
			Name:              "x86",
			ExtraSoftwareDeps: []string{"android_container"},
			ExtraHardwareDeps: hwdep.D(hwdep.X86()),
			Val: mediaPerfTestParameters{
				binaryTranslation: false,
			},
			Pre: mediaPerfBooted,
		}, {
			Name:              "vm_x86",
			ExtraSoftwareDeps: []string{"android_vm"},
			ExtraHardwareDeps: hwdep.D(hwdep.X86()),
			Val: mediaPerfTestParameters{
				binaryTranslation: false,
			},
			Pre: mediaPerfBooted,
		}},
		VarDeps: []string{"arc.MediaPerf.username", "arc.MediaPerf.password"},
	})
}

// MediaPerf automates synthetic media performance benchmark measurements.
// The overall final benchmark score is combined and uploaded as well.
func MediaPerf(ctx context.Context, s *testing.State) {
	// Shorten the test context so that even if the test times out
	// there will be time to clean up.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 1*time.Minute)
	defer cancel()

	// Obtain specific APK file name for the CPU architecture being tested.
	a := s.PreValue().(arc.PreData).ARC
	apkName, err := mediaperf.ApkNameForArch(ctx, a)
	if err != nil {
		s.Fatal("Failed to get APK name: ", err)
	}

	// Geometric mean for tests in the same group are computed together.  All
	// tests where group is not defined will be computed separately using the
	// geometric means from other groups.
	tests := []struct {
		name   string
		prefix string
		group  string
	}{{
		name:   "SequentialDecodeInstrumentedTest",
		prefix: "sequential",
		group:  "decoding",
	}, {
		name:   "ParallelDecodeInstrumentedTest",
		prefix: "parallel",
		group:  "decoding",
	}}

	finalPerfValues := perf.NewValues()
	param := s.Param().(mediaPerfTestParameters)

	config := mediaperf.TestConfig{
		PerfValues: finalPerfValues,
		Prefix:     "mediaperf",
		ApkPath:    s.DataPath(apkName),
		OutDir:     s.OutDir(),
	}

	// Many apps / games run with binary translation (b/169623350#comment8)
	// and thus it's an important use case to exercise.
	if param.binaryTranslation && apkName == mediaperf.X86ApkName {
		config.ApkPath = s.DataPath(mediaperf.ArmApkName)
	}

	var scores arc.ScoreList
	groups := make(map[string][]float64)

	cleanup, setupErr := mediaperf.SetupTest(ctx, &config, a)
	defer func(ctx context.Context) {
		if err := cleanup(ctx); err != nil && setupErr == nil {
			setupErr = errors.Wrap(err, "failed to cleanup after creating test")
		}
	}(cleanupCtx)

	for _, test := range tests {
		config.ClassName = test.name
		config.Prefix = test.prefix

		score, err := runMediaPerfTest(ctx, &config, a)
		if err != nil {
			s.Fatal("Failed to run mediaperf test: ", err)
		}

		// Put scores in the same group together, else add to top-level scores.
		if test.group != "" {
			groups[test.group] = append(groups[test.group], score)
		} else {
			scores = append(scores, score)
		}
	}

	for _, group := range groups {
		score, err := arc.CalcGeometricMean(group)
		if err != nil {
			s.Fatal("Failed to process geometric mean: ", err)
		}
		scores = append(scores, score)
	}
	// Calculate grand mean (geometric) of top-level scores which includes the
	// geometric means from each group.
	totalScore, err := arc.CalcGeometricMean(scores)
	if err != nil {
		s.Fatal("Failed to process geometric mean: ", err)
	}

	finalPerfValues.Set(
		perf.Metric{
			Name:      "total_score",
			Unit:      "None",
			Direction: perf.SmallerIsBetter,
			Multiple:  false,
		}, totalScore)
	s.Logf("Finished all tests with total score: %.2f", totalScore)

	s.Log("Uploading perf metrics")

	if err := config.PerfValues.Save(s.OutDir()); err != nil {
		s.Fatal("Failed to save final perf metrics: ", err)
	}
}

// runMediaPerfTest will launch the synthetic benchmark.
func runMediaPerfTest(ctx context.Context, config *mediaperf.TestConfig, a *arc.ARC) (float64, error) {
	shorterCtx, cancel := context.WithTimeout(ctx, 500*time.Second)
	defer cancel()

	return mediaperf.RunTest(shorterCtx, config, a)
}
