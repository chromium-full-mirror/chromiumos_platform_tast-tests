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
	"go.chromium.org/tast-tests/cros/local/graphics/expectations"
	"go.chromium.org/tast-tests/cros/local/gtest"
	"go.chromium.org/tast/core/testing"
)

// clvkTest is used to describe the config used to run each test.
type clvkTest struct {
	exe string // The test executable name.
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         Clvk,
		Desc:         "Run OpenCL implementation on top of Vulkan using clvk",
		BugComponent: "b:1171198", // ChromeOS > Platform > Graphics > GPU > OpenCL
		Contacts: []string{
			"chromeos-gfx@google.com",
			"rjodin@chromium.org",
		},
		Attr:         []string{},
		SoftwareDeps: []string{"vulkan"},
		Fixture:      "graphicsNoChrome",
		Params: []testing.Param{{
			Name: "api_tests",
			Val: clvkTest{
				exe: "api_tests",
			},
			Timeout:   1 * time.Minute,
			ExtraAttr: []string{"group:graphics", "graphics_opencl", "graphics_perbuild"},
		}, {
			Name: "simple_test",
			Val: clvkTest{
				exe: "simple_test",
			},
			Timeout:   1 * time.Minute,
			ExtraAttr: []string{"group:mainline", "group:cq-medium"},
		}},
	})
}

func Clvk(ctx context.Context, s *testing.State) {
	test := s.Param().(clvkTest)

	// Allow to see clvk error and warn messages directly in test logFile.
	os.Setenv("CLVK_LOG", "2")
	// Make sure the test has write permission in clvk's temporary folder.
	os.Setenv("CLVK_COMPILER_TEMP_DIR", os.TempDir())

	expectation, err := expectations.GetTestExpectation(ctx, s.TestName())
	if err != nil {
		s.Fatal("Failed to load test expectation: ", err)
	}
	// Schedules a post-test expectations handling. If the test is expected to
	// fail, but did not, then this generates an error.
	defer func() {
		if err := expectation.HandleFinalExpectation(); err != nil {
			s.Error("Unmet expectation: ", err)
		}
	}()

	const testPath = "/usr/local/opencl"
	testExec := filepath.Join(testPath, test.exe)
	logFile := filepath.Join(s.OutDir(), filepath.Base(test.exe)+".txt")
	if test.exe == "api_tests" {
		if report, err := gtest.New(testExec,
			gtest.Logfile(logFile),
		).Run(ctx); err != nil && report != nil {
			passedTests := report.PassedTestNames()
			failedTests := report.FailedTestNames()
			if expErr := expectation.ReportErrorf("Passed %d tests, failed %d tests (%s) - %v", len(passedTests), len(failedTests), failedTests, err); expErr != nil {
				s.Error("Unexpected error: ", expErr)
			}
		} else if err != nil && report == nil {
			if expErr := expectation.ReportError(err); expErr != nil {
				s.Error("Unexpected error: ", expErr)
			}
		}
	} else {
		f, err := os.Create(logFile)
		if err != nil {
			s.Fatal("Failed to create a log file: ", err)
		}
		defer f.Close()

		cmd := testexec.CommandContext(ctx, testExec)
		cmd.Stdout = f
		cmd.Stderr = f
		if err = cmd.Run(testexec.DumpLogOnError); err != nil {
			if expErr := expectation.ReportError(err); expErr != nil {
				s.Error("Unexpected error: ", expErr)
			}
		}
	}
}
