// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/audio/debug"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	suspendStressTimePerRound = 20 * time.Second
	testBlockSize             = 480
)

type crasSuspendStressParam struct {
	testNode string
	rounds   int
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrasSuspendStress,
		Desc:         "Verifies CRAS function abilities over suspended multiple times",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "johnylin@chromium.org"},
		BugComponent: "b:776546",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline"},
		Fixture:      "rebootForAudioDSPFixture",
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{{
			Name: "playback",
			Val: crasSuspendStressParam{
				testNode: "INTERNAL_SPEAKER",
				rounds:   50,
			},
			ExtraAttr:         []string{"informational"},
			ExtraHardwareDeps: hwdep.D(hwdep.Speaker()),
			Timeout:           50*suspendStressTimePerRound + chrome.ResetTimeout,
		}, {
			Name: "capture",
			Val: crasSuspendStressParam{
				testNode: "INTERNAL_MIC",
				rounds:   50,
			},
			ExtraAttr:         []string{"informational"},
			ExtraHardwareDeps: hwdep.D(hwdep.Microphone()),
			Timeout:           50*suspendStressTimePerRound + chrome.ResetTimeout,
		}},
	})
}

func CrasSuspendStress(ctx context.Context, s *testing.State) {
	param := s.Param().(crasSuspendStressParam)

	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Cannot connect to CRAS: ", err)
	}

	// Shorten deadline to leave time for cleanup.
	runCtx, cancel := ctxutil.Shorten(ctx, chrome.ResetTimeout)
	defer cancel()

	// Chrome browser is NOT turned off to better simulate the real use case.
	cr, err := chrome.New(
		runCtx,
		// org.chromium.ChromeFeaturesService does not need login to work.
		// Don't login to speed up the test.
		chrome.NoLogin(),
	)
	if err != nil {
		s.Fatal("Cannot start Chrome: ", err)
	}
	defer cr.Close(ctx)

	testNode, err := cras.GetNodeByType(runCtx, param.testNode)
	if err != nil {
		s.Fatalf("Failed to get %s node: %v", param.testNode, err)
	}

	// Ensure audio works fine before suspend_stress_test.
	if err := runHealthCheckOnce(runCtx, testNode); err != nil {
		s.Fatal("Failed on pre-check: ", err)
	}

	for round := 0; round < param.rounds; round++ {
		s.Logf("Test round %d", round)
		if err := runSuspendResumeOnce(runCtx, testNode); err != nil {
			s.Fatalf("Failed on suspend/resume at round %d: %v", round, err)
		}
		if err := runHealthCheckOnce(runCtx, testNode); err != nil {
			s.Fatalf("Failed on health check at round %d: %v", round, err)
		}
	}
}

func runSuspendResumeOnce(ctx context.Context, node *audio.CrasNode) error {
	const (
		stepSleepTime   = 1 * time.Second
		suspendWaitTime = 15 * time.Second
	)

	// Start a test command process before suspend.
	// The command requests audio stream with the pinned device to ensure
	// the opened device is as expectation (given that Chrome browser is on
	// during the test).
	var cmd *testexec.Cmd
	if node.IsInput {
		cmd = crastestclient.CaptureCommand(ctx, 0, testBlockSize)
	} else {
		cmd = crastestclient.PlaybackCommand(ctx, 0, testBlockSize)
	}
	cmd.Args = append(cmd.Args, "--pin_device", strconv.FormatUint(node.ID>>32, 10))
	if err := cmd.Start(); err != nil {
		return errors.Wrap(err, "failed to start cras_test_client cmd")
	}
	defer func() {
		// Terminate the process after resumed.
		cmd.Kill()
		err := cmd.Wait()
		if exitError, ok := err.(*exec.ExitError); ok && exitError != nil && !exitError.Exited() {
			// The process killed by a signal is expected.
			return
		}
		if err != nil {
			// Record other unforeseen process states if obtained.
			testing.ContextLog(ctx, "WARN: cmd is not exited by SIGKILL; cmd.Wait() got: ", err)
		}
	}()

	// GoBigSleepLint: Wait one second for audio processing stream to be ready.
	//                 TODO(b/165995912) remove the sleep once we can query
	//                 stream state from CRAS.
	if err := testing.Sleep(ctx, stepSleepTime); err != nil {
		return errors.Wrap(err, "timed out on sleep")
	}

	// Run suspend_stress_test to simulate system suspend and resume once.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		out, err := testexec.CommandContext(ctx, "/usr/bin/suspend_stress_test", "-c", "1").Output()
		if err != nil {
			return errors.Wrap(err, "failed to run suspend_stress_test cmd")
		}
		if strings.Contains(string(out), "failed") {
			// Record errors reported from suspend_stress_test and proceed.
			testing.ContextVLogf(ctx, "output log: %q", string(out))
			testing.ContextLog(ctx, "WARN: suspend_stress_test finished with error(s)")
		}
		return nil
	}, &testing.PollOptions{Timeout: suspendWaitTime}); err != nil {
		return errors.Wrap(err, "suspend_stress_test timed out")
	}

	// GoBigSleepLint: Wait one second for audio processing stream to be ready.
	//                 TODO(b/165995912) remove the sleep once we can query
	//                 stream state from CRAS.
	if err := testing.Sleep(ctx, stepSleepTime); err != nil {
		return errors.Wrap(err, "timed out on sleep")
	}
	return nil
}

func runHealthCheckOnce(ctx context.Context, node *audio.CrasNode) error {
	const (
		healthCheckDuration = 1
		healthCheckWaitTime = 5 * time.Second
	)

	// Run a test command process of 1-second duration for health check.
	var cmd *testexec.Cmd
	if node.IsInput {
		cmd = crastestclient.CaptureCommand(ctx, healthCheckDuration, testBlockSize)
	} else {
		cmd = crastestclient.PlaybackCommand(ctx, healthCheckDuration, testBlockSize)
	}
	cmd.Args = append(cmd.Args, "--pin_device", strconv.FormatUint(node.ID>>32, 10))

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := cmd.Run(); err != nil {
			return errors.Wrap(err, "failed to run cras_test_client cmd")
		}
		return nil
	}, &testing.PollOptions{Timeout: healthCheckWaitTime}); err != nil {
		return errors.Wrap(err, "cras_test_client timed out")
	}

	// Check whether DSP coredump exists in dmesg error logs, e.g.
	// [ 1356.568869] sof-audio-pci-intel-mtl 0000:00:1f.3: ------------[ DSP dump start ]------------
	//                                                                    ^^^^^^^^^^^^^^
	// [ 1356.568873] sof-audio-pci-intel-mtl 0000:00:1f.3: IPC timeout
	// [ 1356.568878] sof-audio-pci-intel-mtl 0000:00:1f.3: fw_state: SOF_FW_BOOT_COMPLETE (7)
	// ......
	dmesgOut, err := testexec.CommandContext(ctx, "dmesg", "-r", "-l", "err").Output()
	if err != nil {
		return errors.Wrap(err, "failed to dump dmesg error logs")
	}
	testing.ContextLogf(ctx, "dmesg error output: %q", string(dmesgOut))
	if strings.Contains(string(dmesgOut), "DSP dump start") {
		return errors.New("DSP coredump detected in dmesg")
	}

	// Check whether the audio device failed in the last open according to device info via cras_tests.
	lastOpenResult, err := debug.GetLastOpenResult(ctx, node)
	if err != nil {
		return errors.Wrap(err, "failed to get last open result")
	}
	if lastOpenResult != debug.ResultSuccess {
		return errors.Errorf("last open result unsuccessful: %s", lastOpenResult)
	}
	return nil
}
