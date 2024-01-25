// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"context"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	rmpb "go.chromium.org/chromiumos/system_api/resource_manager_proto"

	"go.chromium.org/tast-tests/cros/local/memory/kernelmeter"
	"go.chromium.org/tast-tests/cros/local/resourced"
	"go.chromium.org/tast-tests/cros/local/sched"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type resourcedTestParams struct {
	isBaseline bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         Resourced,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that resourced works",
		Contacts:     []string{"chromeos-memory@google.com", "vovoy@chromium.org"},
		BugComponent: "b:167286", // ChromeOS > Platform > System > Memory Management
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      3 * time.Minute,
		Params: []testing.Param{{
			ExtraAttr: []string{"informational"},
			Val: resourcedTestParams{
				isBaseline: false,
			},
		}, {
			Name: "baseline",
			Val: resourcedTestParams{
				isBaseline: true,
			},
		}},
	})
}

// checkSetGameMode tests SetGameMode functionality and also the tunings that come along with the change of game mode if specified.
// We will check the tuning of swappiness along with the change of game mode if checkSwappinessTuning is true;
// and we will check the tuning transparent huge pages along with the change of game mode if checkTHPTuning is true.
func checkSetGameMode(ctx context.Context, rm *resourced.Client, checkSwappinessTuning, checkTHPTuning bool) (resErr error) {
	// Get the original game mode.
	origGameMode, err := rm.GameMode(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to query game mode state")
	}
	testing.ContextLog(ctx, "Original game mode: ", origGameMode)

	defer func() {
		// Restore game mode.
		if err = rm.SetGameMode(ctx, origGameMode); err != nil {
			if resErr == nil {
				resErr = errors.Wrap(err, "failed to reset game mode state")
			} else {
				testing.ContextLog(ctx, "Failed to reset game mode state: ", err)
			}
		}
	}()

	// Set game mode to different value.
	var newGameMode uint8
	if origGameMode == 0 {
		newGameMode = 1
	}
	if err = rm.SetGameMode(ctx, newGameMode); err != nil {
		return errors.Wrap(err, "failed to set game mode state")
	}
	testing.ContextLog(ctx, "Set game mode: ", newGameMode)

	if checkSwappinessTuning {
		if err := validateSwappiness(ctx, newGameMode); err != nil {
			return errors.Wrap(err, "validate swappiness failed")
		}
	}

	if checkTHPTuning {
		if err := validateTHP(ctx, newGameMode); err != nil {
			return errors.Wrap(err, "validate THP failed")
		}
	}

	// Check game mode is set to the new value.
	gameMode, err := rm.GameMode(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to query game mode state")
	}
	if newGameMode != gameMode {
		return errors.Errorf("set game mode to: %d, but got game mode: %d", newGameMode, gameMode)
	}
	return nil
}

// checkSetGameModeWithTimeout tests SetGamSetGameModeWithTimeout functionality and also the tunings that come along with the change of game mode if specified.
// We will check the tuning of swappiness along with the change of game mode if checkSwappinessTuning is true;
// and we will check the tuning of transparent huge pages along with the change of game mode if checkTHPTuning is true.
func checkSetGameModeWithTimeout(ctx context.Context, rm *resourced.Client, checkSwappinessTuning, checkTHPTuning bool) (resErr error) {
	var newGameMode uint8 = resourced.GameModeBorealis
	if err := rm.SetGameModeWithTimeout(ctx, newGameMode, 1); err != nil {
		return errors.Wrap(err, "failed to set game mode state")
	}
	testing.ContextLog(ctx, "Set game mode with 1 second timeout: ", newGameMode)
	if checkSwappinessTuning {
		if err := validateSwappiness(ctx, newGameMode); err != nil {
			return errors.Wrap(err, "validate swappiness failed")
		}
	}

	if checkTHPTuning {
		if err := validateTHP(ctx, newGameMode); err != nil {
			return errors.Wrap(err, "validate THP failed")
		}
	}

	// Check game mode is set to the new value.
	gameMode, err := rm.GameMode(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to query game mode state")
	}
	if newGameMode != gameMode {
		return errors.Errorf("set game mode to: %d, but got game mode: %d", newGameMode, gameMode)
	}

	// Check game mode is reset after timeout.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		gameMode, err := rm.GameMode(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to query game mode state")
		}
		if gameMode != resourced.GameModeOff {
			return errors.New("game mode is not reset")
		}
		return nil
	}, &testing.PollOptions{Timeout: 2 * time.Second, Interval: 100 * time.Millisecond}); err != nil {
		return errors.Wrap(err, "failed to wait for game mode reset")
	}

	// Check swappiness is reset after timeout.
	if checkSwappinessTuning {
		if err := validateSwappiness(ctx, resourced.GameModeOff); err != nil {
			return errors.Wrap(err, "Reset swapiness failed")
		}
	}

	if checkTHPTuning {
		if err := validateTHP(ctx, resourced.GameModeOff); err != nil {
			return errors.Wrap(err, "Reset THP failed")
		}
	}
	return nil
}

