// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"path/filepath"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/diskstats"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/perfboot"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/disk"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type testParams struct {
	chromeArgs []string
}

type bootMetrics struct {
	appLaunchDuration     time.Duration
	appShownDuration      time.Duration
	enabledScreenDuration time.Duration
	appKills              *arc.AppKills
	diskStats             diskstats.DiskStatMap
}

const (
	// bootAttemptCountVarName is the name of the variable to specify the number of success iterations.
	bootAttemptCountVarName = "arc.RegularBoot.bootAttemptCount"
	// maxDailureCountVarName is the name of the variable to specify the number of allowed failed iterations.
	maxFailureCountVarName = "arc.RegularBoot.maxFailureCount"
)

var bootAttemptCountVar = testing.RegisterVarString(
	bootAttemptCountVarName,
	"5",
	"The number of iterations to try regular ARCVM boot.",
)

var maxFailureCountVar = testing.RegisterVarString(
	maxFailureCountVarName,
	"2",
	"The number of failed iterations allowed.",
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RegularBoot,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "This exercises a scenario when user logs in Chrome where ARC is already provisioned and tries to use ARC app immediately. App launch delay is reported for this case. This does not do acutual reboot however drops caches before each iteration to match the cold start scenario",
		Contacts:     []string{"arc-performance@google.com", "khmel@chromium.org"},
		// ChromeOS > Software > ARC++ > Performance
		BugComponent: "b:168382",
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{"chrome", "chrome_internal"},
		Timeout:      25 * time.Minute,
		Vars:         []string{bootAttemptCountVarName, maxFailureCountVarName},
		Params: []testing.Param{{
			ExtraAttr:         []string{"crosbolt_arc_perf_qual"},
			ExtraSoftwareDeps: []string{"android_container"},
			Val:               testParams{},
		}, {
			Name:              "vm",
			ExtraAttr:         []string{"crosbolt_arc_perf_qual"},
			ExtraSoftwareDeps: []string{"android_vm"},
			Val:               testParams{},
		}, {
			Name:              "no_guest_ureadahead_vm",
			ExtraSoftwareDeps: []string{"android_vm"},
			Val: testParams{
				chromeArgs: []string{"--arcvm-ureadahead-mode=disabled"},
			},
		}, {
			// TODO(b/301311408): Remove this, when KeyMint is fully launched and is the default.
			Name:              "keymint_vm",
			ExtraSoftwareDeps: []string{"android_vm_t"},
			Val: testParams{
				chromeArgs: []string{"--enable-features=ArcSwitchToKeyMintOnT,ArcSwitchToKeyMintOnTOverride"},
			},
		}},
		VarDeps: []string{
			"arc.perfAccountPool",
		},
	})
}

// RegularBoot steps through multiple ARC boots.
func RegularBoot(ctx context.Context, s *testing.State) {
	// Disable multicast to make sure CPU can be stabilized for capturing performance metrics
	cleanup, err := setup.DisableMulticastSetup(ctx)
	if err != nil {
		s.Fatal("Could not disable multicast: ", err)
	}
	defer cleanup(ctx)

	params := s.Param().(testParams)
	creds, err := performArcInitialBoot(ctx, s.RequiredVar("arc.perfAccountPool"), params.chromeArgs)
	if err != nil {
		s.Fatal("Failed to do initial optin: ", err)
	}

	iterationCount, err := strconv.Atoi(bootAttemptCountVar.Value())
	if err != nil {
		s.Fatalf("Invalid %v value: %v", bootAttemptCountVarName, bootAttemptCountVar.Value())
	}
	maxFailureCount, err := strconv.Atoi(maxFailureCountVar.Value())
	if err != nil {
		s.Fatalf("Invalid %v value: %v", maxFailureCountVarName, maxFailureCountVar.Value())
	}

	successCount := 0
	failureCount := 0
	perfValues := perf.NewValues()
	for successCount < iterationCount {
		s.Logf("Running ARC regular boot iteration #%d out of %d",
			successCount+1, iterationCount)
		bootMetrics, err := performArcRegularBoot(ctx, s.OutDir(), creds, params.chromeArgs, successCount, failureCount)
		if err != nil {
			failureCount++
			if failureCount > maxFailureCount {
				s.Fatal("Too many failures for regular boot. Last one: ", err)
			}
			testing.ContextLogf(ctx, "Reguar boot failed: %q. will retry", err)
			continue
		}

		successCount++
		perfValues.Append(perf.Metric{
			Name:      "app_launch_time",
			Unit:      "seconds",
			Direction: perf.SmallerIsBetter,
			Multiple:  true,
		}, bootMetrics.appLaunchDuration.Seconds())
		perfValues.Append(perf.Metric{
			Name:      "app_shown_time",
			Unit:      "seconds",
			Direction: perf.SmallerIsBetter,
			Multiple:  true,
		}, bootMetrics.appShownDuration.Seconds())
		perfValues.Append(perf.Metric{
			Name:      "boot_progress_enable_screen",
			Unit:      "seconds",
			Direction: perf.SmallerIsBetter,
			Multiple:  true,
		}, bootMetrics.enabledScreenDuration.Seconds())
		if bootMetrics.appKills != nil {
			bootMetrics.appKills.AppendPerfMetrics(perfValues, "")
		}
		bootMetrics.diskStats.AppendPerfMetrics(perfValues, "")
	}

	if err := perfValues.Save(s.OutDir()); err != nil {
		s.Fatal("Failed saving perf data: ", err)
	}
}

