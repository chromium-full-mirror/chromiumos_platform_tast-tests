// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RecordAudio,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test audio record file",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291",
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.Microphone()),
		Attr:         []string{"group:intel-nda"},
		Timeout:      10 * time.Minute,
		Fixture:      "chromeLoggedIn",
	})
}

func RecordAudio(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*chrome.Chrome)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}
	kb, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to open the keyboard: ", err)
	}
	defer kb.Close(ctx)

	out, err := testexec.CommandContext(ctx, "arecord", "-l").CombinedOutput(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to check arecord command: ", err)
	}
	arecordOutput := string(out)
	if strings.Contains(arecordOutput, "no soundcards found") {
		s.Fatal("Failed to recognize sound cards")
	}

	if !strings.Contains(arecordOutput, "DMIC") {
		s.Fatal("Failed to find DMIC in arecord command")
	}

	re := regexp.MustCompile(`card (\d+):`)
	cardInfo := re.FindAllStringSubmatch(arecordOutput, -1)
	if cardInfo == nil {
		s.Fatal("Failed to get card info from arecord command")
	}
	cardNo := cardInfo[0][1]

	reDevice := regexp.MustCompile(`device (\d+):\sDMIC`)
	deviceInfo := reDevice.FindAllStringSubmatch(arecordOutput, -1)
	if deviceInfo == nil {
		s.Fatal("Failed to get device info from arecord command")
	}
	deviceNo := deviceInfo[0][1]

	recWavFileName := "30SEC_REC.wav"
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's Download path: ", err)
	}
	recWavFile := filepath.Join(downloadsPath, recWavFileName)

	arecordArgs := []string{fmt.Sprintf("-Dhw:%s,%s", cardNo, deviceNo),
		"-d", "30", // duration
		"-f", "S32_LE", // format
		"-c", "4", // number of channels
		"-r", "48000", // sample rate
		recWavFile, // output file
	}
	cmd := testexec.CommandContext(ctx, "arecord", arecordArgs...)
	if err := cmd.Run(testexec.DumpLogOnError); err != nil {
		s.Fatalf("Failed to execute %q: %v", cmd, err)
	}

	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch the Files App: ", err)
	}
	defer func(ctx context.Context) {
		if err := files.Close(ctx); err != nil {
			s.Error("Failed to close the Files App: ", err)
		}
		if err := os.Remove(filepath.Join(downloadsPath, recWavFileName)); err != nil {
			s.Errorf("Failed to delete file %q: %v", recWavFileName, err)
		}
		if err = kb.Accel(ctx, "Ctrl+W"); err != nil {
			s.Error("Failed to close Audio player: ", err)
		}
	}(cleanupCtx)

	if err := uiauto.Combine("open wav file in downloads",
		files.OpenDownloads(),
		files.OpenFile(recWavFileName),
	)(ctx); err != nil {
		s.Fatal("Failed to open file in downloads: ", err)
	}

	// GoBigSleepLint: Sample time for the audio to play for 5 seconds.
	if err := testing.Sleep(ctx, 5*time.Second); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	// Get Current active node.
	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Failed to create Cras object")
	}
	audioDeviceName, _, err := cras.SelectedOutputDevice(ctx)
	if err != nil {
		s.Fatal("Failed to get the selected audio device: ", err)
	}

	devName, err := crastestclient.FirstRunningDevice(ctx, audio.OutputStream)
	if err != nil {
		s.Fatal("Failed to detect running output device: ", err)
	}

	if audioDeviceName != devName {
		s.Fatalf("Failed to route the audio through expected audio node: got %q; want %q", devName, audioDeviceName)
	}
}
