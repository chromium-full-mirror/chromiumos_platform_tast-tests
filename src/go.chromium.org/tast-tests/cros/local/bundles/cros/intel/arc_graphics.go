// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/playstore"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/resourced"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ARCGraphics,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test ARC++ graphics performance",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291",
		Attr:         []string{"group:intel-nda"},
		SoftwareDeps: []string{"android_p", "chrome"},
		Timeout:      chrome.GAIALoginTimeout + arc.BootTimeout + 10*time.Minute,
		Fixture:      "arcBootedWithPlayStore",
		VarDeps:      []string{"ui.gaiaPoolDefault"},
	})
}

func ARCGraphics(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*arc.PreData).Chrome
	a := s.FixtValue().(*arc.PreData).ARC
	d := s.FixtValue().(*arc.PreData).UIDevice

	const (
		pkgName  = "com.robtopx.geometrydashmeltdown"
		appName  = "Geometry Dash Meltdown"
		webGLURL = "https://storage.googleapis.com/chromiumos-test-assets-public/power_ThermalLoad/aquarium/aquarium.html?numFish=5000"
	)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	pv := perf.NewValues()
	defer func() {
		if err := pv.Save(s.OutDir()); err != nil {
			s.Error("Failed to save perf data: ", err)
		}
	}()

	initialMeasurement, err := cpu.MeasureUsage(ctx, 5*time.Second)
	if err != nil {
		s.Fatal("Failed to get initial CPU consumption: ", err)
	}
	initialAvailableMemory, err := availableMemory(ctx)
	if err != nil {
		s.Fatal("Failed to measure initial available memory: ", err)
	}
	pv.Set(perf.Metric{
		Name: "initial_CPU_usage", Unit: "percent", Direction: perf.SmallerIsBetter,
	}, initialMeasurement)
	pv.Set(perf.Metric{
		Name: "initial_available_memory", Unit: "bytes", Direction: perf.BiggerIsBetter,
	}, float64(initialAvailableMemory))

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	if err := playstore.InstallApp(ctx, a, d, pkgName, &playstore.Options{}); err != nil {
		s.Fatal("Failed to install ARC++ Geometry Dash Meltdown app: ", err)
	}

	// Setup keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)

	ui := uiauto.New(tconn)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	// Search for and launch the app.
	if err := uiauto.Combine("search for Geometry Dash Meltdown in launcher",
		launcher.SearchAndLaunchWithQuery(tconn, kb, appName, appName),
		ui.WaitUntilExists(nodewith.Name(appName).HasClass("Widget")),
	)(ctx); err != nil {
		s.Fatal("Failed to search and launch: ", err)
	}

	// Open WebGL page.
	conn, err := cr.NewConn(ctx, webGLURL)
	if err != nil {
		s.Fatal("Failed to create new Chrome connection: ", err)
	}
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

	finalMeasurement, err := cpu.MeasureUsage(ctx, 5*time.Second)
	if err != nil {
		s.Fatal("Failed to get final CPU consumption: ", err)
	}
	finalAvailableMemory, err := availableMemory(ctx)
	if err != nil {
		s.Fatal("Failed to measure final available memory: ", err)
	}
	pv.Set(perf.Metric{
		Name: "final_CPU_usage", Unit: "percent", Direction: perf.SmallerIsBetter,
	}, finalMeasurement)
	pv.Set(perf.Metric{
		Name: "final_available_memory", Unit: "bytes", Direction: perf.BiggerIsBetter,
	}, float64(finalAvailableMemory))

	if finalAvailableMemory > initialAvailableMemory {
		s.Error("Failed, final memory usage is less than initial")
	}
	if finalMeasurement < initialMeasurement {
		s.Error("Failed, final CPU usage is less than initial")
	}
}

// availableMemory returns the available memory using D-Bus method.
func availableMemory(ctx context.Context) (uint64, error) {
	rm, err := resourced.NewClient(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "failed to create Resource Manager client")
	}
	available, err := rm.AvailableMemoryKB(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "failed to get ChromeOS available memory")
	}
	return available, nil
}
