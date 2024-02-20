// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/crosconfig"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
	"golang.org/x/exp/slices"
)

// TODO(b/239385484) : remove "beetley" when b/239385484 is fixed.
// TODO(b/245058202) : remove "bob" when b/245058202 is fixed.
// TODO(b/285830729) : remove "geralt" when b/285830729 is fixed.
// TODO(b/312097873) : remove "brya" when b/309904720 is fixed.
var alsaConformanceUnstableModels = []string{"beetley", "bob", "geralt", "brya"}

// TODO(b/136614687): Relex the criteria for grunt devices, the audio still sounds fine as CRAS can compensate the rate, if the rate error is not huge.
var relexedCriteriaModels = []string{"aleena", "barla", "careena", "kasumi", "kasumi360", "liara", "treeya360", "treeya"}

// AEC requires the processing data in s16le format while the present SOF does not respect the capabilities for processing modules. The client will still get available formats s16,s24,s32le on querying PCM capabilities (device info).
// TODO(b/244418775): remove this when b/244418775 is fixed
var fixFormatS16CaptureModels = []string{"redrix", "gimble", "anahera", "yaviks", "yavikso"}

var mergeThresholdSize480Boards = []string{"volteer", "brya", "nissa"}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ALSAConformance,
		Desc:         "Runs alsa_conformance_test to test basic functions of ALSA",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "yuhsuan@chromium.org", "cychiang@chromium.org"},
		BugComponent: "b:776546",
		Attr:         []string{"group:mainline"},
		HardwareDeps: hwdep.D(hwdep.Speaker(), hwdep.Microphone()),
		Fixture:      "rebootForAudioDSPFixture",
		Timeout:      10 * time.Minute,
		Params: []testing.Param{
			{
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(alsaConformanceUnstableModels...)),
				ExtraAttr:         []string{"group:cq-medium", "group:cq-minimal"},
			},
			{
				Name:              "unstable",
				ExtraHardwareDeps: hwdep.D(hwdep.Model(alsaConformanceUnstableModels...)),
				ExtraAttr:         []string{"informational"},
			},
		},
	})
}

