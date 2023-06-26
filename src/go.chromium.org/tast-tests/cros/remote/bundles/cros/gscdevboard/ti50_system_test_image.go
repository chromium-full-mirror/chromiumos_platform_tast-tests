// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"

	"go.chromium.org/tast/core/testing"
)

const timeLimit = 2 * time.Minute

func init() {
	testing.AddTest(&testing.Test{
		Func:    Ti50SystemTestImage,
		Desc:    "Ti50 system test",
		Timeout: 20 * time.Minute,
		Contacts: []string{
			"chromeos-faft@google.com",
			"ecgh@chromium.org",
			"ti50-core@google.com",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_ab"},
		Params: []testing.Param{{
			Name:      "sta",
			Val:       true, // hasKernelTests
			Fixture:   fixture.SystemTestAutoDevboard,
			ExtraAttr: []string{"gsc_image_sta", "gsc_he"},
		}, {
			Name:      "sta2",
			Val:       false, // hasKernelTests
			Fixture:   fixture.SystemTestAuto2Devboard,
			ExtraAttr: []string{"gsc_image_sta2", "gsc_he"},
		}},
	})
}

func Ti50SystemTestImage(ctx context.Context, s *testing.State) {

	f := s.FixtValue().(*fixture.Value)
	board := utils.NewDevboardHelper(f.DevBoard(), s)

	err := board.Open(ctx)
	if err != nil {
		s.Fatal("Open console port: ", err)
	}

	if err = board.Reset(ctx); err != nil {
		s.Fatal("Failed to reset: ", err)
	}

	// Deassert PLT_RST_L to prevent deep sleep while tests are running.
	board.GpioSet(ctx, ti50.GpioTi50PltRstL, true)

	hasKernelTests := s.Param().(bool)
	if hasKernelTests {
		if f.TestbedType == ti50.GscHostEmulation {
			// TODO(b/287468118 b/288103083) Kernel tests should pass in host emulation.
			s.Log("Skipping kernel tests due to host emulation bugs")
		} else {
			s.Log("Kernel tests:")
			checkTestResults(ctx, s, board, "KERNEL")
		}
	}

	s.Log("App tests:")
	checkTestResults(ctx, s, board, "APP")
}

func checkTestResults(ctx context.Context, s *testing.State, board ti50.DevBoard, sectionName string) {
	_, err := board.ReadSerialSubmatch(ctx, regexp.MustCompile("##"+regexp.QuoteMeta(sectionName)+" TESTS START"))
	if err != nil {
		s.Fatal("Failed to read section start: ", err)
	}
	endMarker := "##" + regexp.QuoteMeta(sectionName) + " TESTS END"
	re := regexp.MustCompile("(" + endMarker + `|##TEST (SKIP|START) (\S+)\s)`)
	for {
		m, err := board.ReadSerialSubmatch(ctx, re)
		if err != nil {
			s.Fatal("Failed to read next test: ", err)
		}
		match := string(m[0])
		if match == endMarker {
			return
		}
		start := string(m[2])
		testName := string(m[3])
		result := "Skip"
		if start != "SKIP" {
			result = waitForTest(ctx, s, board, testName)
		}
		if result == "Fail" {
			s.Errorf("%s test failed", testName)
		}
	}
}

func waitForTest(ctx context.Context, s *testing.State, board ti50.DevBoard, testName string) string {
	lineRe := regexp.MustCompile(`.*[\r\n]+`)
	slowCryptoRe := regexp.MustCompile("Long running SW crypto operation")
	resultRe := regexp.MustCompile("##TEST RESULT " + regexp.QuoteMeta(testName) + `: (\S+)`)
	testTime := time.Now()
	var line string
	lineTime := time.Now()
	timeLimit := timeLimit

	var elapsedTime time.Duration
	for ; elapsedTime < timeLimit; elapsedTime = time.Since(testTime) {
		m, err := board.ReadSerialSubmatch(ctx, lineRe)
		if err != nil {
			// Tests might be silent for several seconds, so just
			// try the read again.
			continue
		}
		delay := time.Since(lineTime)
		if delay > 10*time.Second {
			s.Logf("(%q took %v)", line, delay.Round(time.Second))
		}
		lineTime = time.Now()
		line = strings.TrimSpace(string(m[0]))
		if m := resultRe.FindStringSubmatch(line); m != nil {
			result := m[1]
			s.Logf("%s: %s (%v)", testName, result, elapsedTime.Round(time.Second))
			return result
		}
		if slowCryptoRe.MatchString(line) {
			timeLimit += 10 * time.Minute
			s.Log("(Waiting for slow crypto.)")
		}
	}
	s.Logf("Still waiting for test %s after %v, giving up", testName, elapsedTime.Round(time.Second))
	delay := time.Since(lineTime)
	s.Logf("Waited %v at %q", delay.Round(time.Second), line)
	s.Fatalf("%s test failed to finish in time", testName)
	return ""
}