// performArcInitialBoot performs initial boot that includes ARC provisioning and returns GAIA
// credentials to use for regular boot wih preserved state.
func performArcInitialBoot(ctx context.Context, credPool string, chromeArgs []string) (chrome.Creds, error) {
	// Disable ArcWindowPredictor to let chrome record the necessary histograms.
	// TODO(b/259517082): Stop disabling ArcWindowPredictor.
	chromeArgs = append(chromeArgs, "--disable-features=ArcWindowPredictor")
	chromeArgs = append(chromeArgs, arc.DisableSyncFlags()...)

	// Options are tuned for the fastest boot, we don't care about
	// initial provisioning performance, which is monitored in other tests.
	opts := []chrome.Option{
		chrome.ARCSupported(),
		chrome.GAIALoginPool(credPool),
		chrome.ExtraArgs(chromeArgs...)}

	testing.ContextLog(ctx, "Create initial Chrome")
	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		return chrome.Creds{}, errors.Wrap(err, "failed to connect to Chrome")
	}
	defer cr.Close(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return chrome.Creds{}, errors.Wrap(err, "failed to create test connection")
	}

	testing.ContextLog(ctx, "ARC is not enabled, perform optin")
	if err := optin.Perform(ctx, cr, tconn); err != nil {
		return chrome.Creds{}, errors.Wrap(err, "failed to optin")
	}

	if err := optin.WaitForPlayStoreShown(ctx, tconn, 2*time.Minute); err != nil {
		return chrome.Creds{}, errors.Wrap(err, "failed to wait Play Store shown")
	}

	return cr.Creds(), nil
}

