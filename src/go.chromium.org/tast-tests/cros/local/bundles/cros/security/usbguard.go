// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package security

import (
	"context"
	"os"
	"time"

	"golang.org/x/sys/unix"

	"go.chromium.org/tast-tests/cros/common/fixture"
	upstartcommon "go.chromium.org/tast-tests/cros/common/upstart"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/session"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         USBGuard,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check that USBGuard-related feature flags work as intended",
		Fixture:      fixture.ChromeLoggedIn,
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome", "usbguard"},
		BugComponent: "b:1048474", // ChromeOS > Security > Usb_bouncer/Usbguard
		Contacts: []string{
			"chromeos-security@google.com",
			"allenwebb@chromium.org",
			"jorgelo@chromium.org",
		},
	})
}

func USBGuard(ctx context.Context, s *testing.State) {
	const (
		defaultUser = "testuser@gmail.com"
		defaultPass = "testpass"

		usbguardFeature    = "USBGuard"
		usbbouncerFeature  = "USBBouncer"
		usbguardJob        = "usbguard"
		usbguardWrapperJob = "usbguard-wrapper"
		usbguardProcess    = "usbguard-daemon"
		usbguardPolicy     = "/run/usbguard/rules.conf"
		usbguardUID        = 20123
		usbguardGID        = 20123

		seccompPolicyFilename = "usbguard.policy"

		jobTimeout = 10 * time.Second
	)

	unlockScreen := func() (rerr error) {
		ew, err := input.Keyboard(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to open keyboard device")
		}
		defer func() {
			if err := ew.Close(ctx); err != nil {
				rerr = errors.Wrap(err, "failed to close keyboard device")
			}
		}()

		if err := ew.Type(ctx, defaultPass+"\n"); err != nil {
			return errors.Wrap(err, "failed to type password")
		}
		return nil
	}

	expectUsbguardRunning := func(running, onLockScreen bool) error {
		goal := upstartcommon.StopGoal
		state := upstartcommon.WaitingState
		if running {
			goal = upstartcommon.StartGoal
			state = upstartcommon.RunningState
		}
		err := upstart.WaitForJobStatus(ctx, usbguardJob, goal, state, upstart.TolerateWrongGoal, jobTimeout)
		if err != nil {
			return errors.Wrapf(err, "unexpected job status for %v", usbguardJob)
		}

		if running {
			if _, err := os.Stat(usbguardPolicy); err != nil {
				return errors.Wrapf(err, "failed finding policy %v", usbguardPolicy)
			}
		} else if !onLockScreen {
			err := upstart.WaitForJobStatus(ctx, usbguardWrapperJob, goal, state, upstart.TolerateWrongGoal, jobTimeout)
			if err != nil {
				return errors.Wrapf(err, "failed to wait on job %v to stop", usbguardWrapperJob)
			}
			if _, err := os.Stat(usbguardPolicy); err == nil {
				return errors.Errorf("policy %v unexpectedly exists", usbguardPolicy)
			} else if !os.IsNotExist(err) {
				return errors.Wrapf(err, "failed checking policy %v", usbguardPolicy)
			}
		}
		return nil
	}

	checkUsbguardRespawn := func() error {
		_, _, pid, err := upstart.JobStatus(ctx, usbguardJob)
		if err != nil {
			return errors.Wrapf(err, "failed to get %v pid", usbguardJob)
		}
		if pid == 0 {
			return errors.Errorf("no pid for %v", usbguardJob)
		}

		if err := unix.Kill(pid, unix.SIGKILL); err != nil {
			err = errors.Wrapf(err, "failed to kill %v(%v)", usbguardProcess, pid)
		}
		return err
	}

	if err := expectUsbguardRunning(false /*running*/, false /*onLockScreen*/); err != nil {
		s.Errorf("Unexpected initial job status for %q: %v", usbguardJob, err)
		return
	}

	// Watch for the ScreenIsLocked signal.
	sm, err := session.NewSessionManager(ctx)
	if err != nil {
		s.Fatal("Failed to connect session_manager: ", err)
	}

	sw, err := sm.WatchScreenIsLocked(ctx)
	if err != nil {
		s.Error("Failed to observe the lock screen being shown: ", err)
		return
	}
	defer func() {
		if err := sw.Close(ctx); err != nil {
			s.Error("Failed to close session manager ScreenIsLocked watcher: ", err)
		}
	}()

	s.Log("Locking the screen")
	if err := sm.LockScreen(ctx); err != nil {
		s.Error("Failed to lock the screen: ", err)
		return
	}

	s.Log("Verifying the usbguard job is running")
	if err := expectUsbguardRunning(true /*running*/, true /*onLockScreen*/); err != nil {
		s.Error("Failed to check if usbguard is running: ", err)
	}

	s.Log("Verifying the lock screen is shown")
	select {
	case <-sw.Signals:
		s.Log("Got ScreenIsLocked signal")
	case <-ctx.Done():
		s.Error("Didn't get ScreenIsLocked signal: ", ctx.Err())
		return
	}

	if locked, err := sm.IsScreenLocked(ctx); err != nil {
		s.Error("Failed to get lock screen state: ", err)
		return
	} else if !locked {
		s.Error("Got unlocked; expected locked")
		return
	}

	s.Logf("Killing %q to check for respawn", usbguardProcess)
	if err = checkUsbguardRespawn(); err != nil {
		s.Errorf("Failed to check that %v job respawns: %v", usbguardJob, err)
	}

	// Watch for the ScreenIsUnlocked signal.
	swUnlocked, err := sm.WatchScreenIsUnlocked(ctx)
	if err != nil {
		s.Error("Failed to observe the lock screen being dismissed: ", err)
		return
	}
	defer func() {
		if err := swUnlocked.Close(ctx); err != nil {
			s.Error("Failed to close session manager ScreenIsUnlocked watcher: ", err)
		}
	}()

	s.Log("Unlocking the screen")
	if err := unlockScreen(); err != nil {
		s.Error("Failed to unlock the screen: ", err)
		return
	}

	s.Log("Verifying the lock screen is dismissed")
	select {
	case <-swUnlocked.Signals:
		s.Log("Got ScreenIsUnlocked signal")
	case <-ctx.Done():
		s.Error("Didn't get ScreenIsUnlocked signal: ", ctx.Err())
		return
	}

	if err := expectUsbguardRunning(false /*running*/, false /*onLockScreen*/); err != nil {
		s.Errorf("Unexpected final job status for %q: %v", usbguardJob, err)
	}

	if locked, err := sm.IsScreenLocked(ctx); err != nil {
		s.Error("Failed to get lock screen state: ", err)
		return
	} else if locked {
		s.Error("Got locked; expected unlocked")
		return
	}
}
