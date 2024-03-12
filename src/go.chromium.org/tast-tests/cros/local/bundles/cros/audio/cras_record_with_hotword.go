// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/audio/fixture"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrasRecordWithHotword,
		Desc:         "Verifies CRAS record function works correctly with hotword",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "bailideng@google.com"},
		BugComponent: "b:776546",
		HardwareDeps: hwdep.D(hwdep.Microphone()),
		Fixture:      fixture.UIStopped{Parent: "rebootForAudioDSPFixture"}.Instance(),
		Attr:         []string{"group:mainline", "informational"},
	})
}

func CrasRecordWithHotword(ctx context.Context, s *testing.State) {
	const duration = 5 * time.Second
	const rate = 48000

	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Failed to connect to CRAS: ", err)
	}
	if err := cras.SetActiveNodeByType(ctx, "INTERNAL_MIC"); err != nil {
		crastestclient.DumpAudioDiagnostics(ctx, s.OutDir())
		s.Fatal("Failed to set internal mic to active: ", err)
	}

	// Set timeout to duration + 10s, which is the time buffer to complete the normal execution.
	runCtx, cancel := context.WithTimeout(ctx, duration+10*time.Second)
	defer cancel()

	// Start hotword and capture.
	clientCtx, clientCancel := context.WithCancel(runCtx)
	defer clientCancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func(runCtx context.Context) {
		defer wg.Done()
		cmd := exec.CommandContext(runCtx, "cras_test_client", "--listen_for_hotword", "/dev/null")
		if err := cmd.Run(); err != nil && runCtx.Err() == nil {
			s.Error("Hotword capture failed: ", err)
		}
	}(clientCtx)

	if err := crastestclient.DumpAudioDiagnostics(runCtx, s.OutDir()); err != nil {
		s.Error("Failed to dump audio diagnostics: ", err)
	}

	// Run capture.
	recording := audio.TestRawData{
		Path:          filepath.Join(s.OutDir(), "capture.raw"),
		BitsPerSample: 16,
		Channels:      2,
		Rate:          rate,
		Duration:      int(duration.Seconds()),
	}

	testing.ContextLog(runCtx, "Capture output to ", recording.Path)
	if err := crastestclient.CaptureFileCommand(
		runCtx, recording.Path,
		recording.Duration,
		recording.Channels,
		recording.Rate).Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to capture: ", err)
	}

	// Stop hotword capture.
	clientCancel()
	wg.Wait()
}
