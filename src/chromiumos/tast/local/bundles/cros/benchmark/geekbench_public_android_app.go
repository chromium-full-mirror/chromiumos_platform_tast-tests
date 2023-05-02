// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package benchmark

import (
	"context"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	androidui "chromiumos/tast/common/android/ui"
	"chromiumos/tast/common/perf"
	"chromiumos/tast/local/apps"
	"chromiumos/tast/local/arc"
	"chromiumos/tast/local/arc/playstore"
	"chromiumos/tast/local/bundles/cros/benchmark/setup"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/cuj"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	shortUITimeout       = 5 * time.Second
	defaultUITimeout     = 15 * time.Second
	benchmarkTestTimeout = 30 * time.Minute

	geekbenchPkgName = "com.primatelabs.geekbench5"
	activityName     = "com.primatelabs.geekbench.HomeActivity"
	buttonClassName  = "android.widget.Button"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         GeekbenchPublicAndroidApp,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Execute Geekbench public Android App to do benchmark testing and retrieve the results",
		Contacts:     []string{"chromeos-perf-reliability-eng@google.com", "cienet-development@googlegroups.com", "xibin@google.com"},
		BugComponent: "b:1025042", // ChromeOS > EngProd > Platform > SPERA > Automation
		// Purposely leave the empty Attr here. Public benchmark tests are not included in crosbolt group for now.
		Attr:         []string{},
		SoftwareDeps: []string{"chrome", "arc"},
		HardwareDeps: hwdep.D(
			hwdep.InternalDisplay(),
			// Since the public benchmark will publish data online, run it only on certain approved models.
			setup.PublicBenchmarkAllowed(),
		),
		Timeout: benchmarkTestTimeout + 5*time.Minute,
		Fixture: setup.BenchmarkARCFixture,
	})
}

func GeekbenchPublicAndroidApp(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*arc.PreData).Chrome
	a := s.FixtValue().(*arc.PreData).ARC

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	defer func(ctx context.Context) {
		faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_dump")
		a.DumpUIHierarchyOnError(ctx, filepath.Join(s.OutDir(), "arc"), s.HasError)
	}(cleanupCtx)

	device, err := a.NewUIDevice(ctx)
	if err != nil {
		s.Fatal("Failed to setup ARC device: ", err)
	}
	defer device.Close(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API connection: ", err)
	}

	if err := installGeekbench(ctx, tconn, device, a); err != nil {
		s.Fatal("Failed to install Geekbench: ", err)
	}

	if err := openGeekbench(ctx, tconn, device, a); err != nil {
		s.Fatal("Failed to launch Geekbench: ", err)
	}

	const (
		runCPUBenchmark  = "RUN CPU BENCHMARK"
		benchmarkResults = "Benchmark Results"
	)

	startTime := time.Now() // Geekbench test start time.
	runCPUBenchmarkButton := device.Object(androidui.Text(runCPUBenchmark), androidui.ClassName(buttonClassName))
	if err := cuj.FindAndClick(runCPUBenchmarkButton, defaultUITimeout)(ctx); err != nil {
		s.Fatalf("Failed to click %q: %v", runCPUBenchmark, err)
	}

	// Wait for the Geekbench to produce test result.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		resultLabel := device.Object(androidui.TextContains(benchmarkResults))
		if err := resultLabel.WaitForExists(ctx, time.Second); err != nil {
			s.Logf("Result label not found - Geekbench test is still running. Elapsed time: %s", time.Since(startTime))
			return errors.Wrap(err, "failed to find benchmark result label")
		}
		return nil
	}, &testing.PollOptions{Timeout: benchmarkTestTimeout}); err != nil {
		s.Fatal("Failed to run Geekbench: ", err)
	}

	// If the "Open with" popup appears, select open Geekbench browser with Chrome.
	if err := openGeekbenchBrowserWithChrome(device)(ctx); err != nil {
		s.Fatal("Failed to open Geekbench Browser with Chrome: ", err)
	}

	if err := readAndSaveResult(ctx, tconn, s.OutDir()); err != nil {
		s.Fatal("Failed to read and save result: ", err)
	}
}

