// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package gameapp contains the test code for Game app.
package gameapp

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	androidui "go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Run runs a specific game app test for a specific duration.
// game is the game app for testing.
// playTime is the time to play the game and record power consumption.
func Run(ctx context.Context, cr *chrome.Chrome, a *arc.ARC, d *androidui.Device, game GameApp, outDir, testName string, playTime time.Duration) (retErr error) {
	if err := game.Install(ctx); err != nil {
		return errors.Wrap(err, "failed to install Game")
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()
	defer func(ctx context.Context) {
		// Close the ARC UI device before dumping ARC UI Hierarchy.
		// Otherwise uiautomator might exist with errors.
		if err := d.Close(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to close the ARC UI device: ", err)
		}
		a.DumpUIHierarchyOnError(ctx, filepath.Join(outDir, "arc"), func() bool { return retErr != nil })
		faillog.DumpUITreeWithScreenshotOnError(ctx, outDir, func() bool { return retErr != nil }, cr, "ui_dump")
		if err := game.End(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to end game: ", err)
		}
		if err := game.Uninstall(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to uninstall game: ", err)
		}
	}(cleanupCtx)

	testing.ContextLog(ctx, "Start setting up the power recorder")
	const recordInterval = 5 * time.Second

	r := power.NewRecorder(ctx, recordInterval, outDir, testName)
	defer r.Close(cleanupCtx)

	if err := r.Cooldown(ctx); err != nil {
		return errors.Wrap(err, "Cooldown failed")
	}
	if err := game.Launch(ctx); err != nil {
		return errors.Wrap(err, "failed to launch game")
	}
	if err := game.EnterGameScene(ctx); err != nil {
		return errors.Wrap(err, "failed to enter game scene")
	}
	// Take a screenshot to see it enter the game scene.
	takeScreenshot(ctx, cr, outDir, "enter_game_scene")
	if err := r.Start(ctx); err != nil {
		return errors.Wrap(err, "cannot start collecting power metrics")
	}
	if err := game.Play(ctx, playTime); err != nil {
		return errors.Wrap(err, "failed to play the game")
	}
	if err := r.Finish(ctx); err != nil {
		return errors.Wrap(err, "cannot finish collecting power metrics")
	}
	// Take a screenshot to see the screen after collecting data.
	takeScreenshot(ctx, cr, outDir, "finish")

	return nil
}

// takeScreenshot captures a fullscreen screenshot.
func takeScreenshot(ctx context.Context, cr *chrome.Chrome, outDir, name string) error {
	path := fmt.Sprintf("%s/screenshot-%s.png", outDir, name)
	if err := screenshot.CaptureChrome(ctx, cr, path); err != nil {
		testing.ContextLog(ctx, "Failed to capture screenshot: ", err)
	}
	return nil
}
