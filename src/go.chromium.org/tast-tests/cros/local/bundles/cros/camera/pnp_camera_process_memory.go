// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/camera/pnp"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	powersetup "go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	crosCameraServiceDaemonName     = "cros-camera"
	crosCameraServiceExecutablePath = "/usr/bin/cros_camera_service"
	crosCameraAlgoDaemonName        = "cros-camera-algo"
	crosCameraAlgoExecutablePath    = "/usr/bin/cros_camera_algo"
	crosCameraGPUAlgoDaemonName     = "cros-camera-gpu-algo"
	crosCameraGPUAlgoExecutablePath = "/usr/bin/cros_camera_algo --type=gpu"

	stableMemoryWaitingTime = 1 * time.Minute
)

type smapsInfo struct {
	Rss int
	Pss int
}

type cameraProcess struct {
	daemonName     string
	executablePath string
}

// Header line is of the following pattern:
// <address range> <permissions> <offset> <dev> <inode> <path>
var headerRegex = regexp.MustCompile(`^([^ ]+) ([^ ]+) ([^ ]+) ([^ ]+) ([^ ]+)\s+([^ ]*)$`)

// Memory size line is of the following pattern(except for VmFlags info)
// <info name>: <size of data> kB
var memorySizeRegex = regexp.MustCompile(`^([^ ]+):\s+([^ ]+)\s+kB$`)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PNPCameraProcessMemory,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Collect memory usage of camera service",
		Contacts:     []string{"chromeos-camera-eng@google.com", "esker@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Fixture:      pnp.StablePowerLacros,
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{caps.BuiltinCamera, "chrome", "camera_app"},
		Timeout:      5 * time.Minute,
	})
}

func processFullNameToPids(ctx context.Context, processFullName string) ([]string, error) {
	pgrepCmd := testexec.CommandContext(ctx, "pgrep", "-fx", processFullName)
	pgrepOutput, err := pgrepCmd.Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to run pgrep with path %s", processFullName)
	}
	return strings.Split(string(bytes.TrimSuffix(pgrepOutput, []byte{'\n'})), "\n"), nil
}

func getSmapsInfo(ctx context.Context, processFullName string) (map[string]*smapsInfo, error) {
	pids, err := processFullNameToPids(ctx, processFullName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get pids from process %s", processFullName)
	}

	smapsInfoMap := make(map[string]*smapsInfo)
	for _, pid := range pids {
		catSmapsCmd := testexec.CommandContext(ctx, "cat", "/proc/"+pid+"/smaps")
		catSmapsOutput, err := catSmapsCmd.Output(testexec.DumpLogOnError)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to dump smaps info of pid %s", pid)
		}

		lines := strings.Split(string(catSmapsOutput), "\n")
		var smapsInfoEntry *smapsInfo
		for _, line := range lines {
			if matches := headerRegex.FindStringSubmatch(line); matches != nil && matches[1] != "VmFlags:" {
				smapsInfoKey := matches[6]
				if _, ok := smapsInfoMap[smapsInfoKey]; !ok {
					smapsInfoMap[smapsInfoKey] = &smapsInfo{}
				}
				smapsInfoEntry = smapsInfoMap[smapsInfoKey]
			} else if matches := memorySizeRegex.FindStringSubmatch(line); matches != nil {
				dataSize, err := strconv.Atoi(matches[2])
				if err != nil {
					return nil, errors.Wrapf(err, "failed to convert data size %s to an integer", matches[2])
				}
				if matches[1] == "Rss" {
					smapsInfoEntry.Rss += dataSize
				} else if matches[1] == "Pss" {
					smapsInfoEntry.Pss += dataSize
				}
			}
		}
	}
	return smapsInfoMap, nil
}

func getSmapsRollupInfo(ctx context.Context, processFullName string) (*smapsInfo, error) {
	pids, err := processFullNameToPids(ctx, processFullName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get pids from process %s", processFullName)
	}

	smapsInfoEntry := &smapsInfo{}
	for _, pid := range pids {
		catSmapsCmd := testexec.CommandContext(ctx, "cat", "/proc/"+pid+"/smaps_rollup")
		catSmapsOutput, err := catSmapsCmd.Output(testexec.DumpLogOnError)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to dump smaps_rollup info of pid %s", pid)
		}

		lines := strings.Split(string(catSmapsOutput), "\n")
		for _, line := range lines {
			if matches := memorySizeRegex.FindStringSubmatch(line); matches != nil {
				dataSize, err := strconv.Atoi(matches[2])
				if err != nil {
					return nil, errors.Wrapf(err, "failes to convert data size %s to an integer", matches[2])
				}
				if matches[1] == "Rss" {
					smapsInfoEntry.Rss += dataSize
				} else if matches[1] == "Pss" {
					smapsInfoEntry.Pss += dataSize
				}
			}
		}
	}
	return smapsInfoEntry, nil
}