func readSwappiness(ctx context.Context) (int, error) {
	fileBytes, err := ioutil.ReadFile("/proc/sys/vm/swappiness")

	if err != nil {
		return 0, errors.Wrap(err, "failed to read swappiness")
	}
	swappinessVal, errConv := strconv.Atoi(strings.TrimSuffix(string(fileBytes), "\n"))
	if errConv != nil {
		return 0, errors.Wrap(errConv, "failed to parse swappiness to int")
	}
	return swappinessVal, nil
}

func readTHP(ctx context.Context, thpFile string) (string, error) {
	thp, err := ioutil.ReadFile(thpFile)
	if err != nil {
		return "", err
	}
	// The thp is of format like `[always] madvise never`, with the value
	// inside [] as the mode that's currently used.
	re := regexp.MustCompile(`.*\[(.+)\].*`)
	match := re.FindStringSubmatch(string(thp))
	return match[1], nil
}

// validateSwappiness checks swappiness is tuned correctly:
//  1. for borealis game, tuned to 30;
//  2. for others, not tuned.
func validateSwappiness(ctx context.Context, newGameMode uint8) error {
	const BorealisSwappiness = 30
	const DefaultSwappiness = 60
	// GoBigSleepLint: add a sleep to avoid possible flakiness that can
	// be caused by the async modification of swappiness.
	testing.Sleep(ctx, 500*time.Millisecond)
	swappinessVal, err := readSwappiness(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to read swappiness")
	}
	if newGameMode == resourced.GameModeBorealis {
		// For borealis Game, swappiness should be 30.
		if swappinessVal != BorealisSwappiness {
			return errors.Errorf("swappiness value should be 30, but get %d", swappinessVal)
		}
	} else {
		// For other cases, swappiness should be default 60.
		if swappinessVal != DefaultSwappiness {
			return errors.Errorf("swappiness value should be 60, but get %d", swappinessVal)
		}
	}
	testing.ContextLog(ctx, "Swappiness validation succeed")
	return nil
}

// validateTHP checks if transparent huage page is tuned correctly:
//  1. for borealis game, tuned to always mode;
//  2. for others, not tuned.
func validateTHP(ctx context.Context, newGameMode uint8) error {
	const BorealisTHP = "always"
	const DefaultTHP = "madvise"

	const thpFile = "/sys/kernel/mm/transparent_hugepage/enabled"
	if _, err := os.Stat(thpFile); err != nil {
		testing.ContextLog(ctx, "THP is not enabled, skip the validation of THP tuning")
		return nil
	}
	thp, err := readTHP(ctx, thpFile)
	if err != nil {
		return errors.Wrap(err, "failed to read THP mode")
	}
	memInfo, err := kernelmeter.MemInfo()
	if err != nil {
		return errors.Wrap(err, "cannot obtain memory info")
	}
	// THP tuning is enabled for boards with total memory > 8GiB.
	if memInfo.Total <= kernelmeter.NewMemSizeMiB(9*1024) {
		testing.ContextLog(ctx, "THP tuning is not enabled on this device, skip the validation of THP tuning")
		return nil
	}

	if newGameMode == resourced.GameModeBorealis {
		// For borealis Game, THP should be always mode.
		if thp != BorealisTHP {
			return errors.Errorf("THP mode should be always, but got %s", thp)
		}
	} else {
		// For other cases, THP should be default madvise mode.
		if thp != DefaultTHP {
			return errors.Errorf("THP mode should be madvise, but got %s", thp)
		}
	}
	testing.ContextLog(ctx, "THP validation succeed")
	return nil
}

func checkQueryMemoryStatus(ctx context.Context, rm *resourced.Client) error {
	availableKB, err := rm.AvailableMemoryKB(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to query available memory")
	}
	testing.ContextLog(ctx, "GetAvailableMemoryKB returns: ", availableKB)

	foregroundAvailableKB, err := rm.ForegroundAvailableMemoryKB(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to query foreground available memory")
	}
	testing.ContextLog(ctx, "GetForegroundAvailableMemoryKB returns: ", foregroundAvailableKB)

	m, err := rm.MemoryMarginsKB(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to query memory margins")
	}
	testing.ContextLog(ctx, "GetMemoryMarginsKB returns, critical: ", m.CriticalKB, ", moderate: ", m.ModerateKB)

	componentMemoryMargins, err := rm.ComponentMemoryMarginsKB(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to query component memory margins")
	}

	testing.ContextLogf(ctx, "GetComponentMemoryMarginsKB returns %+v", componentMemoryMargins)

	return nil
}

