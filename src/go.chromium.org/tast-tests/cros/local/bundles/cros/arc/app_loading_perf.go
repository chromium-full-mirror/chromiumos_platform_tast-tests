// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/apploading"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/nethelper"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"

	"go.chromium.org/tast/core/errors"
)

// testParameters contains all the data needed to run a single test iteration.
type testParameters struct {
	binaryTranslation bool
}

type groupMap map[string]arc.ScoreList

type groupDef struct {
	upload        bool
	aggregationFn func(scores arc.ScoreList) (float64, error)
	parent        string
}

var (
	// extraArgs is a list of extra arguments to pass to Chrome.
	extraArgs = []string{"--disable-arc-cpu-restriction", "--disable-features=ArcExternalStorageAccess", "--disable-features=FirmwareUpdaterApp"}

	// arcAppLoadingBooted is a precondition similar to arc.Booted() with no opt-in and disables some heavy Android activities that use system resources.
	arcAppLoadingBooted = arc.NewPrecondition("arcapploading_booted", nil /* GAIAVARS */, nil /* GAIALOGINPOOLVARS */, false /* O_DIRECT */, append(arc.DisableSyncFlags(), extraArgs...)...)

	arcAppLoadingBootedWithPvSchedEnabled = arc.NewPreconditionWithPvSchedEnabled("arcapploading_booted_with_pvsched", nil /* GAIAVARS */, nil /* GAIALOGINPOOLVARS */, false /* O_DIRECT */, append(arc.DisableSyncFlags(), extraArgs...)...)
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           AppLoadingPerf,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Captures set of apploading performance metrics and uploads them as perf metrics",
		Contacts: []string{
			"arc-performance@google.com",
			"khmel@chromium.org",
		},
		// ChromeOS > Software > ARC++ > Performance
		BugComponent: "b:168382",
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{"chrome"},
		Data:         []string{apploading.X86ApkName, apploading.ArmApkName},
		Timeout:      35 * time.Minute,
		Params: []testing.Param{
			{
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm"},
				Val: testParameters{
					binaryTranslation: true,
				},
				Pre: arcAppLoadingBooted,
			}, {
				Name:              "vm_pvsched",
				BugComponent:      "b:167279",
				ExtraSoftwareDeps: []string{"android_vm"},
				Val: testParameters{
					binaryTranslation: true,
				},
				Pre:               arcAppLoadingBootedWithPvSchedEnabled,
				ExtraHardwareDeps: hwdep.D(hwdep.HasParavirtSchedControl()),
			}, {
				Name:              "vm_x86",
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraHardwareDeps: hwdep.D(hwdep.X86()),
				Val: testParameters{
					binaryTranslation: false,
				},
				Pre: arcAppLoadingBooted,
			}},
		VarDeps: []string{"arc.AppLoadingPerf.username", "arc.AppLoadingPerf.password"},
	})
}

