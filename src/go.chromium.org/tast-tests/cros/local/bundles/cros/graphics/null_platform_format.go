// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// Intel SoCs with GPU Gen 9 (SKL) or earlier don't support scanning out 30bpp framebuffers.
var intelGen9AndEarlierGPUSoCs = []string{"pinetrail", "broadwell", "apollolake", "skylake"}

func init() {
	testing.AddTest(&testing.Test{
		Func:         NullPlatformFormat,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that the null_platform_test passes for at least one format with a given color depth",
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		Contacts: []string{
			"chromeos-gfx-video@google.com",
			"mcasas@chromium.org",
		},
		Attr:         []string{"group:graphics", "graphics_perbuild"},
		SoftwareDeps: []string{"no_qemu"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Timeout:      time.Minute,
		Params: []testing.Param{{
			Name: "24bpp",
			Val:  []string{"XR24", "XB24"},
		}, {
			Name: "30bpp",
			Val:  []string{"AR30", "AB30", "XR30", "XB30"},
			ExtraHardwareDeps: hwdep.D(
				hwdep.SkipCPUSocFamily("mediatek", "rockchip", "qualcomm"),
				hwdep.SkipGPUFamily(intelGen9AndEarlierGPUSoCs...),
				hwdep.SkipGPUFamily("stoney")),
		}},
		Fixture: "gpuWatchHangs",
	})
}

// NullPlatformFormat runs null_platform_test binary test for a given format.
func NullPlatformFormat(ctx context.Context, state *testing.State) {
	if err := upstart.StopJob(ctx, "ui"); err != nil {
		state.Fatal("Failed to stop ui job: ", err)
	}
	defer upstart.EnsureJobRunning(ctx, "ui")

	const testCommand string = "null_platform_test"

	f, err := os.Create(filepath.Join(state.OutDir(), filepath.Base(testCommand)+".txt"))
	if err != nil {
		state.Fatal("Failed to create a log file: ", err)
	}
	defer f.Close()

	const formatFlag string = "-f"
	formats := state.Param().([]string)
	invocationError := make(map[string]error)

	for _, format := range formats {
		invocationCommand := shutil.EscapeSlice([]string{testCommand, formatFlag, format})
		state.Log("Running ", invocationCommand)

		// Execute the null_platform_test for a given format
		cmd := testexec.CommandContext(ctx, testCommand, []string{formatFlag, format}...)
		cmd.Stdout = f
		cmd.Stderr = f
		if err := cmd.Run(); err != nil {
			invocationError[invocationCommand] = err
		} else {
			state.Logf("Run succeeded for %s", invocationCommand)
			return
		}
	}
	state.Errorf("Failed to run %s for all formats", testCommand)
	for command, err := range invocationError {
		exitCode, ok := testexec.ExitCode(err)
		if !ok {
			state.Errorf("Failed to run %s: %v", command, err)
		} else {
			state.Errorf("Command %s exited with status %v", command, exitCode)
		}
	}
}
