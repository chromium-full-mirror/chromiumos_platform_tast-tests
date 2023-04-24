// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package benchmark

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"chromiumos/tast/common/android/adb"
	"chromiumos/tast/common/android/ui"
	"chromiumos/tast/common/perf"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/arc"
	"chromiumos/tast/local/bundles/cros/benchmark/setup"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/screenshot"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

const (
	apkName            = "com_futuremark_pcmark.apk"
	pcmarkPkgName      = "com.futuremark.pcmark.android.benchmark"
	pcmarkAppName      = "PCMark"
	pcmarkActivityName = "com.futuremark.gypsum.activity.SplashPageActivity"
	pcmarkResultLabel  = "Work 3.0 performance score "
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PCMarkWorkAndroidApp,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Execute PCMark Android App v3.0.4061 to do benchmark for PCMark Work and acquire test score",
		Contacts:     []string{"chromeos-perf-reliability-eng@google.com", "cienet-development@googlegroups.com", "xibin@google.com"},
		BugComponent: "b:1025042", // ChromeOS > EngProd > Platform > SPERA > Automation
		// Purposely leave the empty Attr here. Public benchmark tests are not included in crosbolt group for now.
		Attr:         []string{"group:crosbolt", "crosbolt_weekly"},
		SoftwareDeps: []string{"arc", "chrome"},
		HardwareDeps: hwdep.D(
			hwdep.InternalDisplay(),
			// Since the public benchmark will publish data online, run it only on certain approved models.
			setup.PublicBenchmarkAllowed(),
		),
		Timeout: 45 * time.Minute,
		Fixture: "arcBooted",
		Data:    []string{apkName},
	})
}

// PCMarkWorkAndroidApp executes PCMark Android App to do benchmark for PCMark Work and acquire test score.
func PCMarkWorkAndroidApp(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*arc.PreData).Chrome
	a := s.FixtValue().(*arc.PreData).ARC

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	uiDevice := s.FixtValue().(*arc.PreData).UIDevice

	s.Log("Installing with InstallOptionGrantPermissions: ", apkName)
	if err := a.Install(ctx, s.DataPath(apkName), adb.InstallOptionGrantPermissions); err != nil {
		s.Fatal("Failed to install app: ", err)
	}

	s.Log("Launching PCMark app")
	if err := launchPCMark(ctx, tconn, uiDevice, a, pcmarkPkgName, pcmarkActivityName); err != nil {
		s.Fatal("Failed to launch PCMark: ", err)
	}

	btnRun, err := installPCMarkComponents(ctx, uiDevice)
	if err != nil {
		s.Fatal("Failed to install PCMark components and get ready to run: ", err)
	}

	// Run PCMark benchmark test and wait for results.
	resultObj := uiDevice.Object(ui.TextContains(pcmarkResultLabel))
	if err := runPCMark(ctx, uiDevice, btnRun, resultObj); err != nil {
		s.Fatal("Failed to run PCMark: ", err)
	}

	// Take result screenshot which can be checked manually if needed.
	if err := screenshot.CaptureChrome(ctx, cr, filepath.Join(s.OutDir(), "result.png")); err != nil {
		testing.ContextLog(ctx, "Failed to take screenshot: ", err)
	}

	fScore, err := collectTestResult(ctx, cr, resultObj)
	if err != nil {
		s.Fatal("Failed to collect PCMark test result: ", err)
	}

	pv := perf.NewValues()
	pv.Set(perf.Metric{
		Name:      "Benchmark.PCMark",
		Unit:      "score",
		Direction: perf.BiggerIsBetter,
	}, float64(fScore))

	if err = pv.Save(s.OutDir()); err != nil {
		s.Fatal("Failed to save performance values: ", err)
	}
}

func launchPCMark(ctx context.Context, tconn *chrome.TestConn, device *ui.Device, ar *arc.ARC, pkg, activity string) error {
	act, err := arc.NewActivity(ar, pkg, activity)
	if err != nil {
		return errors.Wrap(err, "failed to create new activity")
	}
	defer act.Close(ctx)

	if err = act.StartWithDefaultOptions(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to start app")
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return ash.WaitForVisible(ctx, tconn, pkg)
	}, &testing.PollOptions{Timeout: 5 * time.Minute}); err != nil {
		return errors.Wrap(err, "failed to wait for the PCMark APP window")
	}

	if err := setup.DismissMobilePrompt(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to dismiss 'designed for mobile' prompt")
	}

	// Click the "ACCEPT" button if it shows up.
	accept := device.Object(ui.TextContains("ACCEPT"))
	if err := clickUIObjIfExists(ctx, accept); err != nil {
		return errors.Wrap(err, "failed to dismiss 'ACCEPT' prompt")
	}

	testing.ContextLog(ctx, "Wait for home page properly rendered")
	obj := device.Object(ui.TextStartsWith("Benchmark performance and battery life with tests based on everyday activities."))
	if err := obj.WaitForExists(ctx, 2*time.Minute); err != nil {
		return errors.Wrap(err, "failed to continue in PCMark APP")
	}

	return nil
}

