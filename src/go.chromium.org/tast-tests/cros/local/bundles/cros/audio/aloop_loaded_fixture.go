// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"os"
	"regexp"
	"strconv"
	"time"

	upstartcommon "go.chromium.org/tast-tests/cros/common/upstart"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/fixture"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type aloopLoadedFixtureParam struct {
	channels   int
	uiJobGoal  upstartcommon.Goal
	uiJobState upstartcommon.State
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         AloopLoadedFixture,
		Desc:         "Test the AloopLoaded fixture",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "aaronyu@google.com"},
		BugComponent: "b:776546",
		Attr:         []string{"group:mainline"},
		Timeout:      1 * time.Minute,
		Params: []testing.Param{
			{
				Fixture: fixture.AloopLoaded{}.Instance(),
				Val: aloopLoadedFixtureParam{
					channels:   8, // the default
					uiJobGoal:  upstartcommon.StartGoal,
					uiJobState: upstartcommon.RunningState,
				},
			},
			{
				Name:    "stereo",
				Fixture: fixture.AloopLoaded{Channels: 2}.Instance(),
				Val: aloopLoadedFixtureParam{
					channels:   2,
					uiJobGoal:  upstartcommon.StartGoal,
					uiJobState: upstartcommon.RunningState,
				},
			},
			{
				Name: "without_ui",
				Fixture: fixture.AloopLoaded{
					Parent: fixture.UIStopped{}.Instance(),
				}.Instance(),
				Val: aloopLoadedFixtureParam{
					channels:   8, // the default
					uiJobGoal:  upstartcommon.StopGoal,
					uiJobState: upstartcommon.WaitingState,
				},
			},
			{
				Name: "stereo_without_ui",
				Fixture: fixture.AloopLoaded{
					Channels: 2,
					Parent:   fixture.UIStopped{}.Instance(),
				}.Instance(),
				Val: aloopLoadedFixtureParam{
					channels:   2,
					uiJobGoal:  upstartcommon.StopGoal,
					uiJobState: upstartcommon.WaitingState,
				},
			},
		},
	})
}

// AloopLoadedFixture tests the AloopLoaded fixture.
func AloopLoadedFixture(ctx context.Context, s *testing.State) {
	const (
		aloopModulePath = "/sys/module/snd_aloop/"
		crasAloopType   = "ALSA_LOOPBACK"
	)

	param := s.Param().(aloopLoadedFixtureParam)

	fileInfo, err := os.Stat(aloopModulePath)
	if err != nil {
		s.Fatalf("Failed to stat %s: %v", aloopModulePath, err)
	}
	if !fileInfo.IsDir() {
		s.Fatalf("%s is not a directory", aloopModulePath)
	}

	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Cannot connect to CRAS: ", err)
	}
	if _, err := cras.GetNodeByType(ctx, crasAloopType); err != nil {
		s.Error("CRAS alsa loopback device not found: ", err)
	}

	channels, err := crasAloopChannels()
	if err != nil {
		s.Error("Cannot get number of aloop channels: ", err)
	} else if channels != param.channels {
		s.Errorf("Unexpected aloop channels: want %d; got %d", param.channels, channels)
	}

	// Check for UI job status
	goal, state, _, err := upstart.JobStatus(ctx, "ui")
	if err != nil {
		s.Fatal("Cannot check state of ui job: ", err)
	}
	if goal != param.uiJobGoal || state != param.uiJobState {
		s.Errorf("Expected UI in %s/%s; got %s/%s", param.uiJobGoal, param.uiJobState, goal, state)
	}
}

// crasAloopChannels the number of channels of the aloop device configured in CRAS.
func crasAloopChannels() (int, error) {
	// TODO(aaronyu): Figure out the channel count from CRAS, instead of a config file.
	b, err := os.ReadFile("/usr/share/alsa/ucm/Loopback/HiFi.conf")
	if err != nil {
		return 0, err
	}

	config := string(b)
	m := regexp.MustCompile(`PlaybackChannels "(\d+)"`).FindStringSubmatch(config)
	if m == nil {
		return 0, errors.New("cannot find PlaybackChannels from UCM")
	}

	return strconv.Atoi(m[1])
}
