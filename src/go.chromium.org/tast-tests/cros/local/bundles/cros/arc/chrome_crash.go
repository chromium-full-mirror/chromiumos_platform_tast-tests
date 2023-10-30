// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"golang.org/x/sys/unix"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash/ashproc"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromeCrash,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test ash chrome crash handling of ARC",
		Contacts:     []string{"arc-core@google.com"},
		// ChromeOS > Software > ARC++ > Core
		BugComponent: "b:488493",
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Val:               false,
			ExtraAttr:         []string{"group:mainline", "informational"},
			ExtraSoftwareDeps: []string{"android_container"},
		}, {
			Name:              "logged_in",
			Val:               true,
			ExtraAttr:         []string{"group:mainline", "informational"},
			ExtraSoftwareDeps: []string{"android_container"},
		}, {
			Name:              "vm_logged_in",
			Val:               true,
			ExtraAttr:         []string{"group:mainline", "group:hw_agnostic"},
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
		Timeout: 10 * time.Minute,
	})
}

func ChromeCrash(ctx context.Context, s *testing.State) {
	loggedIn := s.Param().(bool)

	// Shorten the context to save time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if loggedIn {
		func() {
			cr, err := chrome.New(ctx, chrome.ARCEnabled())
			if err != nil {
				s.Fatal("Failed to connect to Chrome: ", err)
			}
			defer func() {
				if err := cr.Close(cleanupCtx); err != nil {
					s.Fatal("Failed to close Chrome: ", err)
				}
			}()
			a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
			if err != nil {
				s.Fatal("Failed to start ARC: ", err)
			}
			defer func() {
				if err := a.Close(ctx); err != nil {
					s.Fatal("Failed to close ARC connection: ", err)
				}
			}()
		}()
	} else {
		// Ensure login screen.
		if err := upstart.RestartJob(ctx, "ui"); err != nil {
			s.Fatal("Failed to restart ui: ", err)
		}
		s.Log("Waiting for Android init process")
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			_, err := arc.InitPID()
			return err
		}, &testing.PollOptions{Timeout: arc.BootTimeout}); err != nil {
			s.Fatal("Failed to wait for Android init process: ", err)
		}
	}
	oldPID, err := arc.InitPID()
	if err != nil {
		s.Fatal("Failed to get init PID before reboot: ", err)
	}

	// Chrome crash should result in Android reboot.
	s.Log("Inducing chrome crash")

	proc, err := ashproc.Root()
	if err != nil {
		s.Fatal("Failed to get chrome proc: ", err)
	}
	if err := proc.SendSignalWithContext(ctx, unix.SIGSEGV); err != nil {
		s.Fatal("Failed to crash chrome: ", err)
	}

	s.Log("Waiting for a new Android init process")
	if err = testing.Poll(ctx, func(ctx context.Context) error {
		pid, err := arc.InitPID()
		if err != nil {
			return err
		}
		if pid == oldPID {
			return errors.New("init still exists")
		}
		return nil
	}, &testing.PollOptions{Timeout: arc.BootTimeout}); err != nil {
		s.Fatal("Failed to wait for restarted Android init process: ", err)
	}
}