// performArcRegularBoot performs ARC boot and starts Play Store app deferred and waits it is
// actually shown. It returns:
//   - time between the user session is created and and Play Store window is shown.
//   - time to fully start Android system server. This is included into the metric above.
//
// Note, it is not actually possible to measure this time directly from test due to tast
// login is complex and ends after user session is actually created. Instead it uses existing ARC
// histogram first app launch request and delay. Combined value is actual time that representss
// the hardest case when user stars ARC app immideatly after login. First app launch request
// represents here the overhead from tast Chrome login implementation.
// This also resets system caches before login to simulate scenario when user uses Chromebook after
// reboot.
func performArcRegularBoot(ctx context.Context, testDir string, creds chrome.Creds, chromeArgs []string, successCount, failureCount int) (retResult *bootMetrics, retErr error) {
	// Use custom cooling config that is bit relaxed from default implementation
	// in order to reduce failure rate especially on AMD low-end devices.
	coolDownConfig := cpu.CoolDownConfig{
		PollTimeout:          300 * time.Second,
		PollInterval:         2 * time.Second,
		TemperatureThreshold: 52000,
		CoolDownMode:         cpu.CoolDownStopUI,
	}

	var result bootMetrics

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if _, err := cpu.WaitUntilCoolDown(ctx, coolDownConfig); err != nil {
		return &result, errors.Wrap(err, "failed to wait until CPU is cooled down")
	}

	// Drop caches to simulate cold start when data not in system caches already.
	if err := disk.DropCaches(ctx); err != nil {
		return &result, errors.Wrap(err, "failed to drop caches")
	}

	opts := []chrome.Option{
		chrome.ARCSupported(),
		chrome.GAIALogin(creds),
		chrome.KeepState(),
		chrome.ExtraArgs(chromeArgs...),
		chrome.ExtraArgs(append(arc.DisableSyncFlags(),
			// Disable ArcWindowPredictor to let chrome record the necessary histograms.
			// TODO(b/259517082): Stop disabling ArcWindowPredictor.
			"--disable-features=ArcWindowPredictor")...)}

	testing.ContextLog(ctx, "Create Chrome")
	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		return &result, errors.Wrap(err, "failed to connect to Chrome")
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return &result, errors.Wrap(err, "failed to create test connection")
	}

	testing.ContextLog(ctx, "Starting Play Store window deferred")
	if err := apps.Launch(ctx, tconn, apps.PlayStore.ID); err != nil {
		return &result, errors.Wrap(err, "failed to launch Play Store")
	}

	if err := optin.WaitForPlayStoreShown(ctx, tconn, 2*time.Minute); err != nil {
		return &result, errors.Wrap(err, "failed to wait Play Store shown")
	}

	// Collect disk stats immediately after Play Store is shown to reduce the
	// impact of further disk access.
	result.diskStats, err = diskstats.CollectDiskStats(ctx)
	if err != nil {
		return &result, errors.Wrap(err, "failed to read disk stats")
	}

	a, err := arc.New(ctx, testDir, cr.NormalizedUser())
	if err != nil {
		return &result, errors.Wrap(err, "failed to connect to ARC")
	}
	defer a.Close(cleanupCtx)

	defer func() {
		var logcatFilePath string
		if retErr == nil {
			logcatFilePath = filepath.Join(testDir, "logcat_success_"+strconv.Itoa(successCount+1)+".txt")
		} else {
			logcatFilePath = filepath.Join(testDir, "logcat_failure_"+strconv.Itoa(failureCount+1)+".txt")
		}
		if err := a.DumpLogcat(ctx, logcatFilePath); err != nil {
			testing.ContextLogf(ctx, "Failed to save logcat output to %v: %v", logcatFilePath, err)
		}
	}()

	delay, err := readFirstAppLaunchHistogram(ctx, tconn, "Arc.FirstAppLaunchDelay.TimeDelta")
	if err != nil {
		return &result, err
	}

	request, err := readFirstAppLaunchHistogram(ctx, tconn, "Arc.FirstAppLaunchRequest.TimeDelta")
	if err != nil {
		return &result, err
	}

	delayShown, err := readFirstAppLaunchHistogram(ctx, tconn, "Arc.FirstAppLaunchDelay.TimeDeltaUntilAppLaunch")
	if err != nil {
		return &result, err
	}

	p, err := perfboot.GetPerfValues(ctx, tconn, a)
	if err != nil {
		return &result, errors.Wrap(err, "failed to extract ARC boot metrics")
	}
	result.appLaunchDuration = request + delay
	result.appShownDuration = request + delayShown
	result.enabledScreenDuration = p["boot_progress_enable_screen"]

	result.appKills, err = arc.GetAppKills(ctx, tconn)
	if err != nil {
		testing.ContextLog(ctx, "Failed to collect ARC app kill counts: ", err)
	}

	out, err := a.Command(ctx, "getprop", "dev.arc.accountsignin.result").Output(testexec.DumpLogOnError)
	if err != nil {
		return &result, errors.Wrap(err, "failed to check signin status")
	}

	outStr := string(out)
	// TODO(b/): Use histogram account check instead once ready"
	if outStr != "OK,0\n" && outStr != "\n" {
		return &result, errors.Errorf("Detected re-signin (b/297139280) with result %s", outStr)
	}

	return &result, nil
}

// readFirstAppLaunchHistogram reads histogram and converts it to Duration.
func readFirstAppLaunchHistogram(ctx context.Context, tconn *chrome.TestConn, name string) (time.Duration, error) {
	metric, err := metrics.WaitForHistogram(ctx, tconn, name, 20*time.Second)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to get %s histogram", name)
	}

	timeMs, err := metric.Mean()
	if err != nil {
		return 0, errors.Wrapf(err, "failed to read %s histogram", name)
	}

	return time.Duration(timeMs * float64(time.Millisecond)), nil
}
