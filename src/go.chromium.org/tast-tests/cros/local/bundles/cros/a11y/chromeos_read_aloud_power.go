// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package a11y

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type readAloudPowerParam struct {
	enabled bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromeosReadAloudPower,
		LacrosStatus: testing.LacrosVariantUnneeded, // b:338224021 not support lacros yet.
		Desc:         "Checks read loud feature power usage",
		Contacts: []string{
			"komo-eng@google.com",
			"trewin@google.com",
			"xiuwen@google.com",
		},
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		BugComponent: "b:1372781",
		Timeout:      30*time.Minute + power.RecorderTimeout,
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{
			{
				Name:    "ash_enable",
				Fixture: setup.PowerAshReadAloudWithFlagOn,
				Val: readAloudPowerParam{
					enabled: true,
				},
			},
			{
				Name:    "ash_disable",
				Fixture: setup.PowerAsh,
				Val: readAloudPowerParam{
					enabled: false,
				},
			},
		},
	})
}

func ChromeosReadAloudPower(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	bt := s.FixtValue().(setup.PowerUIFixtureData).Bt
	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr

	conn, _, cleanup, err := browserfixt.SetUpWithURL(ctx, cr, bt, "https://en.wikipedia.org/wiki/ChromeOS")
	if err != nil {
		s.Fatal("Failed to open wiki page: ", err)
	}
	defer cleanup(cleanupCtx)
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	tconn, err := cr.TestAPIConn(ctx)
	ui := uiauto.New(tconn)

	wikiTextField := nodewith.ClassName("BrowserFrame")
	openInReadModeItem := nodewith.NameContaining("Open in reading mode").ClassName("MenuItemView")
	playAudioButton := nodewith.Name("Play keyboard shortcut k").Role(role.Button).ClassName("toolbar-button audio-controls")
	pauseAudioButton := nodewith.Name("Pause keyboard shortcut k").Role(role.Button).ClassName("toolbar-button audio-controls")

	r := power.NewRecorder(ctx, 10*time.Second, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)
	if err := r.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}
	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}

	// Test body.
	param := s.Param().(readAloudPowerParam)
	if param.enabled {
		if err := uiauto.Combine("open reading mode",
			ui.RightClickUntil(wikiTextField, ui.Exists(openInReadModeItem)),
			ui.DoDefaultUntil(openInReadModeItem, ui.Exists(playAudioButton)),
		)(ctx); err != nil {
			s.Fatal("Failed to verify open reading mode: ", err)
		}

		//GoBigSleepLint: avoid reading mode is not fully loaded with the right content b:337985323.
		testing.Sleep(ctx, 3*time.Second)
		if err := ui.DoDefaultUntil(playAudioButton, ui.Exists(pauseAudioButton))(ctx); err != nil {
			s.Fatal("Failed to start reading: ", err)
		}
	}

	// GoBigSleepLint: sleep to let the device idle.
	if err := testing.Sleep(ctx, 12*time.Minute); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	// Stop power test and clean the environment.
	if err := r.Finish(ctx); err != nil {
		s.Error("Failed to finish collecting power metrics: ", err)
	}
}
