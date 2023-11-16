// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast-tests/cros/local/graphics/expectations"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// v4l2SummaryRegExp is the regexp to find the summary result from the binary log.
var v4l2SummaryRegExp = regexp.MustCompile(`Total.*: \d+, Succeeded: \d+, Failed: \d+, Warnings: \d+`)

func init() {
	testing.AddTest(&testing.Test{
		Func: PlatformV4L2,
		Desc: "Runs v4l2 compliance tests",
		Contacts: []string{
			"chromeos-gfx-video@google.com",
			"stevecho@chromium.org",
		},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		Attr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
		// TODO(b/280450423): revisit this issue after Asurada kernel uprev
		// from current kernel 5.4
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("hayato", "spherion")),
		SoftwareDeps: []string{"v4l2_codec"},
		Timeout:      2 * time.Minute,
		Fixture:      "gpuWatchHangs",
		Params: []testing.Param{{
			// -v: Turn on verbose reporting.
			Name: "decoder",
			Val:  []string{"v4l2-compliance", "-d", "/dev/video-dec0", "-v"},
		}},
	})
}

// PlatformV4L2 runs v4l2-compliance binary test.
func PlatformV4L2(ctx context.Context, s *testing.State) {
	// Test doesn't use the graphicsNoChrome fixture since the driver may
	// write errors to the kernel logs which are picked up by the GPU
	// watchdog.
	if err := upstart.StopJob(ctx, "ui"); err != nil {
		s.Fatal("Failed to stop ui job: ", err)
	}
	defer upstart.EnsureJobRunning(ctx, "ui")

	expectation, err := expectations.GetTestExpectation(ctx, s.TestName())
	if err != nil {
		s.Fatal("Unable to get test expectation: ", err)
	}
	// Schedules a post-test expectations handling. If the test is expected to
	// fail, but did not, then this generates an error.
	defer func() {
		if err := expectation.HandleFinalExpectation(); err != nil {
			s.Error("Unmet expectation: ", err)
		}
	}()

	// TODO(b/311270670): Re-enable after we can disable just [MTK_V4L2][ERROR] logs
	graphics.DisableSysLogCheck()

	command := s.Param().([]string)

	s.Log("Running ", shutil.EscapeSlice(command))

	logFile := filepath.Join(s.OutDir(), filepath.Base(command[0])+".txt")

	f, err := os.Create(logFile)
	if err != nil {
		s.Fatal("Failed to create a log file: ", err)
	}
	defer f.Close()

	cmd := testexec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Stdout = f
	cmd.Stderr = f
	if err := cmd.Run(testexec.DumpLogOnError); err != nil {
		exitCode, ok := testexec.ExitCode(err)
		if !ok {
			s.Fatalf("Failed to run %s: %v", command[0], err)
		}

		contents, err := ioutil.ReadFile(logFile)
		if err != nil {
			s.Fatal("Failed to read the log file: ", err)
		}

		matches := v4l2SummaryRegExp.FindAllStringSubmatch(string(contents), -1)
		if matches == nil {
			s.Fatal("Failed to find matches for summary result")
		}
		if len(matches) != 1 {
			s.Fatalf("Found %d matches for summary result; want 1", len(matches))
		}

		if exitCode > 0 {
			if expErr := expectation.ReportErrorf("%s", matches); expErr != nil {
				s.Error("Unexpected error: ", expErr)
			}
		} else {
			s.Logf("%s", matches)
		}
	}
}
