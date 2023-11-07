// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package geekbench

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

// GBInfo has the information to run GeekbenchCUJ
type GBInfo struct {
	Name string
	// When needLicense is set to true, we will look for Geekbench email and key.
	NeedLicense bool
	// Version of Geekbench to run.
	Version int
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
func Run(ctx context.Context, gbInfo GBInfo, fixtValue interface{}, stateVars map[string]string) (e error) {
	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	var cr *chrome.Chrome
	var tconn *chrome.TestConn
	var cont *vm.Container
	var err error

	if strings.Contains(gbInfo.Name, "crostini") {
		tconn = fixtValue.(crostini.PreData).TestAPIConn
		cr = fixtValue.(crostini.PreData).Chrome
		cont = fixtValue.(crostini.PreData).Container
	} else {
		cr = fixtValue.(chrome.HasChrome).Chrome()

		tconn, err = cr.TestAPIConn(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to connect to test API")
		}
	}

	const (
		resultFileName    = "geekbench_result.txt"
		geekbenchPlar     = "geekbench.plar"
		geekbenchWorkload = "geekbench-workload.plar"
	)
	geekbenchPlarSource := fmt.Sprintf("geekbench%d.plar", gbInfo.Version)
	geekbenchLicense := fmt.Sprintf("Geekbench %d.preferences", gbInfo.Version)
	folderName := fmt.Sprintf("geekbench%d_cuj", gbInfo.Version)

	gbDir, err := gbInfo.createGBDir(ctx, cont, cr, folderName)
	if err != nil {
		return errors.Wrap(err, "failed to setup Geekbench directory")
	}
	defer gbDir.rmDir(ctx)

	var execName string
	if strings.Contains(gbInfo.Name, "custom") {
		execName = "geekbench_custom"
	} else {
		// Crostini VM is always 64-bit.
		execName, err = getExecutableFileName(ctx, strings.Contains(gbInfo.Name, "crostini") /* ignoreARMBitWidth */, gbInfo.Version)
		if err != nil {
			return errors.Wrap(err, "failed to get executable name")
		}
	}

	execFilePath := filepath.Join(gbDir.path, execName)
	resultPath := filepath.Join(gbDir.path, resultFileName)
	gbFiles := geekbenchFiles{
		binarySrc:  stateVars[execName],
		binaryDest: execFilePath,
		plarSrc:    stateVars[geekbenchPlarSource],
		plarDest:   filepath.Join(gbDir.path, geekbenchPlar),
	}

	// Additional file for Geekbench6.
	if gbInfo.Version == 6 {
		gbFiles.workloadSrc = stateVars["geekbench6-workload.plar"]
		gbFiles.workloadDest = filepath.Join(gbDir.path, geekbenchWorkload)
	}

	err = gbInfo.pushGBFiles(ctx, cont, gbFiles)
	if err != nil {
		return errors.Wrap(err, "failed to push Geekbench files")
	}

	// License is only needed for the public version of the test.
	if gbInfo.NeedLicense {
		email := stateVars["email"]
		key := stateVars["key"]

		licensePath := filepath.Join(gbDir.path, geekbenchLicense)
		license := fmt.Sprintf(`{"license_key": "%s", "license_user": "%s"}`, key, email)
		if err := gbInfo.pushGBLicense(ctx, cont, licensePath, license); err != nil {
			return errors.Wrap(err, "failed to push license file; make sure inputs are correct")
		}
	}

	recorder, err := cujrecorder.NewRecorder(ctx, cr, tconn, nil, cujrecorder.RecorderOptions{
		CooldownBeforeRun: true,
		TurnOffDisplay:    true,
		Mode:              cujrecorder.Benchmark,
	})
	if err != nil {
		return errors.Wrap(err, "failed to create the recorder")
	}
	defer recorder.Close(cleanupCtx)

	if err := recorder.Run(ctx, func(ctx context.Context) error {
		recorder.Annotate(ctx, "Run_Geekbench_Executable")
		out, err := execCommand(ctx, cont, execFilePath, resultPath).Output(testexec.DumpLogOnError)
		if err != nil {
			if gbInfo.NeedLicense && strings.Contains(string(out), "Error: The `--no-upload` switch") {
				return errors.Wrap(err, "failed to verify Geekbench license; make sure email and key are entered correctly in command line or Geekbench preferences file")
			}
			return errors.Wrap(err, "Geekbench execution failed")
		}
		return nil
	}); err != nil {
		return errors.Wrap(err, "failed to conduct the recorder task")
	}

	outDir, ok := testing.ContextOutDir(ctx)
	if !ok || outDir == "" {
		return errors.New("failed to get the out directory")
	}

	logFilePath := filepath.Join(outDir, resultFileName)
	if err := gbInfo.retrieveGBFile(ctx, cont, resultPath, logFilePath); err != nil {
		return errors.Wrap(err, "failed to get result file")
	}

	scores := make(map[string]float64)
	if err := RetrieveScore(ctx, logFilePath, scores); err != nil {
		return errors.Wrap(err, "failed to retrieve Geekbench score")
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
		return errors.Wrap(err, "failed to record the performance data")
	}
	if err := pv.Save(outDir); err != nil {
		return errors.Wrap(err, "failed to save the performance data")
	}
	return nil
}