func checkMemoryPressureSignal(ctx context.Context, rm *resourced.Client) error {
	// Check MemoryPressureChrome signal is sent.
	ctxWatcher, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	watcher, err := rm.NewChromePressureWatcher(ctxWatcher)
	if err != nil {
		return errors.Wrap(err, "failed to create PressureWatcher")
	}
	defer watcher.Close(ctx)

	select {
	case sig := <-watcher.Signals:
		testing.ContextLogf(ctx, "Got MemoryPressureChrome signal, level: %d, delta: %d", sig.Level, sig.Delta)
	case <-ctxWatcher.Done():
		return errors.New("didn't get MemoryPressureChrome signal")
	}

	return nil
}

func checkSetRTCAudioActive(ctx context.Context, rm *resourced.Client) error {
	// Get the original RTC audio active.
	origRTCAudioActive, err := rm.RTCAudioActive(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to query RTC audio active")
	}
	testing.ContextLog(ctx, "Original RTC audio active: ", origRTCAudioActive)

	defer func() {
		// Restore RTC audio active.
		if err = rm.SetRTCAudioActive(ctx, origRTCAudioActive); err != nil {
			testing.ContextLog(ctx, "Failed to reset RTC audio active: ", err)
		}
	}()

	// Set RTC audio ative to different value.
	newRTCAudioActive := resourced.RTCAudioActiveOff
	if origRTCAudioActive == resourced.RTCAudioActiveOff {
		newRTCAudioActive = resourced.RTCAudioActiveOn
	}
	if err = rm.SetRTCAudioActive(ctx, newRTCAudioActive); err != nil {
		// On machines not supporting Intel hardware EPP, SetRTCAudioActive returning error is expected.
		testing.ContextLog(ctx, "Failed to set RTC audio active: ", err)
	}
	testing.ContextLog(ctx, "Set RTC audio active: ", newRTCAudioActive)

	// Check RTC audio active is set to the new value.
	rtcAudioActive, err := rm.RTCAudioActive(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to query RTC audio active")
	}
	if newRTCAudioActive != rtcAudioActive {
		return errors.Errorf("failed to set RTC audio active: got %d, want: %d", rtcAudioActive, newRTCAudioActive)
	}
	return nil
}

func checkSetFullscreenVideo(ctx context.Context, rm *resourced.Client) (resErr error) {
	var newFullscreenVideo uint8 = resourced.FullscreenVideoActive
	var timeout uint32 = 1
	if err := rm.SetFullscreenVideoWithTimeout(ctx, newFullscreenVideo, timeout); err != nil {
		// On machines not supporting Intel hardware EPP, SetFullscreenVideoWithTimeout returning error is expected.
		testing.ContextLog(ctx, "Failed to set full screen video active: ", err)
		return nil
	}
	testing.ContextLogf(ctx, "Set full screen video active to %d with %d second timeout", newFullscreenVideo, timeout)

	// Check full screen video state is set to the new value.
	fullscreenVideo, err := rm.FullscreenVideo(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to query full screen video state")
	}
	if newFullscreenVideo != fullscreenVideo {
		return errors.Errorf("failed to set full screen video state: got %d, want: %d", fullscreenVideo, newFullscreenVideo)
	}

	// Check full screen video state is reset after timeout.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		fullscreenVideo, err := rm.FullscreenVideo(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to query full screen video state")
		}
		if fullscreenVideo != resourced.FullscreenVideoInactive {
			return errors.New("full screen video state is not reset")
		}
		return nil
	}, &testing.PollOptions{Timeout: time.Duration(2*timeout) * time.Second, Interval: 100 * time.Millisecond}); err != nil {
		return errors.Wrap(err, "failed to wait for full screen video state reset")
	}

	return nil
}

func checkPowerSupplyChange(ctx context.Context, rm *resourced.Client) (resErr error) {
	// Check PowerSupplyChange method can be called successfully.
	if err := rm.PowerSupplyChange(ctx); err != nil {
		return errors.Wrap(err, "failed to call power supply change")
	}

	return nil
}

