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
		Attr:         []string{"group:gsc", "gsc_dt_ab", "gsc_dt_shield", "gsc_nightly"},
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
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	b := utils.NewDevboardHelper(s)

	th.MustSucceed(b.Open(ctx), "Open gsc UART")
	defer b.Close(ctx)

	b.Reset(ctx)

	// Deassert PLT_RST_L to prevent deep sleep while tests are running.
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)

	hasKernelTests := s.Param().(bool)
	if hasKernelTests {
		s.Log("Kernel tests:")
		checkTestResults(ctx, s, b, "KERNEL")
	}

	s.Log("App tests:")
	checkTestResults(ctx, s, b, "APP")
}

func checkTestResults(ctx context.Context, s *testing.State, b utils.DevboardHelper, sectionName string) {
	// Reporting fatal console output such as "Kernel panic" is a feature of
	// CommandImage::WaitUntilMatch().  We cannot use that here, because system_test_auto does
	// not support a command prompt.  Eventually, we want to either "upgrade" system_test_auto
	// to have a prompt (something that has been proposed before for other reasons), or we
	// could create a SerialOutputImage base class, move WaitUntilMatch() down into that one,
	// and make CommandImage derive from it.
	idx, m, err := b.ReadSerialSubmatch(ctx, ti50.FatalMsg, regexp.MustCompile("##"+regexp.QuoteMeta(sectionName)+" TESTS START"))
	if err != nil {
		s.Fatal("Failed to read section start: ", err)
	}
	if idx == 0 {
		s.Fatal("Fatal output: ", strings.TrimRight(string(m[0]), "\r\n"))
	}
	endMarker := "##" + regexp.QuoteMeta(sectionName) + " TESTS END"
	re := regexp.MustCompile("(" + endMarker + `|##TEST (SKIP|START) (\S+)\s)`)
	for {
		idx, m, err := b.ReadSerialSubmatch(ctx, ti50.FatalMsg, re)
		if err != nil {
			s.Fatal("Failed to read next test: ", err)
		}
		if idx == 0 {
			s.Fatal("Fatal output: ", strings.TrimRight(string(m[0]), "\r\n"))
		}
		match := string(m[0])
		if match == endMarker {
			return
		}
		start := string(m[2])
		testName := string(m[3])
		result := "Skip"
		if start != "SKIP" {
			result = waitForTest(ctx, s, b, testName)
		}
		if result == "Fail" {
			s.Errorf("%s test failed", testName)
		}
	}
}

func waitForTest(ctx context.Context, s *testing.State, b utils.DevboardHelper, testName string) string {
	lineRe := regexp.MustCompile(`.*[\r\n]+`)
	slowCryptoRe := regexp.MustCompile("Long running SW crypto operation")
	resultRe := regexp.MustCompile("##TEST RESULT " + regexp.QuoteMeta(testName) + `: (\S+)`)
	testTime := time.Now()
	var line string
	lineTime := time.Now()
	timeLimit := timeLimit

	var elapsedTime time.Duration
	for ; elapsedTime < timeLimit; elapsedTime = time.Since(testTime) {
		idx, m, err := b.ReadSerialSubmatch(ctx, ti50.FatalMsg, lineRe)
		if err != nil {
			// Tests might be silent for several seconds, so just
			// try the read again.
			continue
		}
		if idx == 0 {
			s.Fatal("Fatal output: ", strings.TrimRight(string(m[0]), "\r\n"))
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
