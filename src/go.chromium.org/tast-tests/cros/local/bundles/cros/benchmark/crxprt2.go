// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package benchmark

import (
	"context"
	"math"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/benchmark/setup"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/cws"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const crxprtRunningTime = 45 * time.Minute

func init() {
	testing.AddTest(&testing.Test{
		Func:         CRXPRT2,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Execute Chrome extension CrXPRT 2 to do benchmark and acquire test score",
		Contacts:     []string{"chromeos-perf-reliability-eng@google.com", "cienet-development@googlegroups.com", "xibin@google.com"},
		BugComponent: "b:1025042", // ChromeOS > EngProd > Platform > SPERA > Automation
		// Purposely leave the empty Attr here. Public benchmark tests are not included in crosbolt group for now.
		Attr:         []string{},
		SoftwareDeps: []string{"chrome", "arc"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Timeout:      crxprtRunningTime + 15*time.Minute,
		VarDeps:      []string{"benchmark.username"},
		Params: []testing.Param{
			{
				Val:     browser.TypeAsh,
				Fixture: setup.BenchmarkChromeFixture,
			},
			{
				Name:              "lacros",
				ExtraSoftwareDeps: []string{"lacros"},
				Val:               browser.TypeLacros,
				Fixture:           setup.BenchmarkLacrosFixture,
			},
		},
	})
}

func CRXPRT2(ctx context.Context, s *testing.State) {
	const (
		extName     = "CrXPRT 2"
		windowName  = "CrXPRT"
		extID       = "ldeofhcgjhplegompgciolncekblpkad"
		extStoreURL = "https://chrome.google.com/webstore/detail/crxprt-2/ldeofhcgjhplegompgciolncekblpkad?pli=1&_ind=category%252Fextensions&_asi=1&source=5"
		extPageURL  = "chrome-extension://ldeofhcgjhplegompgciolncekblpkad/index.html"

		btnIDNext  = "next_btn"
		btnIDStart = "start_perf_btn"
	)

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	// Shorten context a bit to allow for cleanup.
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	bt := s.Param().(browser.Type)
	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, bt)
	if err != nil {
		s.Fatal("Failed to set up browser: ", err)
	}
	defer closeBrowser(closeCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(closeCtx, s.OutDir(), s.HasError, cr, "ui_dump")

	s.Logf("Launching %s", extName)
	if err := launchChromeExtension(ctx, br, tconn, extName, extID, extStoreURL); err != nil {
		s.Fatalf("Failed to launch %s, error: %v", extName, err)
	}

	ui := uiauto.New(tconn).WithInterval(time.Second)

	root := nodewith.Name(windowName).Role(role.RootWebArea)
	footer := nodewith.HasClass("footer").Role(role.GenericContainer)
	if err := uiauto.Combine("wait for launch",
		ui.WaitUntilExists(root),
		ui.WaitUntilExists(footer),
	)(ctx); err != nil {
		s.Fatalf("Failed to wait for %s to launch: %v", extName, err)
	}

	crxprtNodeFinder := nodewith.Ancestor(root)

	// CrXPRT 2 provide two types of test, this test case will execute the performance test.
	s.Log("Select performance test")
	pTestFinder := crxprtNodeFinder.Name("Performance test").Role(role.Link).HasClass("test_sel_perfbtn").Focusable().Linked()
	if err := ui.LeftClick(pTestFinder)(ctx); err != nil {
		s.Fatal("Failed to select performance test: ", err)
	}

	s.Log("Connecting to the app")
	conn, err := br.NewConnForTarget(ctx, chrome.MatchTargetURL(extPageURL))
	if err != nil {
		s.Fatalf("Failed to connect to extension: %s, error: %+v", extName, err)
	}
	defer conn.Close()
	defer conn.CloseTarget(closeCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(closeCtx, s.OutDir(), s.HasError, cr, "ui_dump_extension")

	const (
		enabledButtonClass  = "blue_btn"
		disabledButtonClass = "gray_btn"
	)

	// The button might be offscreen or covered by footer,
	// and uiauto.Context.MakeVisible() will not work if the node is not offscreen or
	// partially covered (overlay) by other node (in this case, the footer might cover the button).
	nextBtnFinder := crxprtNodeFinder.Name("Next").Role(role.Link).HasClass(enabledButtonClass).Focusable().Linked()
	if err := ui.DoDefault(nextBtnFinder)(ctx); err != nil {
		s.Fatal("Failed to got to next step: ", err)
	}

	startBtn := crxprtNodeFinder.Name("Start").Role(role.Link)
	if err := ui.WaitForLocation(startBtn)(ctx); err != nil {
		s.Fatal("Failed to wait for start button to be stable: ", err)
	}
	info, err := ui.Info(ctx, startBtn)
	if err != nil {
		s.Fatal("Failed to get info of start button: ", err)
	}

	// CrXPRT 2 demand the device name to be set before execution.
	if info.ClassName == disabledButtonClass {
		s.Log("Start button is grayed out; need to set up the deivce name")
		kb, err := input.Keyboard(ctx)
		if err != nil {
			s.Fatal("Failed to create the keyboard: ", err)
		}
		defer kb.Close(ctx)

		// Set the device name.
		s.Log("Typing device name")
		inputFieldFinder := crxprtNodeFinder.Role(role.TextField).Editable().Focusable()
		deviceName := s.RequiredVar("benchmark.username")
		if err := uiauto.Combine("input device name and wait for start button to change state",
			ui.DoDefaultUntil(inputFieldFinder, ui.WithTimeout(3*time.Second).WaitUntilExists(inputFieldFinder.Focused())),
			kb.TypeAction(deviceName),
			ui.WaitUntilGone(startBtn.HasClass(disabledButtonClass)),
		)(ctx); err != nil {
			s.Fatal("Failed to input device name and wait for start button to change state: ", err)
		}
	}

	// The button might be offscreen or covered by footer,
	// and uiauto.Context.MakeVisible() will not work if the node is not offscreen or
	// partially covered (overlay) by other node (in this case, the footer might cover the button).
	if err := ui.DoDefault(startBtn.HasClass(enabledButtonClass))(ctx); err != nil {
		s.Fatal("Failed to click start button to start the benchmark: ", err)
	}

	iterationsContainer := nodewith.Role(role.GenericContainer).HasClass("iterations")
	iterationsText := nodewith.NameContaining("iterations").Role(role.StaticText).Ancestor(iterationsContainer)
	if err := ui.WaitUntilExists(iterationsText)(ctx); err != nil {
		s.Fatal("Benchmark might not have started executing: ", err)
	}

	startTime := time.Now()
	s.Logf("Benchmark %s is executing", extName)

	fScore := math.NaN()
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := ui.EnsureGoneFor(iterationsText, 5*time.Second)(ctx); err != nil {
			s.Logf("Iteration text found - %s test is still running. Elapsed time: %s", extName, time.Since(startTime))
			return errors.Wrap(err, "still executing")
		}

		resultArea := crxprtNodeFinder.Role(role.GenericContainer).HasClass("result_page_result_area")
		scoreReg := regexp.MustCompile(`^\d+$`)
		scoreHeading := nodewith.NameRegex(scoreReg).Role(role.Heading).Ancestor(resultArea)
		info, err := ui.Info(ctx, scoreHeading)
		if err != nil {
			return testing.PollBreak(errors.New("failed to get the info of result node"))
		}

		strScore := strings.TrimSpace(info.Name)
		if fScore, err = strconv.ParseFloat(strScore, 64); err != nil {
			return testing.PollBreak(errors.Wrapf(err, "failed to parser the result [%s] of benchmark", strScore))
		}

		return nil
	}, &testing.PollOptions{Timeout: crxprtRunningTime, Interval: 30 * time.Second}); err != nil {
		s.Fatal("Failed to get benchmark score, error: ", err)
	}

	if err := screenshot.CaptureChrome(ctx, cr, filepath.Join(s.OutDir(), "result.png")); err != nil {
		s.Error("Failed to take screenshot: ", err)
	}

	pv := perf.NewValues()
	pv.Set(perf.Metric{
		Name:      "Benchmark.CrXPRT",
		Unit:      "score",
		Direction: perf.BiggerIsBetter,
	}, fScore)
	s.Logf("Benchmark %s final score: %f", extName, fScore)
	if err = pv.Save(s.OutDir()); err != nil {
		s.Fatal("Failed to store values: ", err)
	}
}

func launchChromeExtension(ctx context.Context, br *browser.Browser, tconn *chrome.TestConn, name, ID, URL string) error {
	isInstalled, err := ash.ChromeAppInstalled(ctx, tconn, ID)
	if err != nil {
		return errors.Wrap(err, "failed to check Chrome app existance")
	}
	if !isInstalled {
		testing.ContextLogf(ctx, "Chrome extension %s not exist, try to install", name)
		app := cws.App{Name: name, URL: URL}
		if err := cws.InstallApp(ctx, br, tconn, app); err != nil {
			return errors.Wrapf(err, "failed to install %s", name)
		}
	}
	if err := apps.Launch(ctx, tconn, ID); err != nil {
		return errors.Wrapf(err, "failed to launch %s", name)
	}
	if err := ash.WaitForApp(ctx, tconn, ID, time.Minute); err != nil {
		return errors.Wrapf(err, "failed to wait for %s to appear", name)
	}

	return nil
}
