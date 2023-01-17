// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"os"
	"time"

	"chromiumos/tast/common/fixture"
	upstartcommon "chromiumos/tast/common/upstart"
	"chromiumos/tast/local/audio"
	"chromiumos/tast/local/upstart"
	"chromiumos/tast/testing"
)

type aloopLoadedFixtureParam struct {
	uiJobGoal  upstartcommon.Goal
	uiJobState upstartcommon.State
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         AloopLoadedFixture,
		Desc:         "Test the AloopLoaded fixture",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "aaronyu@google.com"},
		BugComponent: "b:875484",
		Attr:         []string{"group:mainline", "informational"},
		Timeout:      1 * time.Minute,
		Params: []testing.Param{
			{
				Fixture: fixture.AloopLoaded,
				Val: aloopLoadedFixtureParam{
					uiJobGoal:  upstartcommon.StartGoal,
					uiJobState: upstartcommon.RunningState,
				},
			},
			{
				Name:    "without_ui",
				Fixture: fixture.AloopLoadedWithoutUI,
				Val: aloopLoadedFixtureParam{
					uiJobGoal:  upstartcommon.StopGoal,
					uiJobState: upstartcommon.WaitingState,
				},
			},
		},
	})
}

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

	// Check for UI job status
	goal, state, _, err := upstart.JobStatus(ctx, "ui")
	if err != nil {
		s.Fatal("Cannot check state of ui job: ", err)
	}
	if goal != param.uiJobGoal || state != param.uiJobState {
		s.Errorf("Expected UI in %s/%s; got %s/%s", param.uiJobGoal, param.uiJobState, goal, state)
	}
}