func ALSAConformance(ctx context.Context, s *testing.State) {
	model, err := crosconfig.Get(ctx, "/", "name")
	if err != nil {
		s.Fatal("Failed query model: ", err)
	}

	// TODO(yuhsuan): Tighten the ratio if the current version is stable. (b/136614687)
	rateCriteria := 0.1
	rateErrCriteria := 100.0
	if slices.Contains(relexedCriteriaModels, model) {
		rateCriteria = 0.15
	}

	// Stop UI in advance for this test to avoid the node being selected by UI.
	if err := upstart.StopJob(ctx, "ui"); err != nil {
		s.Fatal("Failed to stop ui: ", err)
	}
	defer upstart.EnsureJobRunning(ctx, "ui")

	// Use a shorter context to save time for cleanup.
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Failed to connect to CRAS: ", err)
	}

	// Only test on internal mic and internal speaker until below demands are met.
	// 1. Support label to force the test run on DUT having a headphone jack. (crbug.com/936807)
	// 2. Have a method to get correct PCM name from CRAS. (b/142910355).
	if err := cras.SetActiveNodeByType(ctx, "INTERNAL_MIC"); err != nil {
		crastestclient.DumpAudioDiagnostics(ctx, s.OutDir())
		s.Fatal("Failed to set internal mic active: ", err)
	}

	if err := cras.SetActiveNodeByType(ctx, "INTERNAL_SPEAKER"); err != nil {
		crastestclient.DumpAudioDiagnostics(ctx, s.OutDir())
		s.Fatal("Failed to set internal speaker active: ", err)
	}

	crasNodes, err := cras.GetNodes(ctx)
	if err != nil {
		crastestclient.DumpAudioDiagnostics(ctx, s.OutDir())
		s.Fatal("Failed to obtain CRAS nodes: ", err)
	}

	// Stop CRAS to make sure the audio device won't be occupied.
	s.Log("Stopping CRAS")
	if err := upstart.StopJob(ctx, "cras"); err != nil {
		s.Fatal("Failed to stop CRAS: ", err)
	}

	defer func(ctx context.Context) {
		// GoBigSleepLint: Sleep for 10 seconds to reset drivers and then restart CRAS.
		testing.Sleep(ctx, 10*time.Second)
		s.Log("Starting CRAS")
		if err := upstart.EnsureJobRunning(ctx, "cras"); err != nil {
			s.Fatal("Failed to start CRAS: ", err)
		}
	}(ctx)

	// Use a shorter context to save time for cleanup.
	ctx, cancel = ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	// checkOutput parses and checks out, stdout from alsa_conformance_test.py.
	// It returns the number of failed tests and failure reasons.
	checkOutput := func(out []byte) (numFails int, failReasons []string) {
		result := struct {
			Pass       int `json:"pass"`
			Fail       int `json:"fail"`
			TestSuites []struct {
				Name  string `json:"name"`
				Pass  int    `json:"pass"`
				Fail  int    `json:"fail"`
				Tests []struct {
					Name   string `json:"name"`
					Result string `json:"result"`
					Error  string `json:"error"`
				} `json:"tests"`
			} `json:"testSuites"`
		}{}

		if err := json.Unmarshal(out, &result); err != nil {
			s.Fatal("Failed to unmarshal test results: ", err)
		}
		s.Logf("alsa_conformance_test.py results: %d passed %d failed", result.Pass, result.Fail)

		for _, suite := range result.TestSuites {
			for _, test := range suite.Tests {
				if test.Result != "pass" {
					failReasons = append(failReasons, test.Name+": "+test.Error)
				}
			}
		}

		return result.Fail, failReasons
	}

	runTest := func(stream audio.StreamType) {

		var node *audio.CrasNode
		for i, n := range crasNodes {
			if n.Active && n.IsInput == (stream == audio.InputStream) {
				node = &crasNodes[i]
				break
			}
		}

		if node == nil {
			s.Fatal("Failed to find selected device: ", err)
		}

		s.Logf("Selected %s device: %s", stream, node.DeviceName)
		alsaDev := "hw:" + strings.Split(node.DeviceName, ":")[2]
		s.Logf("Running alsa_conformance_test on %s device %s", stream, alsaDev)

		var arg string
		if stream == audio.InputStream {
			arg = "-C"
		} else {
			arg = "-P"
		}
		var args = []string{arg, alsaDev, "--rate-criteria-diff-pct", fmt.Sprintf("%f", rateCriteria), "--rate-err-criteria", fmt.Sprintf("%f", rateErrCriteria), "--json"}
		board, err := crosconfig.Get(ctx, "/pvs", "program")
		if err != nil {
			s.Log("Failed to query board: ", err)
			// Setting board to be empty string and it should not be run with merge-thld-size 480
			board = ""
		}
		if slices.Contains(mergeThresholdSize480Boards, board) {
			args = append(args, "--merge-thld-size", "480")
		}
		if slices.Contains(fixFormatS16CaptureModels, model) && stream == audio.InputStream {
			args = append(args, "--allow-formats", "S16_LE")
		}

		out, err := testexec.CommandContext(
			ctx, "alsa_conformance_test.py", args...).Output(testexec.DumpLogOnError)
		if err != nil {
			s.Fatal("Failed to run alsa_conformance_test: ", err)
		}

		filename := fmt.Sprintf("%s.json", stream)
		if err := os.WriteFile(filepath.Join(s.OutDir(), filename), out, 0644); err != nil {
			s.Error("Failed to save raw results: ", err)
		}

		fail, failReasons := checkOutput(out)

		if fail != len(failReasons) {
			s.Errorf("The number of failures and reasons does not match. (fail: %d, failReason: %d)", fail, len(failReasons))
		}

		if fail != 0 {
			s.Errorf("Device %s %s stream had %d failure(s): %q", alsaDev, stream, fail, failReasons)
		}
	}

	runTest(audio.InputStream)
	runTest(audio.OutputStream)
}