// installPCMarkComponents installs the PCMark's in-app software component,
// and returns the "RUN" button object.
func installPCMarkComponents(ctx context.Context, uiDevice *ui.Device) (*ui.Object, error) {
	// PCMark needs in-app installation before it can show the RUN button.
	// Check if RUN or "INSTALL" button is available.
	var btnRun, btnInstall *ui.Object
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if btnRun = buttonWithText(ctx, uiDevice, "RUN"); btnRun != nil {
			return nil
		}
		if btnInstall = buttonWithRegex(ctx, uiDevice, `INSTALL\(.*\)`); btnInstall != nil {
			return nil
		}
		return errors.New("Neither RUN nor INSTALL button is available")
	}, &testing.PollOptions{Timeout: 1 * time.Minute, Interval: 1 * time.Second}); err != nil {
		return nil, err
	}
	if btnRun == nil && btnInstall != nil {
		testing.ContextLog(ctx, "Installing PCMark component")
		if err := btnInstall.Click(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to intsall PCMark component")
		}
		btnConfirm := buttonWithText(ctx, uiDevice, "INSTALL")
		if btnConfirm == nil {
			return nil, errors.New("failed to find install confirm button")
		}
		if err := btnConfirm.Click(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to confirm PCMark component installation")
		}

		if err := testing.Poll(ctx, func(context.Context) error {
			// RUN button will shown after the installation has finished.
			if btnRun = buttonWithText(ctx, uiDevice, "RUN"); btnRun == nil {
				return errors.New("component installation has not completed yet")
			}
			return nil
		}, &testing.PollOptions{Timeout: 5 * time.Minute, Interval: 1 * time.Second}); err != nil {
			return nil, errors.Wrap(err, "failed to wait for component installation to complete")
		}
	}
	return btnRun, nil
}

func runPCMark(ctx context.Context, uiDevice *ui.Device, btnRun, resultObj *ui.Object) error {
	testing.ContextLog(ctx, "Executing benchmark")
	if err := btnRun.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to click RUN button to start benchmark test")
	}

	startTime := time.Now()
	lastLogTime := startTime
	if err := testing.Poll(ctx, func(context.Context) error {
		if err := resultObj.WaitForExists(ctx, 20*time.Millisecond); err != nil {
			currentTime := time.Now()
			if currentTime.Sub(lastLogTime) > 30*time.Second {
				// Print log every 30 seconds.
				lastLogTime = currentTime
				testing.ContextLogf(ctx, "Result label not found - PCMark test is still running. Elapsed time: %s", currentTime.Sub(startTime))
			}
			return errors.Wrap(err, "result label not found")
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Minute, Interval: time.Second}); err != nil {
		return errors.Wrap(err, "failed to wait for benchmark to finish its execution")
	}

	return nil
}

func collectTestResult(ctx context.Context, cr *chrome.Chrome, resultObj *ui.Object) (float64, error) {
	resultText, err := resultObj.GetText(ctx)
	if err != nil {
		return 0.0, errors.Wrap(err, "failed to get result text")
	}

	// Get the score from the test result text.
	strScore := strings.TrimSpace(resultText[len(pcmarkResultLabel):])
	testing.ContextLogf(ctx, "Benchmark score: [%s]", strScore)

	fScore, err := strconv.ParseFloat(strScore, 64)
	if err != nil {
		return 0.0, errors.Wrapf(err, "failed to parser score string %q", strScore)
	}
	return fScore, nil
}

func findUIObj(ctx context.Context, obj *ui.Object) bool {
	return obj.WaitForExists(ctx, 10*time.Second) == nil
}

func clickUIObj(ctx context.Context, obj *ui.Object) error {
	if found := findUIObj(ctx, obj); !found {
		return errors.New("failed to find ui object")
	}
	if err := obj.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to click ui object")
	}
	return nil
}

func clickUIObjIfExists(ctx context.Context, obj *ui.Object) error {
	if found := findUIObj(ctx, obj); !found {
		return nil
	}
	return obj.Click(ctx)
}

// buttonWithText finds button with certain text.
// It is found that PCMark UI can change accross different runs - the text string might be in
// "field: text" or "field: content-description".
// Here use multiple ui.SelectorOption to find a ui object.
func buttonWithText(ctx context.Context, d *ui.Device, text string) (obj *ui.Object) {
	obj = d.Object(ui.Text(text))
	if findUIObj(ctx, obj) {
		return obj
	}
	obj = d.Object(ui.DescriptionContains(text))
	if findUIObj(ctx, obj) {
		return obj
	}
	return nil
}

// buttonWithRegex finds button with text matching the given regex string.
// It is found that PCMark UI can change accross different runs - the text string might be in
// "field: text" or "field: content-description".
// Here use multiple ui.SelectorOption to find a ui object.
func buttonWithRegex(ctx context.Context, d *ui.Device, reg string) (obj *ui.Object) {
	obj = d.Object(ui.TextMatches(reg))
	if findUIObj(ctx, obj) {
		return obj
	}
	obj = d.Object(ui.DescriptionMatches(reg))
	if findUIObj(ctx, obj) {
		return obj
	}
	return nil
}
