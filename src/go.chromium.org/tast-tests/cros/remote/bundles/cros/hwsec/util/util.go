// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package util

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/common/pkcs11"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	hwsecremote "go.chromium.org/tast-tests/cros/remote/hwsec"
	"go.chromium.org/tast-tests/cros/remote/u2fd"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// ServoPowerButtonHelper presses the power button using servo key press.
// Note that this will only work on specific test suites where servo-micro
// is connected (e.g., firmware_cr50).
type ServoPowerButtonHelper struct {
	svo *servo.Servo
}

// NewServoPowerButtonHelper creates a new ServoPowerButtonHelper.
func NewServoPowerButtonHelper(svo *servo.Servo) ServoPowerButtonHelper {
	return ServoPowerButtonHelper{svo}
}

// PressAndRelease implements PowerButtonHelper.PressAndRelease.
func (helper ServoPowerButtonHelper) PressAndRelease(ctx context.Context) error {
	return helper.svo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurTab)
}

// SetU2fdFlags sets the flags and restarts u2fd, which will re-create the u2f device.
func SetU2fdFlags(ctx context.Context, helper *hwsecremote.FullHelperRemote, u2f, g2f, userKeys bool) (retErr error) {
	const (
		uf2ForcePath      = "/var/lib/u2f/force/u2f.force"
		gf2ForcePath      = "/var/lib/u2f/force/g2f.force"
		userKeysForcePath = "/var/lib/u2f/force/user_keys.force"
	)

	cmd := helper.CmdRunner()
	dCtl := helper.DaemonController()

	if err := dCtl.Stop(ctx, hwsec.U2fdDaemon); err != nil {
		return errors.Wrap(err, "failed to stop u2fd")
	}
	defer func(ctx context.Context) {
		if err := dCtl.Start(ctx, hwsec.U2fdDaemon); err != nil {
			if retErr != nil {
				testing.ContextLog(ctx, "Failed to restart u2fd: ", err)
			} else {
				retErr = errors.Wrap(err, "failed to restart u2fd")
			}
		}
	}(ctx)

	// Remove flags.
	if _, err := cmd.Run(ctx, "sh", "-c", "rm -f /var/lib/u2f/force/*.force"); err != nil {
		return errors.Wrap(err, "failed to remove flags")
	}
	if u2f {
		if _, err := cmd.Run(ctx, "touch", uf2ForcePath); err != nil {
			return errors.Wrap(err, "failed to set u2f flag")
		}
	}
	if g2f {
		if _, err := cmd.Run(ctx, "touch", gf2ForcePath); err != nil {
			return errors.Wrap(err, "failed to set g2f flag")
		}
	}
	if userKeys {
		if _, err := cmd.Run(ctx, "touch", userKeysForcePath); err != nil {
			return errors.Wrap(err, "failed to set userKeys flag")
		}
	}
	return nil
}

// EnsureChapsSlotsInitialized ensures chaps is initialized.
func EnsureChapsSlotsInitialized(ctx context.Context, chaps *pkcs11.Chaps) error {
	return testing.Poll(ctx, func(context.Context) error {
		slots, err := chaps.ListSlots(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to list chaps slots")
		}
		testing.ContextLog(ctx, slots)
		if len(slots) < 2 {
			return errors.Wrap(err, "chaps initialization hasn't finished")
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  30 * time.Second,
		Interval: time.Second,
	})
}

// U2fDevicePath waits until the integrated u2f device path exists, and
// returns it.
func U2fDevicePath(ctx context.Context, cmd *hwsecremote.CmdRunnerRemote) (string, error) {
	const (
		VID = "18D1"
		PID = "5212"
	)

	lsCmd := fmt.Sprintf("ls /sys/bus/hid/devices/*:%s:%s.*/hidraw", VID, PID)
	var dev string
	err := testing.Poll(ctx, func(context.Context) error {
		data, err := cmd.Run(ctx, "sh", "-c", lsCmd)
		if err != nil {
			return errors.Wrap(err, "failed to list files")
		}
		dev = strings.TrimSpace(string(data))
		return nil
	}, &testing.PollOptions{
		Timeout:  30 * time.Second,
		Interval: 100 * time.Millisecond,
	})

	if err != nil {
		return "", errors.Wrap(err, "failed to find a HID device")
	}
	return "/dev/" + dev, nil
}

// CopyFilesToRemote is a convenient helper to call u2fd.CopyFilesToRemote
// using known data constants.
func CopyFilesToRemote(ctx context.Context, s *testing.State, cl *dutfs.Client) (string, error) {
	return u2fd.CopyFilesToRemote(ctx, s.DUT(), cl, map[string]string{
		s.DataPath("webauthn.html"): "webauthn.html",
		s.DataPath("bundle.js"):     "bundle.js",
	})
}