func checkSetMemoryMargins(ctx context.Context, rm *resourced.Client) (resErr error) {
	// Query the original memory margins for comparison.
	marginBefore, err := rm.MemoryMarginsKB(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to query memory margins")
	}

	// Set the new memory margins.
	const (
		defaultCritical uint32 = 520
		defaultModerate uint32 = 4000
		newCritical     uint32 = defaultCritical * 2
		newModerate     uint32 = defaultModerate * 2
	)
	if err = rm.SetMemoryMarginsBps(ctx, newCritical, newModerate); err != nil {
		return errors.Wrap(err, "failed to set memory margins")
	}

	// Query the new memory margin after setting.
	marginAfter, err := rm.MemoryMarginsKB(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to query memory margins")
	}

	// Restore to the default memory margins.
	if err = rm.SetMemoryMarginsBps(ctx, defaultCritical, defaultModerate); err != nil {
		return errors.Wrap(err, "failed to set memory margins to default")
	}

	// The new memory margins should be larger.
	if marginAfter.CriticalKB <= marginBefore.CriticalKB {
		return errors.Errorf("unexpected critical margin after setting, before: %d, after: %d", marginBefore.CriticalKB, marginAfter.CriticalKB)
	}
	if marginAfter.ModerateKB <= marginBefore.ModerateKB {
		return errors.Errorf("unexpected moderate margin after setting, before: %d, after: %d", marginBefore.ModerateKB, marginAfter.ModerateKB)
	}

	return nil
}

func checkReportBackgroundProcesses(ctx context.Context, rm *resourced.Client) (resErr error) {
	// Check ReportBackgroundProcesses method can be called successfully.
	if err := rm.ReportBackgroundProcesses(ctx, rmpb.ReportBackgroundProcesses_ASH, []int32{101, 102, 103}); err != nil {
		return errors.Wrap(err, "failed to call report background processes")
	}

	return nil
}

func checkSchedQoS(ctx context.Context, rm *resourced.Client) error {
	p, err := sched.CreateSampleProcessThreadPair(ctx, nil)
	if err != nil {
		return errors.Wrap(err, "failed to create process")
	}
	defer p.KillAndWait()

	if err := rm.SetProcessState(ctx, p.Pid, resourced.QoSProcessNormal); err != nil {
		return errors.Wrap(err, "failed to set process state")
	}
	for _, state := range []uint8{resourced.QoSThreadUrgentBursty, resourced.QoSThreadUrgent, resourced.QoSThreadBalanced, resourced.QoSThreadEco, resourced.QoSThreadUtility, resourced.QoSThreadBackground} {
		if err := rm.SetThreadState(ctx, p.Pid, p.Tid, state); err != nil {
			return errors.Wrap(err, "failed to set thread state")
		}
	}
	if err := rm.SetProcessState(ctx, p.Pid, resourced.QoSProcessBackground); err != nil {
		return errors.Wrap(err, "failed to set process state")
	}
	return nil
}

func Resourced(ctx context.Context, s *testing.State) {
	rm, err := resourced.NewClient(ctx)
	if err != nil {
		s.Fatal("Failed to create Resource Manager client: ", err)
	}

	if s.Param().(resourcedTestParams).isBaseline {
		// Baseline checks.
		if err := checkSetGameMode(ctx, rm, false, false); err != nil {
			s.Fatal("Checking SetGameMode failed: ", err)
		}

		if err := checkQueryMemoryStatus(ctx, rm); err != nil {
			s.Fatal("Querying memory status failed: ", err)
		}

		if err := checkMemoryPressureSignal(ctx, rm); err != nil {
			s.Fatal("Checking memory pressure signal failed: ", err)
		}

		if err := checkSetGameModeWithTimeout(ctx, rm, false, false); err != nil {
			s.Fatal("Checking SetGameModeWithTimeout failed: ", err)
		}

		if err := checkSetRTCAudioActive(ctx, rm); err != nil {
			s.Fatal("Checking SetRTCAudioActive failed: ", err)
		}

		if err := checkSetFullscreenVideo(ctx, rm); err != nil {
			s.Fatal("Checking SetFullscreenVideoWithTimeout failed: ", err)
		}

		if err := checkPowerSupplyChange(ctx, rm); err != nil {
			s.Fatal("Checking PowerSupplyChange failed: ", err)
		}

		if err := checkSchedQoS(ctx, rm); err != nil {
			s.Fatal("Checking SchedQoS failed: ", err)
		}

		return
	}

	// New tests will be added here. Stable tests are promoted to baseline.
	if err := checkSetMemoryMargins(ctx, rm); err != nil {
		s.Fatal("Setting memory margins failed: ", err)
	}

	if err := checkSetGameMode(ctx, rm, true, true); err != nil {
		s.Fatal("Checking swappiness/THP tuning with SetGameMode failed: ", err)
	}

	if err := checkSetGameModeWithTimeout(ctx, rm, true, true); err != nil {
		s.Fatal("Checking swappiness/THP tuning with SetGameModeWithTimeout failed: ", err)
	}

	if err := checkReportBackgroundProcesses(ctx, rm); err != nil {
		s.Fatal("Checking ReportBackgroundProcesses failed: ", err)
	}
}