// AppLoadingPerf automates app loading benchmark measurements to simulate
// system resource utilization in terms of memory, file system, networking,
// graphics, ui, etc. that can found in a game or full-featured app.  Each
// subflow will be tested separately including separate performance metrics
// uploads.  The overall final benchmark score combined and uploaded as well.
func AppLoadingPerf(ctx context.Context, s *testing.State) {
	const (
		// tbfRateMbit* specifies how fast the data will leave the primary bucket (float).
		tbfRateMbit = 20
		// tbfLatency is amount of time a packet can be delayed by token rate before drop (int).
		tbfLatencyMs = 18
		// tbfBurst is the size of the bucket used by rate option (int).
		tbfBurstKb = 10

		fileTestName   = "FileTest"
		memoryTestName = "MemoryTest"
	)

	// Shorten the test context so that even if the test times out
	// there will be time to clean up.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	// Start network helper to serve requests from the app.
	conn, err := nethelper.Start(ctx, apploading.NethelperPort)
	if err != nil {
		s.Fatal("Failed to start nethelper: ", err)
	}
	defer func(ctx context.Context) {
		if err := conn.Close(ctx); err != nil {
			s.Logf("WARNING: Failed to close nethelper connection: %s", err)
		}
	}(cleanupCtx)

	// Add initial traffic control queuing discipline settings (b/169947243) for
	// traffic shaping based on experiments with netem, RTT latency, and iperf3
	// bandwidth measurements.
	if err := conn.AddTcTbf(ctx, tbfRateMbit, tbfLatencyMs, tbfBurstKb); err != nil {
		s.Fatal("Failed to add tc-tbf: ", err)
	}

	finalPerfValues := perf.NewValues()
	param := s.Param().(testParameters)

	// Geometric mean for tests in the same group are computed together.  All
	// tests where group is not defined will be computed separately using the
	// geometric means from other groups.
	tests := []struct {
		name            string
		prefix          string
		subtest         string
		group           string
		iterations      int
		collectPsiStats bool
	}{{
		name:            memoryTestName,
		prefix:          "memory",
		collectPsiStats: true,
	}, {
		name:    fileTestName,
		prefix:  "file_obb",
		subtest: "runObbTest",
		group:   "not_ext4_fs",
	}, {
		name:    fileTestName,
		prefix:  "file_squashfs",
		subtest: "runSquashFSTest",
		group:   "not_ext4_fs",
	}, {
		name:    fileTestName,
		prefix:  "file_esd",
		subtest: "runEsdTest",
		group:   "not_ext4_fs",
	}, {
		name:    fileTestName,
		prefix:  "file_ext4",
		subtest: "runExt4Test",
		group:   "ext4_fs",
	}, {
		name:   "NetworkTest",
		prefix: "network",
	}, {
		name:   "OpenGLTest",
		prefix: "opengl",
	}, {
		name:   "DecompressionTest",
		prefix: "decompression",
	}, {
		name:   "UITest",
		prefix: "ui",
	}}

	// Optional metadata about groups.
	groupMetadata := map[string]groupDef{
		// Original groups from |tests| struct.
		"ext4_fs": {
			aggregationFn: arc.CalcGeometricMean,
			parent:        "io_score",
		},
		"not_ext4_fs": {
			aggregationFn: arc.CalcGeometricMean,
			parent:        "io_score",
		},
		// Second-level storage groups.
		"io_score": {
			upload:        true,
			aggregationFn: arc.CalcGeometricMean,
		},
	}

	// Obtain specific APK file name for the CPU architecture being tested.
	a := s.PreValue().(arc.PreData).ARC
	apkName, err := apploading.ApkNameForArch(ctx, a)
	if err != nil {
		s.Fatal("Failed to get APK name: ", err)
	}
	config := apploading.TestConfig{
		PerfValues: finalPerfValues,
		// Don't disable Wifi for network test since ethernet connection in lab is not guaranteed.
		// Otherwise tc-tbf settings will not be applied since it would have been disabled and reset.
		WifiInterfacesMode: setup.DoNotChangeWifiInterfaces,
		ApkPath:            s.DataPath(apkName),
		OutDir:             s.OutDir(),
	}

	// Many apps / games run with binary translation (b/169623350#comment8)
	// and thus it's an important use case to exercise.
	if param.binaryTranslation && apkName == apploading.X86ApkName {
		config.ApkPath = s.DataPath(apploading.ArmApkName)
	}

	var scores arc.ScoreList
	groups := make(groupMap)
	cr := s.PreValue().(arc.PreData).Chrome

	cleanup, setupErr := apploading.SetupTest(ctx, &config, a, cr)
	defer func(ctx context.Context) {
		if err := cleanup(ctx); err != nil && setupErr == nil {
			setupErr = errors.Wrap(err, "failed to cleanup after creating test")
		}
	}(cleanupCtx)

	for _, test := range tests {
		config.ClassName = test.name
		config.Prefix = test.prefix
		config.Subtest = test.subtest
		config.Iterations = test.iterations
		config.CollectPsiStats = test.collectPsiStats
		if config.Iterations < 1 {
			if config.Iterations < 0 {
				s.Fatal("Invalid number of iterations used: ", config.Iterations)
			}
			config.Iterations = 1 // Set to 1 if not specified (0).
		}

		score, err := runAppLoadingTest(ctx, &config, a, cr)
		if err != nil {
			s.Fatal("Failed to run apploading test: ", err)
		}

		// Put scores in the same group together, else add to top-level scores.
		if test.group != "" {
			if _, ok := groupMetadata[test.group]; !ok {
				s.Fatal("Test defined with invalid group: ", test.group)
			}
			groups[test.group] = append(groups[test.group], score)
		} else {
			scores = append(scores, score)
		}
	}

	// Aggregate groups of scores from individual test runs and repeat
	// the process on the results, until no further aggregation is needed.
	isOriginalGroup := true
	for len(groups) > 0 {
		parentGroups := make(groupMap)

		for groupName, group := range groups {
			groupInfo, groupFound := groupMetadata[groupName]
			if !groupFound {
				s.Fatal("Failed to find definition for group:", groupName)
			}

			score, err := groupInfo.aggregationFn(group)
			if err != nil {
				s.Fatal("Failed to process geometric mean: ", err)
			}
			if isOriginalGroup {
				scores = append(scores, score)
			}
			if parent := groupInfo.parent; parent != "" {
				if _, ok := groupMetadata[parent]; !ok {
					s.Fatal("Failed to find definition for parent group: ", parent)
				}
				parentGroups[parent] = append(parentGroups[parent], score)
			}
			if groupInfo.upload {
				finalPerfValues.Set(
					perf.Metric{
						Name:      groupName,
						Unit:      "MB_per_sec",
						Direction: perf.BiggerIsBetter,
						Multiple:  false,
					}, score)
			}
		}

		groups = parentGroups
		isOriginalGroup = false // from here on, only aggregations.
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
			Direction: perf.BiggerIsBetter,
			Multiple:  false,
		}, totalScore)
	s.Logf("Finished all tests with total score: %.2f", totalScore)

	s.Log("Uploading perf metrics")

	if err := finalPerfValues.Save(s.OutDir()); err != nil {
		s.Fatal("Failed to save final perf metrics: ", err)
	}
}

// runAppLoadingTest will test each app loading subflow with timeout.
func runAppLoadingTest(ctx context.Context, config *apploading.TestConfig, a *arc.ARC, cr *chrome.Chrome) (float64, error) {
	shorterCtx, cancel := context.WithTimeout(ctx, 500*time.Second)
	defer cancel()

	// Each subflow should take no longer than 8.5 minutes based on stainless
	// data. If it takes longer, very likely the app is stuck (e.g. b/169367367).
	return apploading.RunTest(shorterCtx, config, a, cr)
}
