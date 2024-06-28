// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// enumCallback is the function used to set up the fixture by returning Chrome options.
type enumCallback func(ctx context.Context) error

type enumTestConfig struct {
	Pre  enumCallback
	Post enumCallback
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         KernelSmokeEnum,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Smoke test for Camera Enumeration",
		Contacts:     []string{"chromeos-camera-eng@google.com", "hidenorik@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Attr:         []string{"group:mainline", "group:camera-stability", "group:camera-kernelnext"},
		SoftwareDeps: []string{caps.BuiltinCamera},
		HardwareDeps: hwdep.D(hwdep.CameraEnumerated()),
		Params: []testing.Param{
			{
				Name:      "screen_dim",
				Val:       enumTestConfig{Pre: screenDimByPolicy, Post: powerResetPolicy},
				ExtraAttr: []string{"informational"},
			}, {
				Name:      "screen_off",
				Val:       enumTestConfig{Pre: screenOffByPolicy, Post: powerResetPolicy},
				ExtraAttr: []string{"informational"},
			},
		},
	})
}

func KernelSmokeEnum(ctx context.Context, s *testing.State) {
	config, ok := s.Param().(enumTestConfig)
	if !ok {
		s.Fatal("Failed to parse test param")
	}

	shortCtx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if err := config.Pre(shortCtx); err != nil {
		s.Fatal("Failed to execute pre hook: ", err)
	}
	defer func(ctx context.Context) {
		if err := config.Post(ctx); err != nil {
			s.Fatal("Failed to exec post hook: ", err)
		}
	}(ctx)

	if err := testutil.CheckBuiltinCameraEnumeration(shortCtx); err != nil {
		s.Fatal("Failed to enumerate all builtin cameras: ", err)
	}
}

func screenDimByPolicy(ctx context.Context) error {
	if err := testexec.CommandContext(ctx, "set_power_policy", "--ac_screen_dim_delay=1").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to change power policy")
	}

	// GoBigSleepLint: Need to wait for screen to be dimmed.
	if err := testing.Sleep(ctx, 1*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}

	return nil
}

func screenOffByPolicy(ctx context.Context) error {
	if err := testexec.CommandContext(ctx, "set_power_policy", "--ac_screen_off_delay=1").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to change power policy")
	}

	// GoBigSleepLint: Need to wait for screen to be off.
	if err := testing.Sleep(ctx, 1*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}

	return nil
}

func powerResetPolicy(ctx context.Context) error {
	if err := testexec.CommandContext(ctx, "set_power_policy", "reset").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to reset power policy")
	}
	return nil
}
