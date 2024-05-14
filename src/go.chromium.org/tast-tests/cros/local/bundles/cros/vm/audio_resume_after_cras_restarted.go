// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vm

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/vm/audioutils"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/vm/dlc"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const runAudioResumeAfterCrasRestarted string = "run-audio-resume-after-cras-restarted.sh"

type audioResumeAfterCrasRestartedParams struct {
	crosvmArgs    []string
	vhostUserArgs []string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         AudioResumeAfterCrasRestarted,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check that aplay resumes playing after cras is restarted during the playback",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "pteerapong@chromium.org"},
		// ChromeOS > Platform > Technologies > Audio > VM
		BugComponent: "b:1332660",
		Attr:         []string{"group:mainline", "group:sw_gates_virt", "sw_gates_virt_enabled"},
		Data:         []string{runAudioResumeAfterCrasRestarted},
		Timeout:      3 * time.Minute,
		SoftwareDeps: []string{"vm_host", "chrome", "dlc"},
		// TODO(b/288063328): Fix frequent "Failed to install DLC" error on these models
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("pujjoteen", "steelix", "rusty", "vorticon", "bobba")),
		Fixture:      "vmDLC",
		Params: []testing.Param{
			{
				Name: "virtio_cras_snd",
				Val: audioResumeAfterCrasRestartedParams{
					crosvmArgs: []string{"--virtio-snd", "capture=true,backend=cras,socket_type=legacy"},
				},
			},
		},
	})
}

// AudioResumeAfterCrasRestarted is used as one of the standard SW gates (go/pe-sw-gates).
// Please ask to crosvm-core@ if you want to modify or delete this test.
func AudioResumeAfterCrasRestarted(ctx context.Context, s *testing.State) {
	param := s.Param().(audioResumeAfterCrasRestartedParams)
	data := s.FixtValue().(dlc.FixtData)

	kernelLogPath := filepath.Join(s.OutDir(), "kernel.log")
	outputLogPath := filepath.Join(s.OutDir(), "output.txt")

	kernelArgs := []string{
		fmt.Sprintf("init=%s", s.DataPath(runAudioResumeAfterCrasRestarted)),
		"--",
		outputLogPath,
	}

	config := audioutils.Config{
		CrosvmArgs: param.crosvmArgs,
	}

	var runScriptWG sync.WaitGroup
	runScriptWG.Add(1)
	go func() {
		defer runScriptWG.Done()

		testing.ContextLog(ctx, "Running crosvm to play audio stream")
		if err := audioutils.RunCrosvm(ctx, data.Kernel, kernelLogPath, kernelArgs, config); err != nil {
			s.Fatal("Failed to run crosvm: ", err)
		}
	}()

	testing.ContextLog(ctx, "Waiting for the first stream")
	if _, err := crastestclient.WaitForStreams(ctx, 10*time.Second); err != nil {
		s.Fatal("Failed to wait for first stream: ", err)
	}

	testing.ContextLog(ctx, "Restarting cras")
	if err := testexec.CommandContext(ctx, "restart", "cras").Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to restart cras: ", err)
	}

	// Wait for script to finish and read result
	testing.ContextLog(ctx, "Waiting for the script to finish")
	runScriptWG.Wait()

	// Read output and check that the second aplay returned 0
	output, err := ioutil.ReadFile(outputLogPath)
	if err != nil {
		s.Fatal("Failed to read output file: ", err)
	}
	if !strings.Contains(string(output), "aplay 2 returned 0") {
		s.Fatal("aplay error after restart cras")
	}
}
