// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAAutoQRPower,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Collect power metrics with autoQR flag",
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Contacts:     []string{"chromeos-camera-eng@google.com", "dorahkim@chromium.org"},
		SoftwareDeps: []string{"chrome", "camera_app"},
		Params: []testing.Param{{
			Name:      "autoqr_disabled",
			Fixture:   "ccaPowerReviewWithFakeHALCamera",
			ExtraAttr: []string{"group:crosbolt", "crosbolt_nightly"},
			Val:       cca.PowerTimeParams,
			Timeout:   6*time.Minute + power.RecorderTimeout,
		}, {
			Name:      "autoqr_enabled",
			Fixture:   "ccaPowerReviewWithFakeHALCameraAutoQREnabled",
			ExtraAttr: []string{"group:crosbolt", "crosbolt_nightly"},
			Val:       cca.PowerTimeParams,
			Timeout:   6*time.Minute + power.RecorderTimeout,
		}},
	})
}

func CCAAutoQRPower(ctx context.Context, s *testing.State) {
	startApp := s.FixtValue().(cca.FixtureData).StartApp
	stopApp := s.FixtValue().(cca.FixtureData).StopApp

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	interval := s.Param().(power.TimeParams).Interval
	total := s.Param().(power.TimeParams).Total

	cr := s.FixtValue().(cca.FixtureData).Chrome
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	r := power.NewRecorder(ctx, interval, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)

	if err := power.Cooldown(ctx); err != nil {
		s.Log("Failed to cooldown before the setup: ")
	}

	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}

	if _, err := startApp(ctx); err != nil {
		s.Fatal("Failed to open CCA: ", err)
	}
	defer func(cleanupCtx context.Context) {
		if err := stopApp(cleanupCtx, s.HasError()); err != nil {
			s.Fatal("Failed to close CCA: ", err)
		}
	}(cleanupCtx)

	// GoBigSleepLint: sleep to let the device idle.
	if err := testing.Sleep(ctx, total); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}
	if err := r.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}
}
