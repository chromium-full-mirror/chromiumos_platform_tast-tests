// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package socketutil contains util functions for socket tests.
package socketutil

import (
	"context"
	"net"
	"runtime"
	"time"

	"go.chromium.org/tast/core/errors"
	"golang.org/x/sys/unix"
)

// uids used in the socket tests.
const (
	RootUID    = 0
	ShillUID   = 20104
	ChronosUID = 1000
)

// SwitchUser calls setreuid to switch the euid to uid, and returns a callback
// to switch the user back to root.
func SwitchUser(ctx context.Context, uid int) (cleanupFunc func() error, retErr error) {
	// Lock the goroutine to a thread at first since setreuid() only affects the
	// current thread.
	runtime.LockOSThread()
	defer func() {
		// Make sure to unlock if this function return error.
		if retErr != nil {
			runtime.UnlockOSThread()
		}
	}()

	// Note that we only need to change euid instead of ruid, otherwise we won't
	// be able to switch back. The following code assumes we are running as root
	// now.
	if err := unix.Setreuid(0, uid); err != nil {
		return nil, errors.Wrapf(err, "failed to setuid to %d", uid)
	}

	return func() error {
		var setUIDErr error
		if err := unix.Setreuid(0, 0); err != nil {
			setUIDErr = errors.Wrap(err, "failed to reset uid to root")
		}
		runtime.UnlockOSThread()
		return setUIDErr
	}, nil
}

// IOTest performs the following operations:
// 1. Send a message via conn.
// 2. Receive a message via conn.
// 3. Verify that the two messages are the same.
func IOTest(conn net.Conn) error {
	const msg = "hello world"
	const msgLen = len(msg)

	conn.SetDeadline(time.Now().Add(2 * time.Second))

	if _, err := conn.Write([]byte(msg)); err != nil {
		return errors.Wrapf(err, "failed to write msg to %s", conn.RemoteAddr())
	}

	in := make([]byte, msgLen)
	if _, err := conn.Read(in); err != nil {
		return errors.Wrapf(err, "failed to read msg from %s", conn.RemoteAddr())
	}

	inStr := string(in)
	if inStr != msg {
		return errors.Errorf("msg does not match for %s: got %s, want %s", conn.RemoteAddr(), inStr, msg)
	}

	return nil
}
