// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            fixture.CrasStopped,
		Desc:            "Ensure CRAS is stopped and audio devices are available for direct access",
		Contacts:        []string{"chromeos-audio-bugs@google.com", "aaronyu@google.com"},
		Impl:            crasStoppedFixture{},
		BugComponent:    "b:776546",
		SetUpTimeout:    20 * time.Second,
		TearDownTimeout: 20 * time.Second,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         fixture.FakeCrasClient,
		Desc:         "Use fake cras client for Chrome to avoid UI manipulating volume and mute",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "htcheong@google.com"},
		BugComponent: "b:776546",
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				// Prevent interference of audio preferences.
				// See go/tast-fakecrasaudioclient.
				chrome.ExtraArgs("--use-fake-cras-audio-client-for-dbus"),
			}, nil
		}),
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
}

// crasStoppedFixture is a fixture to stop CRAS and sleep for a while to
// ensure that (ALSA) audio devices are available.
// Tests that need to access audio devices directly should use this fixture.
type crasStoppedFixture struct{}

func (crasStoppedFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	if err := upstart.StopJob(ctx, "cras"); err != nil {
		s.Fatal("Cannot stop cras: ", err)
	}

	// We might need to sleep longer to work around improperly powered down devices.
	// See b/232799132 for an example that generated flakes.
	const sleepDuration = time.Second

	s.Logf("Sleeping for %s to wait for audio device to be ready", sleepDuration)
	// GoBigSleepLint: sleep is the only way to wait for audio device being ready as probing unnecessarily open the device.
	if err := testing.Sleep(ctx, sleepDuration); err != nil {
		s.Fatal("Sleep failed: ", err)
	}
	return nil
}

func (crasStoppedFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if _, err := RestartCras(ctx); err != nil {
		s.Fatal("Cannot restart cras: ", err)
	}
}

func (crasStoppedFixture) Reset(ctx context.Context) error {
	return nil
}

func (crasStoppedFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (crasStoppedFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}
