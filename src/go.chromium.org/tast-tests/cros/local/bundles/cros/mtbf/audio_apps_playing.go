// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mtbf

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc/apputil"
	"go.chromium.org/tast-tests/cros/local/arc/apputil/spotify"
	"go.chromium.org/tast-tests/cros/local/arc/apputil/youtubemusic"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/mtbf"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type audioAppType string

const (
	appSpotify audioAppType = spotify.AppName
	appYtMusic audioAppType = youtubemusic.AppName
)

func init() {
	testing.AddTest(&testing.Test{
		Func: AudioAppsPlaying,
		Desc: "Test and verify top ARC++ audio apps are working",
		Contacts: []string{
			"chromeos-perf-reliability-eng@google.com",
			"abergman@google.com",
			"xibin@google.com",
			"cienet-development@googlegroups.com",
			"vivian.tsai@cienet.com", // Test author.
		},
		// ChromeOS > EngProd > Platform > SPERA > Automation
		BugComponent: "b:1025042",
		// MTBF tests are not included in mainline or crosbolt for now.
		Attr:         []string{"group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "arc"},
		Fixture:      mtbf.LoginReuseFixture,
		Timeout:      5*time.Minute + apputil.InstallationTimeout,
		Params: []testing.Param{
			{
				Name: "spotify",
				Val:  appSpotify,
			}, {
				Name: "youtubemusic",
				Val:  appYtMusic,
			},
		},
	})
}

// AudioAppsPlaying verifies top ARC++ audio apps are working.
func AudioAppsPlaying(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	recorder, err := mtbf.NewRecorder(ctx)
	if err != nil {
		s.Fatal("Failed to start record performance: ", err)
	}
	defer recorder.Record(cleanupCtx, s.OutDir())

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	a := s.FixtValue().(*mtbf.FixtValue).ARC
	device, err := a.NewUIDevice(ctx)
	if err != nil {
		s.Fatal("Failed initializing UI Automator: ", err)
	}
	defer device.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to open test API connection: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard controller: ", err)
	}
	defer kb.Close(ctx)

	var (
		app  apputil.ARCMediaPlayer
		song *apputil.Media
	)

	switch s.Param().(audioAppType) {
	case appYtMusic:
		app, err = youtubemusic.New(ctx, kb, tconn, a, device)
		if err != nil {
			s.Fatal("Failed to create YouTube Music app instance: ", err)
		}
		song = apputil.NewMedia("Blank Space", "Taylor Swift • 3:52")
	case appSpotify:
		app, err = spotify.New(ctx, kb, a, tconn, device, cr.Creds().User)
		if err != nil {
			s.Fatal("Failed to create Spotify app instance: ", err)
		}
		song = apputil.NewMedia("Photograph", "Song • Ed Sheeran")
	default:
		s.Fatal("Unrecognized app type: ", s.Param())
	}

	if err := app.Install(ctx); err != nil {
		s.Fatal("Failed to install app: ", err)
	}

	if _, err := app.Launch(ctx); err != nil {
		s.Fatal("Failed to launch app: ", err)
	}
	defer app.Close(cleanupCtx, cr, s.HasError, s.OutDir())

	if err := app.Play(ctx, song); err != nil {
		s.Fatal("Failed to play song: ", err)
	}

	if ytm, ok := s.Param().(*youtubemusic.YouTubeMusic); ok {
		if err := ytm.RemovePlayRecord(cleanupCtx); err != nil {
			s.Fatal("Failed to remove play record: ", err)
		}
	}
}
