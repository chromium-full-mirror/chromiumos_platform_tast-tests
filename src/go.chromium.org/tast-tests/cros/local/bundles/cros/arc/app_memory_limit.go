// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"regexp"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/disk"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AppMemoryLimit,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Captures set of apploading stress metrics",
		Data:         []string{"ArcAppMemoryLimit.apk"},
		Contacts: []string{
			"arc-performance@google.com",
			"khmel@chromium.org",
			"alanding@chromium.org",
		},
		// ChromeOS > Software > ARC++ > Performance
		BugComponent: "b:168382",
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{"chrome"},
		// We want to test only 4GB devices where user expirence is affected
		// due to low memory condition.
		HardwareDeps: hwdep.D(hwdep.MaxMemory(4500)),
		Timeout:      15 * time.Minute,
		Params: []testing.Param{{
			Name:              "vm",
			ExtraAttr:         []string{"crosbolt_arc_perf_qual"},
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
	})
}

func AppMemoryLimit(ctx context.Context, s *testing.State) {
	const (
		pkgName            = "org.chromium.arc.testapp.memorylimit"
		windowShownTimeout = time.Minute
		windowGoneTimeout  = 10 * time.Minute
	)

	if err := arc.AppendToArcvmDevConf(ctx, "--params=androidboot.arc.lmk.extra_protected_package_list="+pkgName); err != nil {
		s.Fatal("Failed to set protected package: ", err)
	}
	defer arc.RestoreArcvmDevConf(ctx)

	args := arc.DisableSyncFlags()
	cr, err := chrome.New(ctx,
		chrome.ARCEnabled(),
		// Enforce UnRestricted ARC CPU mode in order to have all
		// components are set as fast as possible. This should not
		// affect test results once ARC CPU is unthrottled automatically
		// when app/game is started.
		chrome.UnRestrictARCCPU(),
		chrome.ExtraArgs(args...))
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}
	defer cr.Close(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer a.Close(ctx)

	s.Log("Install app")
	if err := a.Install(ctx, s.DataPath("ArcAppMemoryLimit.apk")); err != nil {
		s.Fatal("Failed to install the APK: ", err)
	}

	s.Log("Drop host file caches")
	// Drop host caches for predictable results.
	if err := disk.DropCaches(ctx); err != nil {
		s.Fatal("Failed to drop caches: ", err)
	}

	s.Log("Start app")

	// Start activity w/ fullscreen and autostart
	if err := a.Command(ctx,
		"am", "start",
		"--windowingMode", "1",
		"-n", pkgName+"/.MainActivity",
		"--ez", "autostart", "true").Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to launch app: ", err)
	}

	s.Log("Waiting app is shown")

	if err := waitWindowShown(ctx, tconn, windowShownTimeout, pkgName); err != nil {
		s.Fatal("Failed to wait window is shown: ", err)
	}

	s.Log("Waiting app is gone (killed by low memory killer)")

	if err := waitWindowGone(ctx, tconn, windowGoneTimeout, pkgName); err != nil {
		s.Fatal("Failed to wait window is gone: ", err)
	}

	s.Log("App is gone, extracting metrics")

	perfValues := perf.NewValues()

	properties := []string{"max", "critical", "low", "moderate"}
	for _, property := range properties {
		value, err := readMemoryProp(ctx, a, property)
		if err != nil {
			s.Fatal("Failed to read property: ", property, err)
		}
		if value == 0 && property == "max" {
			s.Fatal("max property is not set")
		}
		if value != 0 {
			perfValues.Set(perf.Metric{
				Name:      property,
				Unit:      "MB",
				Direction: perf.BiggerIsBetter,
			}, float64(value))

		}
	}

	if err := perfValues.Save(s.OutDir()); err != nil {
		s.Fatal("Failed saving perf data: ", err)
	}
}

// waitWindowShown waits until test app window is shown.
func waitWindowShown(ctx context.Context, tconn *chrome.TestConn, timeout time.Duration, pkgName string) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := ash.GetARCAppWindowInfo(ctx, tconn, pkgName); err != nil {
			return errors.Wrap(err, "window not found")
		}
		return nil
	}, &testing.PollOptions{Timeout: timeout})
}

// waitWindowGone waits until test app window is gone.
func waitWindowGone(ctx context.Context, tconn *chrome.TestConn, timeout time.Duration, pkgName string) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := ash.GetARCAppWindowInfo(ctx, tconn, pkgName); err != nil {
			return nil
		}
		return errors.New("app is still shown")
	}, &testing.PollOptions{Timeout: timeout})
}

// readMemoryProp reads Android property and parses it as int.
func readMemoryProp(ctx context.Context, a *arc.ARC, name string) (int, error) {
	outStr, err := a.GetProp(ctx, "dev.arc.memorylimit."+name)
	if err != nil {
		return 0, errors.Wrap(err, "failed to read property")
	}

	resultPropRegexp := regexp.MustCompile(`^(\d+)$`)
	m := resultPropRegexp.FindStringSubmatch(outStr)
	if m == nil {
		return 0, errors.Errorf("failed to parse property %s: %q", name, outStr)
	}

	val, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, errors.Errorf("failed to parse property %s: %q", name, outStr)
	}

	return val, nil
}
