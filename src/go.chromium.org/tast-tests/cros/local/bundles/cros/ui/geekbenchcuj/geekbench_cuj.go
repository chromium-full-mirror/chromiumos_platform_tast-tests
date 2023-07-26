// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package geekbenchcuj

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/crostini"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast-tests/cros/local/vm"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type geekbenchInfo struct {
	name string
	// When needLicense is set to true, we will look for Geekbench email and key.
	needLicense bool
	// Version of Geekbench to run.
	version int
	// createGBDir create the working directory given the name of the folder.
	createGBDir func(context.Context, *vm.Container, *chrome.Chrome, string) (geekbenchDir, error)
	// pughGBFIles pushes the necessary files specified in geekbenchFiles struct.
	pushGBFiles func(context.Context, *vm.Container, geekbenchFiles) error
	// pushGBLicense writes the license string to the destination.
	pushGBLicense func(context.Context, *vm.Container, string /*licensePath*/, string /*licenseStr*/) error
	// retrieveGBFile retrieves the output result file from DUT.
	retrieveGBFile func(context.Context, *vm.Container, string /*resultPath*/, string /*logPath*/) error
}

// Run runs and collects Geekbench benchmark scores.
func Run(ctx context.Context, s *testing.State) {
	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	var cr *chrome.Chrome
	var tconn *chrome.TestConn
	var cont *vm.Container
	var err error

	gbInfo := s.Param().(geekbenchInfo)
	if gbInfo.name == "" {
		s.Fatal("Failed to parse valid geekbenchInfo struct")
	}

	if gbInfo.name == "crostini" {
		tconn = s.PreValue().(crostini.PreData).TestAPIConn
		cr = s.PreValue().(crostini.PreData).Chrome
		cont = s.PreValue().(crostini.PreData).Container
	} else {
		cr = s.FixtValue().(chrome.HasChrome).Chrome()

		tconn, err = cr.TestAPIConn(ctx)
		if err != nil {
			s.Fatal("Failed to connect to test API: ", err)
		}
	}

	const (
		resultFileName    = "geekbench_result.txt"
		geekbenchPlar     = "geekbench.plar"
		geekbenchWorkload = "geekbench-workload.plar"
	)
	geekbenchPlarSource := fmt.Sprintf("geekbench%d.plar", gbInfo.version)
	geekbenchLicense := fmt.Sprintf("Geekbench %d.preferences", gbInfo.version)
	folderName := fmt.Sprintf("geekbench%d_cuj", gbInfo.version)

	gbDir, err := gbInfo.createGBDir(ctx, cont, cr, folderName)
	if err != nil {
		s.Fatal("Failed to setup Geekbench directory: ", err)
	}
	defer gbDir.rmDir(ctx)

	execName, err := getExecutableFileName(ctx, false /*ignoreARMBitWidth*/, gbInfo.version)
	if err != nil {
		s.Fatal("Failed to get executable name: ", err)
	}

	// We do not have access to publicly available ARM binaries.
	if gbInfo.needLicense && (strings.Contains(execName, "aarch64") || strings.Contains(execName, "armv7")) {
		s.Fatal("Failed to validate architecture, ARM architecture not supported for public automation")
	}

	execFilePath := filepath.Join(gbDir.path, execName)
	resultPath := filepath.Join(gbDir.path, resultFileName)

	gbFiles := geekbenchFiles{
		binarySrc:  s.DataPath(execName),
		binaryDest: execFilePath,
		plarSrc:    s.DataPath(geekbenchPlarSource),
		plarDest:   filepath.Join(gbDir.path, geekbenchPlar),
	}

	// Additional file for Geekbench6.
	if gbInfo.version == 6 {
		gbFiles.workloadSrc = s.DataPath("geekbench6-workload.plar")
		gbFiles.workloadDest = filepath.Join(gbDir.path, geekbenchWorkload)
	}

	err = gbInfo.pushGBFiles(ctx, cont, gbFiles)
	if err != nil {
		s.Fatal("Failed to push Geekbench files: ", err)
	}

	// License is only needed for the public version of the test.
	if gbInfo.needLicense {
		email, ok := s.Var(geekbenchEmail)
		if !ok {
			s.Fatal("Failed to read license email, please provide it in the command line with -var=geekbench.email=email@example.com")
		}

		key, ok := s.Var(geekbenchKey)
		if !ok {
			s.Fatal("Failed to read license key, please provide it in the command line with -var=geekbench.key=key")
		}

		licensePath := filepath.Join(gbDir.path, geekbenchLicense)
		license := fmt.Sprintf(`{"license_key": "%s", "license_user": "%s"}`, key, email)
		if err := gbInfo.pushGBLicense(ctx, cont, licensePath, license); err != nil {
			s.Fatal("Failed to push license file; make sure inputs are correct: ", err)
		}
	}

	recorder, err := cujrecorder.NewRecorder(ctx, cr, tconn, nil, cujrecorder.RecorderOptions{
		CooldownBeforeRun: true,
		TurnOffDisplay:    true,
		Mode:              cujrecorder.Benchmark,
	})
	if err != nil {
		s.Fatal("Failed to create the recorder: ", err)
	}
	defer recorder.Close(cleanupCtx)

	if err := recorder.Run(ctx, func(ctx context.Context) error {
		recorder.Annotate(ctx, "Run_Geekbench_Executable")
		out, err := execCommand(ctx, cont, execFilePath, resultPath).Output(testexec.DumpLogOnError)
		if err != nil {
			if gbInfo.needLicense && strings.Contains(string(out), "Error: The `--no-upload` switch") {
				return errors.Wrap(err, "failed to verify Geekbench license; make sure email and key are entered correctly in command line or Geekbench preferences file")
			}
			return errors.Wrap(err, "geekbench execution failed")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to conduct the recorder task: ", err)
	}

	logFilePath := filepath.Join(s.OutDir(), resultFileName)
	if err := gbInfo.retrieveGBFile(ctx, cont, resultPath, logFilePath); err != nil {
		s.Fatal("Failed to get result file: ", err)
	}

	scores := make(map[string]float64)
	if err := retrieveScore(ctx, logFilePath, scores); err != nil {
		s.Fatal("Failed to retrieve Geekbench score: ", err)
	}

	pv := perf.NewValues()
	for metric, value := range scores {
		pv.Set(perf.Metric{
			Name:      metric,
			Unit:      "score",
			Direction: perf.BiggerIsBetter,
		}, value)
	}

	if err := recorder.Record(ctx, pv); err != nil {
		s.Fatal("Failed to record the performance data: ", err)
	}
	if err := pv.Save(s.OutDir()); err != nil {
		s.Fatal("Failed to save the performance data: ", err)
	}
}
