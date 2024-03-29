// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pre

import (
	"context"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/logsaver"
	"go.chromium.org/tast-tests/cros/local/media/logging"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func initChromeVideoStressFixtures() {
	// Primarily used in stress testing to not reset Chrome state between each test runs.
	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoStress",
		Desc:     "Logged into a user session with logging enabled",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: &chromeVideoStressImpl{
			browserType: browser.TypeAsh,
			fOpt: func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
				return []chrome.Option{
					chrome.ExtraArgs(chromeVideoArgs...),
					chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				}, nil
			},
		},
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	// Same as chromeVideoLacros but used for stress testing.
	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoStressLacros",
		Desc:     "Logged into a user session with logging enabled (lacros)",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: &chromeVideoStressImpl{
			browserType: browser.TypeLacros,
			fOpt: func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
				return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(
					chrome.ExtraArgs(chromeVideoArgs...),
					chrome.LacrosExtraArgs(chromeVideoArgs...),
					chrome.ExtraArgs(chromeBypassPermissionsArgs...),
					chrome.LacrosExtraArgs(chromeBypassPermissionsArgs...))).Opts()
			},
		},
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
}

type chromeVideoStressImpl struct {
	browserType             browser.Type
	fOpt                    chrome.OptionsCallback
	resetChromeBetweenTests bool

	logMarker         *logsaver.Marker // Marker for per-test log.
	origShelfBehavior ash.ShelfBehavior
	vl                *logging.VideoLogger
	cr                *chrome.Chrome
}

func (f *chromeVideoStressImpl) Reset(ctx context.Context) error {
	if err := f.cr.Responded(ctx); err != nil {
		return errors.Wrap(err, "existing Chrome connection is unusable")
	}
	if f.resetChromeBetweenTests {
		if err := f.cr.ResetState(ctx); err != nil {
			return errors.Wrap(err, "failed resetting existing Chrome session")
		}
	}
	return nil
}

func (f *chromeVideoStressImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
	if f.logMarker != nil {
		s.Log("A log marker is already created but not cleaned up")
	}
	logMarker, err := logsaver.NewMarker(f.cr.LogFilename())
	if err == nil {
		f.logMarker = logMarker
	} else {
		s.Log("Failed to start the log saver: ", err)
	}
}

func (f *chromeVideoStressImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
	if f.logMarker != nil {
		if err := f.logMarker.Save(filepath.Join(s.OutDir(), "chrome.log")); err != nil {
			s.Log("Failed to store per-test log data: ", err)
		}
		f.logMarker = nil
	}
}

func (f *chromeVideoStressImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	crOpts, err := f.fOpt(ctx, s)
	if err != nil {
		s.Fatal("Failed to obtain Chrome options: ", err)
	}

	cr, err := chrome.New(ctx, crOpts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	f.cr = cr

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Set shelf to auto-hide.
	dispInfo, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get primary display info: ", err)
	}
	origShelfBehavior, err := ash.GetShelfBehavior(ctx, tconn, dispInfo.ID)
	if err != nil {
		s.Fatal("Failed to get shelf behavior: ", err)
	}
	f.origShelfBehavior = origShelfBehavior
	if err := ash.SetShelfBehavior(ctx, tconn, dispInfo.ID, ash.ShelfBehaviorAlwaysAutoHide); err != nil {
		s.Fatal("Failed to set shelf behavior to Never Auto Hide: ", err)
	}

	// video Logger and mute the devices.
	vl, err := logging.NewVideoLogger()
	if err != nil {
		s.Fatal("Failed to set values for verbose logging")
	}
	f.vl = vl
	if err := crastestclient.Mute(ctx); err != nil {
		s.Fatal("Failed to mute device: ", err)
	}
	chrome.Lock()
	return cr
}

func (f *chromeVideoStressImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	chrome.Unlock()

	tconn, err := f.cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	// Set shelf to auto-hide.
	dispInfo, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get primary display info: ", err)
	}
	if err := ash.SetShelfBehavior(ctx, tconn, dispInfo.ID, f.origShelfBehavior); err != nil {
		s.Fatal("Failed to set shelf behavior to Never Auto Hide: ", err)
	}

	if f.vl != nil {
		f.vl.Close()
	}
	crastestclient.Unmute(ctx)

	if err := f.cr.Close(ctx); err != nil {
		s.Log("Failed to close Chrome connection: ", err)
	}
	f.cr = nil
}
