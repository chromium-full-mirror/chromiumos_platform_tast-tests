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
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/utils"
	"go.chromium.org/tast/core/testing"
)

const timeLimit = 2 * time.Minute

type testSystemTestConfig struct {
	hasKernelTests  bool
	backgroundRegex *regexp.Regexp
}

func init() {
	testing.AddTest(&testing.Test{
		Func:    Ti50SystemTestImage,
		Desc:    "Ti50 system test",
		Timeout: 45 * time.Minute,
		Contacts: []string{
			"cros-hwsec@google.com", // CrOS GSC Developers
			"ecgh@chromium.org",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_dt_ab", "gsc_dt_shield", "gsc_ot_shield", "gsc_ot_fpga_cw310", "gsc_he",
			"gsc_nightly"},
		Params: []testing.Param{{
			Name: "sta",
			Val: testSystemTestConfig{
				hasKernelTests: true},
			Fixture:   fixture.SystemTestAutoDevboard,
			ExtraAttr: []string{"gsc_image_sta"},
		}, {
			Name: "sta2",
			Val: testSystemTestConfig{
				hasKernelTests:  false,
				backgroundRegex: regexp.MustCompile(`&&BACKGROUND (START|RESULT: (\S+))`)},
			Fixture:   fixture.SystemTestAuto2Devboard,
			ExtraAttr: []string{"gsc_image_sta2"},
		}, {
			Name: "sta_a",
			Val: testSystemTestConfig{
				hasKernelTests: false},
			Fixture:   fixture.SystemTestAutoADevboard,
			ExtraAttr: []string{"gsc_image_sta_a"},
		}},
	})
}

var backgroundData struct {
	start  time.Time
	end    time.Time
	result string
}

func processBackground(ctx context.Context, s *testing.State, match string) {
	config := s.Param().(testSystemTestConfig)
	if m := config.backgroundRegex.FindStringSubmatch(match); m != nil {
		// There is only one background regex at the moment, so all of these
		// matching groups are specifically for sta2
		if string(m[1]) == "START" {
			backgroundData.start = time.Now()
		} else {
			backgroundData.end = time.Now()
			backgroundData.result = string(m[2])
		}
	} else {
		s.Fatal("Could not rematch background string: ", match)
	}
}

func Ti50SystemTestImage(ctx context.Context, s *testing.State) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	config := s.Param().(testSystemTestConfig)

	th.MustSucceed(b.Open(ctx), "Open gsc UART")
	defer b.Close(ctx)

	b.Reset(ctx)

	// Deassert PLT_RST_L to prevent deep sleep while tests are running.
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)

	// Can never match since word boundary isn't in an empty string
	neverMatches := regexp.MustCompile(`^\b$`)

	if config.hasKernelTests {
		s.Log("Kernel tests:")
		checkTestResults(ctx, s, b, "KERNEL", neverMatches)
	}

	s.Log("App tests:")
	background := neverMatches
	if config.backgroundRegex != nil {
		background = config.backgroundRegex
	}
	checkTestResults(ctx, s, b, "APP", background)

	// Report background info if part of test
	if config.backgroundRegex != nil {
		// If the background result isn't present
		if backgroundData.result == "" {
			waitForBackgroundRegex(ctx, s, b, background)
		}
		// We should have waited long enough, fail if no status
		if backgroundData.result == "" {
			s.Fatal("Background task never finished")
		}
		delta := backgroundData.end.Sub(backgroundData.start)
		s.Logf("background: %s (%v)", backgroundData.result, delta.Round(time.Second))
		if backgroundData.result == "Fail" {
			s.Error("background test failed")
		}
	}
}

func waitForBackgroundRegex(ctx context.Context, s *testing.State, b utils.DevboardHelper, re *regexp.Regexp) {
	start := time.Now()
	var elapsedTime time.Duration
	for ; elapsedTime < timeLimit; elapsedTime = time.Since(start) {
		idx, m, err := b.ReadSerialSubmatch(ctx, ti50.FatalMsg, re)
		if err != nil {
			// Tests might be silent for several seconds, so just
			// try the read again.
			continue
		}
		if idx == 0 {
			s.Fatal("Fatal output: ", strings.TrimRight(string(m[0]), "\r\n"))
		}
		processBackground(ctx, s, strings.TrimSpace(string(m[0])))
		return
	}
}

func checkTestResults(ctx context.Context, s *testing.State, b utils.DevboardHelper, sectionName string, background *regexp.Regexp) {
	// Reporting fatal console output such as "Kernel panic" is a feature of
	// CommandImage::WaitUntilMatch().  We cannot use that here, because system_test_auto does
	// not support a command prompt.  Eventually, we want to either "upgrade" system_test_auto
	// to have a prompt (something that has been proposed before for other reasons), or we
	// could create a SerialOutputImage base class, move WaitUntilMatch() down into that one,
	// and make CommandImage derive from it.
	for {
		idx, m, err := b.ReadSerialSubmatch(ctx, ti50.FatalMsg, background, regexp.MustCompile("##"+regexp.QuoteMeta(sectionName)+" TESTS START"))
		if err != nil {
			s.Fatal("Failed to read section start: ", err)
		}
		if idx == 0 {
			s.Fatal("Fatal output: ", strings.TrimRight(string(m[0]), "\r\n"))
		}
		if idx == 1 {
			processBackground(ctx, s, string(m[0]))
			continue
		}
		if idx == 2 {
			// We found the start of the normal APP or KERNEL tests, continue to
			// look for individual tests.
			break
		}
	}
	endMarker := "##" + regexp.QuoteMeta(sectionName) + " TESTS END"
	re := regexp.MustCompile("(" + endMarker + `|##TEST (SKIP|START) (\S+)\s)`)
	for {
		idx, m, err := b.ReadSerialSubmatch(ctx, ti50.FatalMsg, background, re)
		if err != nil {
			s.Fatal("Failed to read next test: ", err)
		}
		if idx == 0 {
			s.Fatal("Fatal output: ", strings.TrimRight(string(m[0]), "\r\n"))
		}
		if idx == 1 {
			processBackground(ctx, s, string(m[0]))
			continue
		}
		match := string(m[0])
		if match == endMarker {
			return
		}
		start := string(m[2])
		testName := string(m[3])
		result := "Skip"
		if start != "SKIP" {
			result = waitForTest(ctx, s, b, testName, background)
		}
		if result == "Fail" {
			s.Errorf("%s test failed", testName)
		}
	}
}

func waitForTest(ctx context.Context, s *testing.State, b utils.DevboardHelper, testName string, background *regexp.Regexp) string {
	lineRe := regexp.MustCompile(`.*[\r\n]+`)
	slowCryptoRe := regexp.MustCompile("Long running SW crypto operation")
	resultRe := regexp.MustCompile("##TEST RESULT " + regexp.QuoteMeta(testName) + `: (\S+)`)
	testTime := time.Now()
	var line string
	lineTime := time.Now()
	timeLimit := timeLimit
	if testName == "tpm" {
		// Allow additional time as long as we have only software cryptolib.
		if b.TestbedType == ti50.GscOTShield {
			timeLimit = 15 * time.Minute
		} else if b.TestbedType == ti50.GscOpentitanCw310Fpga {
			timeLimit = 30 * time.Minute
		}
	}

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
		if background.MatchString(line) {
			processBackground(ctx, s, line)
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
