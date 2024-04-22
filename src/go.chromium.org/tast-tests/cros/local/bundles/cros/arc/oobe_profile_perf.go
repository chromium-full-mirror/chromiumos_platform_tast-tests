// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/tracing/linuxperf"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OOBEProfilePerf,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Profile OOBE and post-login",
		Contacts: []string{
			"baseos-perf@google.com",
			"cwd@google.com",
		},
		BugComponent: "b:930563",
		SoftwareDeps: []string{"play_store", "chrome", "arc"},
		Vars: []string{
			"arc.OOBEProfilePerf.user",
			"arc.OOBEProfilePerf.pass",
			"arc.OOBEProfilePerf.iterations",
		},
		Params: []testing.Param{
			{
				ExtraAttr: []string{"group:crosbolt", "crosbolt_nightly"},
				Timeout:   1 * time.Hour,
				// TODO (cwd): expand test devices when know more about storage
				// requirements and test stability.
				ExtraHardwareDeps: hwdep.D(hwdep.Model("ampton", "brya", "steelix", "treeya")),
			},
			{
				Name:    "high_iteration",
				Timeout: 8 * time.Hour,
			},
		},
	})
}

func runOneOOBEProfilePerf(ctx context.Context, i int, outDir string, opts []chrome.Option, p *perf.Values) ([]string, error) {
	// Each run should finish in 10 minutes.
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	var outFiles []string
	cleanupCtx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	// Start perf profile.
	perfDataFile := filepath.Join(outDir, fmt.Sprintf("perf%d.data", i))
	lp, err := linuxperf.NewRecordInstance(
		ctx,
		linuxperf.Output(perfDataFile),
		linuxperf.AllCpus(),
		linuxperf.Stacks(),
		linuxperf.EventWithPeriod("cpu-cycles", 2800000),
	)
	if err != nil {
		return outFiles, errors.Wrap(err, "failed to start perf trace")
	}
	defer lp.Close()
	outFiles = append(outFiles, filepath.Base(perfDataFile))

	// Start Chrome.
	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		return outFiles, errors.Wrap(err, "failed to connect to Chrome")
	}
	defer cr.Close(cleanupCtx)

	testing.ContextLog(ctx, "Opting into Play Store")
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return outFiles, errors.Wrap(err, "failed to connect Test API")
	}
	if err := optin.PerformAndClose(ctx, cr, tconn); err != nil {
		return outFiles, errors.Wrap(err, "failed to opt into Play Store")
	}

	// Verify that ARC starts.
	if _, err := arc.New(ctx, outDir, ""); err != nil {
		return outFiles, errors.Wrap(err, "ARC did not start")
	}

	testing.ContextLog(ctx, "Sleeping after login")

	// GoBigSleepLint This test is for collecting a performance profile to measure
	// the activity after OOBE, so we need to wait a while in order to collect it.
	if err := testing.Sleep(ctx, 5*time.Minute); err != nil {
		return nil, errors.Wrap(err, "failed to sleep after login")
	}

	if err := lp.Stop(); err != nil {
		return outFiles, errors.Wrap(err, "failed to stop perf record command")
	}

	cycles := linuxperf.NewProcessThreadCounts("cpu-cycles")
	if err := linuxperf.Script(ctx, lp.OutFile(), cycles.OnEvent); err != nil {
		return outFiles, errors.Wrap(err, "failed to extract metrics from perf data")
	}

	cyclesJSON, err := json.Marshal(cycles.Counts)
	if err != nil {
		return outFiles, errors.Wrap(err, "failed to convert Linux Perf profile counts to JSON")
	}
	cyclesJSONFile := filepath.Join(outDir, fmt.Sprintf("cpu-cycles%d.json", i))
	if err := os.WriteFile(cyclesJSONFile, cyclesJSON, 0666); err != nil {
		return outFiles, errors.Wrap(err, "failed to save Linux Perf profile count JSON")
	}
	outFiles = append(outFiles, filepath.Base(cyclesJSONFile))

	cycles.AppendProcessCountMetrics(
		p,
		// TODO (cwd): Revisit this list after getting lab data across a few
		// builds and device types.
		[]string{
			"chrome",
			"crosvm",
			"pcivirtio-fs",
			"pcivirtio-gpu",
			"swapper",
			"cros",
			"pcivirtio-net",
			"vsh",
			"kcompactd0",
			"permission_brok",
			"pidof",
			"kswapd0",
			"dbus-daemon",
			"btadapterd",
			"pcivirtio-block",
		},
		"Mcycles",
		1.0/1000000.0,
		"",
	)

	testing.ContextLog(ctx, "Linux Perf process and thread cpu-cycles counts:")
	cycles.Log(ctx, cycles.Total()/1000)

	// Rename logcat so later iterations don't overwrite this one.
	logcatFile := fmt.Sprintf("logcat%d.txt", i)
	if err := os.Rename(filepath.Join(outDir, "logcat.txt"), filepath.Join(outDir, logcatFile)); err != nil {
		return outFiles, errors.Wrap(err, "failed to rename logcat.txt")
	}
	outFiles = append(outFiles, logcatFile)

	return outFiles, nil
}

func OOBEProfilePerf(ctx context.Context, s *testing.State) {
	opts := []chrome.Option{chrome.ARCSupported()}

	if user, ok := s.Var("arc.OOBEProfilePerf.user"); ok {
		pass, _ := s.Var("arc.OOBEProfilePerf.pass")
		opts = append(opts, chrome.GAIALogin(chrome.Creds{
			User: user,
			Pass: pass,
		}))
	} else {
		opts = append(opts, chrome.GAIALoginPool(dma.CredsFromPool(ui.GaiaPoolDefaultVarName)))
	}

	p := perf.NewValues()

	n := 5
	if nString, ok := s.Var("arc.OOBEProfilePerf.iterations"); ok {
		var err error
		n, err = strconv.Atoi(nString)
		if err != nil {
			s.Fatalf("Invalid var arc.OOBEProfilePerf.iterations value %q", nString)
		}
	}

	var zipFiles []string
	for i := 0; i < n; i++ {
		s.Logf("Iteration %d of %d", i+1, n)

		files, err := runOneOOBEProfilePerf(ctx, i, s.OutDir(), opts, p)
		zipFiles = append(zipFiles, files...)
		if err != nil {
			s.Errorf("Error on iteration %d: %s", i, err)
			break
		}
	}

	// Our context has a very long timeout, so shorten it so hangs don't take
	// 8h to detect.
	ctx, cancelCleanupCtx := context.WithTimeout(ctx, 5*time.Minute)
	defer cancelCleanupCtx()

	if err := testexec.CommandContext(
		ctx,
		"bash",
		"-c",
		fmt.Sprintf(
			"cd %s && zip -m perf.zip %s", // Remove uncompressed with -m flag.
			s.OutDir(),
			strings.Join(zipFiles, " "),
		),
	).Run(testexec.DumpLogOnError); err != nil {
		s.Error("Failed to compress Linux Perf output: ", err)
	}

	if err := p.Save(s.OutDir()); err != nil {
		s.Error("Failed to write perf.Values: ", err)
	}
}