func saveSmapsInfoAsJSONFile(smapsInfoMap map[string]*smapsInfo, jsonFilePath string) error {
	jsonData, err := json.MarshalIndent(smapsInfoMap, "", " ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal the smapsInfoMap")
	}

	if err := os.WriteFile(jsonFilePath, jsonData, 0644); err != nil {
		return errors.Wrapf(err, "failed to save the json to path %s", jsonFilePath)
	}
	return nil
}

func saveProcessSmaps(ctx context.Context, cameraProcessList []cameraProcess, outDir, suffix string, pv *perf.Values) error {
	for _, cameraProcess := range cameraProcessList {
		daemonName := cameraProcess.daemonName
		executablePath := cameraProcess.executablePath
		smapsInfoMap, err := getSmapsInfo(ctx, executablePath)
		if err != nil {
			return errors.Wrapf(err, "failed to get smaps info from process %s", executablePath)
		}
		smapsRollupInfo, err := getSmapsRollupInfo(ctx, executablePath)
		if err != nil {
			return errors.Wrapf(err, "failed to get smaps_rollup info from process %s", executablePath)
		}

		rollupSmapsInfoKey := "total"
		if _, ok := smapsInfoMap[rollupSmapsInfoKey]; ok {
			return errors.Wrapf(err, "key %s already exists in smapsInfoMap", rollupSmapsInfoKey)
		}
		smapsInfoMap[rollupSmapsInfoKey] = smapsRollupInfo

		if err := saveSmapsInfoAsJSONFile(smapsInfoMap, path.Join(outDir, fmt.Sprintf("%s_%s.json", daemonName, suffix))); err != nil {
			return errors.Wrapf(err, "failed to save smapsInfoMap from daemon %s", daemonName)
		}
		pv.Set(perf.Metric{
			Name:      fmt.Sprintf("%s_%s_total_RSS", daemonName, suffix),
			Unit:      "kB",
			Direction: perf.SmallerIsBetter,
		}, float64(smapsRollupInfo.Rss))
		pv.Set(perf.Metric{
			Name:      fmt.Sprintf("%s_%s_total_PSS", daemonName, suffix),
			Unit:      "kB",
			Direction: perf.SmallerIsBetter,
		}, float64(smapsRollupInfo.Pss))
	}
	return nil
}

func PNPCameraProcessMemory(ctx context.Context, s *testing.State) {
	// Reserve some time for the cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cameraProcessList := []cameraProcess{
		cameraProcess{
			daemonName:     crosCameraServiceDaemonName,
			executablePath: crosCameraServiceExecutablePath,
		},
		cameraProcess{
			daemonName:     crosCameraAlgoDaemonName,
			executablePath: crosCameraAlgoExecutablePath,
		},
		cameraProcess{
			daemonName:     crosCameraGPUAlgoDaemonName,
			executablePath: crosCameraGPUAlgoExecutablePath,
		},
	}

	var filteredCameraProcessList []cameraProcess
	for _, cameraProcess := range cameraProcessList {
		daemonName := cameraProcess.daemonName
		if enable, err := upstart.IsJobEnabled(daemonName); err != nil {
			s.Fatalf("Failed to get job enablement info of daemon %s: %s", daemonName, err)
		} else if !enable {
			s.Logf("Daemon: %s does not exist hence pass measuring it", daemonName)
			continue
		}

		if err := upstart.RestartJob(ctx, daemonName); err != nil {
			s.Fatalf("Failed to restart %s: %s", daemonName, err)
		}

		filteredCameraProcessList = append(filteredCameraProcessList, cameraProcess)
	}

	// GoBigSleepLint: Wait for the processes to be stable.
	if err := testing.Sleep(ctx, stableMemoryWaitingTime); err != nil {
		s.Fatal("Failed to sleep to warm up")
	}

	perfValue := perf.NewValues()
	saveProcessSmaps(ctx, filteredCameraProcessList, s.OutDir(), "idle", perfValue)

	// Open CCA.
	cr := s.FixtValue().(powersetup.PowerUIFixtureData).Cr
	tb, err := testutil.NewTestBridge(ctx, cr, testutil.UseRealCamera)
	if err != nil {
		s.Fatal("Failed to construct test bridge: ", err)
	}
	defer tb.TearDown(cleanupCtx)
	if err := cca.ClearSavedDir(ctx, cr); err != nil {
		s.Fatal("Failed to clear saved directory: ", err)
	}
	app, err := cca.New(ctx, cr, s.OutDir(), tb)
	if err != nil {
		s.Fatal("Failed to open CCA: ", err)
	}
	defer app.Close(cleanupCtx)

	if err := app.SwitchMode(ctx, cca.Video); err != nil {
		s.Error("Failed to switch to video mode: ", err)
	}

	// GoBigSleepLint: Wait for the processes or CCA to be stable.
	if err := testing.Sleep(ctx, stableMemoryWaitingTime); err != nil {
		s.Fatal("Failed to sleep to warm up")
	}

	saveProcessSmaps(ctx, filteredCameraProcessList, s.OutDir(), "using_cca", perfValue)

	if err := perfValue.Save(s.OutDir()); err != nil {
		s.Fatal("Failed to save perf value: ", err)
	}
}
