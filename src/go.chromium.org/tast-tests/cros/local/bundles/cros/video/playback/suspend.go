// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package playback provides common code for video.Playback* tests.
package playback

import (
	"context"
	"strings"
	"sync"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast-tests/cros/local/syslog"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type suspendConfig struct {
	suspendMode graphics.SuspendMode
	pmTestMode  graphics.PmTestMode
}

func reconnectToBrowser(ctx context.Context, cr *chrome.Chrome, browserType browser.Type) (*chrome.Conn, error) {
	// Reconnect to Chrome.
	testing.ContextLog(ctx, "Reconnect to browser connection")
	if err := cr.Reconnect(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to reconnect to Chrome")
	}
	// Reconnect to browser.
	br, _, err := browserfixt.Connect(ctx, cr, browserType)
	if err != nil {
		return nil, errors.Wrap(err, "failed to reconnect to browser")
	}
	conn, err := br.NewConnForTarget(ctx, func(t *chrome.Target) bool {
		return strings.HasSuffix(t.URL, "video.html")
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to establish browser connection")
	}
	testing.ContextLog(ctx, "Browser connection is established")
	return conn, nil
}

// suspendSystem suspends the system and checks the validity of syslog and video after system resumes.
func suspendSystem(ctx context.Context, cr *chrome.Chrome, reader *syslog.Reader, config Config, sConfig suspendConfig, testName string) error {
	// Check syslog for GPU hangs and decoding errors before we start suspend/resume.
	if err := graphics.CheckSysLog(ctx, testName, reader); err != nil {
		return errors.Wrap(err, "syslog signature found")
	}
	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return errors.New("failed to get output directory")
	}

	if err := graphics.SetPMTest(ctx, sConfig.pmTestMode); err != nil {
		return errors.Wrap(err, "failed to set pm_test")
	}

	out, err := testexec.CommandContext(ctx, "suspend_stress_test", "--count", "1", "--nopremature_wake_fatal", "--record_dmesg_dir", outDir).Output(testexec.DumpLogOnError)
	testing.ContextLog(ctx, "suspend_stress_test Output: ", string(out))
	if err != nil {
		return errors.Wrapf(err, "suspend_stress_test to %v failed", sConfig.suspendMode)
	}
	msg, failed := graphics.GetSuspendStressTestError(ctx, sConfig.suspendMode, sConfig.pmTestMode, string(out))
	if failed {
		return errors.Errorf("suspend_stress_test error while suspend to %v: %v", sConfig.suspendMode, msg)
	}
	conn, err := reconnectToBrowser(ctx, cr, config.BrowserType)
	// Check |currentTime| variable is changing.
	originalPlayingTime, err := getPlayingTime(ctx, conn)
	if err != nil {
		return errors.Wrap(err, "failed to get original playing time")
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		playingTime, err := getPlayingTime(ctx, conn)
		if err != nil {
			return errors.Wrap(err, "failed to get playing time")
		}
		if playingTime == originalPlayingTime {
			return errors.Errorf("playing time %v is the same as original time %v", playingTime, originalPlayingTime)
		}
		return nil
	}, nil); err != nil {
		return errors.Wrap(err, "video playing time is not advancing")
	}
	// Check syslog for GPU hangs and decoding errors after the video starts playing.
	if err := graphics.CheckSysLog(ctx, testName, reader); err != nil {
		return errors.Wrap(err, "syslog signature found")
	}
	return nil
}

// suspendResume tests video playback continuity after system goes through suspend/resume cycle.
func suspendResume(ctx context.Context, cr *chrome.Chrome, config Config, testName string) (resultErr error) {
	var wg sync.WaitGroup
	wg.Add(1)
	errChan := make(chan error)
	quit := make(chan bool)

	origPmMode, err := graphics.GetPMTestState(ctx)
	if err != nil {
		errChan <- errors.Wrap(err, "failed to get original pm_test state")
		return
	}
	testing.ContextLogf(ctx, "Original pm_test state: %q", origPmMode)
	// Always try set the mode back to original.
	defer graphics.SetPMTest(ctx, origPmMode.Current)

	suspendMode := graphics.GetSuspendState(ctx)
	testing.ContextLog(ctx, "DUT will suspend to ", suspendMode)

	pmModes := []graphics.PmTestMode{graphics.PmTestNone, graphics.PmTestFreezer, graphics.PmTestDevices, graphics.PmTestPlatform}
	// Start reading the syslog so we can stop the tests as soon as any GPU hangs/decode errors are found.
	reader, err := syslog.NewReader(ctx, syslog.Severities(syslog.Info, syslog.Warning, syslog.Err))
	if err != nil {
		return errors.Wrap(err, "failed to get syslog reader")
	}
	defer reader.Close()
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-quit:
				return
			default:
				testing.ContextLogf(ctx, "Executing [%v] suspend ", i)
			}

			suspendCtx, cancel := context.WithTimeout(ctx, SuspendSystemTimeout)
			defer cancel()

			// We tried to putting system into different pm_test state to help reveal problems.
			// This switch the mode for every 2 suspend.
			sConfig := suspendConfig{
				suspendMode: suspendMode,
				pmTestMode:  pmModes[(i/2)%len(pmModes)],
			}
			if err := suspendSystem(suspendCtx, cr, reader, config, sConfig, testName); err != nil {
				errChan <- errors.Wrapf(err, "suspend [%v] failed with pm_test mode [%v]", i, sConfig.pmTestMode)
				return
			}
			if suspendCtx.Err() != nil {
				errChan <- errors.Wrapf(err, "suspend/resume cycle took more than %v", SuspendSystemTimeout)
				return
			}
			// GoBigSleepLint: Add a suspend intervals so video can progress.
			if err := testing.Sleep(ctx, SuspendSystemInterval); err != nil {
				errChan <- errors.Wrap(err, "failed to sleep after suspend")
				return
			}
		}
	}()

	// Wait either suspend fails or playbackDuration is passed.
	select {
	case resultErr = <-errChan:
		break
	case <-ctx.Done():
		quit <- true
		return ctx.Err()
	case <-time.After(config.Duration):
		testing.ContextLog(ctx, "playback duration hit, stopping suspend_resume loop")
		quit <- true
	}
	wg.Wait()
	return
}