func openGeekbench(ctx context.Context, tconn *chrome.TestConn, device *androidui.Device, ar *arc.ARC) error {
	act, err := arc.NewActivity(ar, geekbenchPkgName, activityName)
	if err != nil {
		return errors.Wrap(err, "failed to create new activity")
	}
	defer act.Close(ctx)

	if err = act.StartWithDefaultOptions(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to start Geekbench")
	}

	ui := uiauto.New(tconn)
	gotItButton := nodewith.Name("Got it").Role(role.Button)
	if err := uiauto.IfSuccessThen(
		ui.WithTimeout(shortUITimeout).WaitUntilExists(gotItButton),
		ui.LeftClick(gotItButton),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to find button Got it and click it")
	}

	if err := ash.WaitForCondition(ctx, tconn, func(w *ash.Window) bool {
		return w.IsActive && w.ARCPackageName == geekbenchPkgName
	}, &testing.PollOptions{Timeout: time.Minute}); err != nil {
		return errors.Wrap(err, "failed to wait for the Geekbench APP window")
	}

	if err := setup.DismissMobilePrompt(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to dismiss 'designed for mobile' prompt")
	}

	acceptButton := device.Object(androidui.TextContains("ACCEPT"))
	if err := cuj.ClickIfExist(acceptButton, defaultUITimeout)(ctx); err != nil {
		return errors.Wrap(err, "failed to find button ACCEPT and click it")
	}

	return nil
}

func installGeekbench(ctx context.Context, tconn *chrome.TestConn, device *androidui.Device, ar *arc.ARC) error {
	if err := apps.Launch(ctx, tconn, apps.PlayStore.ID); err != nil {
		return errors.Wrap(err, "failed to launch Play Store")
	}

	// Ignore APP close error because it doesn't impact the test logic.
	defer apps.Close(ctx, tconn, apps.PlayStore.ID)

	if err := playstore.InstallApp(ctx, ar, device, geekbenchPkgName, &playstore.Options{TryLimit: -1}); err != nil {
		return errors.Wrapf(err, "failed to install %s", geekbenchPkgName)
	}

	return nil
}

func openGeekbenchBrowserWithChrome(device *androidui.Device) uiauto.Action {
	chromeButton := device.Object(androidui.TextContains("Chrome"), androidui.ClassName(buttonClassName))
	alwaysButton := device.Object(androidui.TextContains("Always"), androidui.ClassName(buttonClassName))
	return uiauto.NamedCombine("open with Chrome browser",
		cuj.ClickIfExist(chromeButton, shortUITimeout),
		cuj.ClickIfExist(alwaysButton, shortUITimeout),
	)
}

func readAndSaveResult(ctx context.Context, tconn *chrome.TestConn, outDir string) error {
	ui := uiauto.New(tconn)

	geekbenchRootWebArea := nodewith.NameContaining("Geekbench Browser").Role(role.RootWebArea)
	cpuScoreTable := nodewith.HasClass("cpu").Ancestor(geekbenchRootWebArea)
	scoreReg := regexp.MustCompile(`^(\d+)$`)
	scoreNode := nodewith.NameRegex(scoreReg).Role(role.StaticText).Ancestor(cpuScoreTable)
	nodes, err := ui.NodesInfo(ctx, scoreNode)
	if err != nil {
		return errors.Wrap(err, "failed to find score")
	}
	if len(nodes) != 2 {
		return errors.Errorf("unexpected number of score nodes: got %d; want 2", len(nodes))
	}

	singleCoreScore, err := strconv.ParseFloat(nodes[0].Name, 64)
	if err != nil {
		return errors.Wrapf(err, "failed to parse Geekbench single core score; got: %s", nodes[0].Name)
	}
	multiCoreScore, err := strconv.ParseFloat(nodes[1].Name, 64)
	if err != nil {
		return errors.Wrapf(err, "failed to parse Geekbench multi core score; got: %s", nodes[1].Name)
	}

	pv := perf.NewValues()

	pv.Set(perf.Metric{
		Name:      "Benchmark.GeekBench.SingleCore",
		Unit:      "score",
		Direction: perf.BiggerIsBetter,
	}, singleCoreScore)

	pv.Set(perf.Metric{
		Name:      "Benchmark.GeekBench.MultiCore",
		Unit:      "score",
		Direction: perf.BiggerIsBetter,
	}, multiCoreScore)

	if err := pv.Save(outDir); err != nil {
		return errors.Wrap(err, "failed to store performance values")
	}
	return nil
}
