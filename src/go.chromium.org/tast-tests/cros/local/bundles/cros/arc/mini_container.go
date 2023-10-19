// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/syslog"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MiniContainer,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Ensures Android mini container is upgraded after login",
		Contacts:     []string{"arc-core@google.com"},
		// ChromeOS > Software > ARC++ > Core
		BugComponent: "b:488493",
		Attr:         []string{"group:mainline", "informational"},
		// Container-R doesn't support mini-container.
		SoftwareDeps: []string{"android_p", "chrome"},
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
	})
}

func MiniContainer(ctx context.Context, s *testing.State) {
	// Make sure the Android container is stopped initially.
	upstart.StopJob(ctx, "ui")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := arc.BootstrapCommand(ctx, "/system/bin/true").Run(); err == nil {
			return errors.New("Android container still running")
		}
		return nil
	}, nil); err != nil {
		s.Fatal("Failed to wait for the Android container to stop: ", err)
	}

	reader, err := syslog.NewReader(ctx)
	if err != nil {
		s.Fatal("Failed to open syslog reader: ", err)
	}
	defer reader.Close()

	initCh := make(chan error, 1)
	sleepCh := make(chan error, 1)

	// Start a goroutine that sends messages over channels as the Android mini container is brought up.
	go func() {
		// Wait for the Android mini container to start.
		initCh <- arc.WaitAndroidInit(ctx, reader)

		// Start a process in the Android mini container. This process should be running
		// until we close *ARC.
		sleepCh <- arc.BootstrapCommand(ctx, "/system/bin/sleep", "86400").Run()
	}()

	cr, err := chrome.New(ctx, chrome.ARCEnabled(), chrome.UnRestrictARCCPU())
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}
	defer cr.Close(ctx)

	// Android mini container should be running at this point usually, but wait for it anyway.
	if err := <-initCh; err != nil {
		s.Fatal("Failed to wait for the Android mini container: ", err)
	}

	// Wait for a while after login to make sure the Android mini container is not turned down
	// even if we do not call arc.New immediately (crbug.com/872135).
	// GoBigSleepLint: TODO(b/306491449): Justify sleep
	if err := testing.Sleep(ctx, 3*time.Second); err != nil {
		s.Fatal("Timed out while sleeping after login: ", err)
	}

	select {
	case err := <-sleepCh:
		s.Fatal("Android mini container failed to upgrade: sleep process killed: ", err)
	default:
	}

	// Wait for Android to fully boot.
	a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer a.Close(ctx)

	select {
	case err := <-sleepCh:
		s.Fatal("Android mini container failed to upgrade: sleep process killed: ", err)
	default:
	}
}
