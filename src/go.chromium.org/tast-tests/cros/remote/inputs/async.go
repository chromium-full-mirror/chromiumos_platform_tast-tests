// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast/core/testing"
)

// RunAsync runs a blocking function |f| asynchronously inside a goroutine. If
// |f| panics, we log the panic message, sleep briefly to give the log time to
// be visible, and then re-panic.
//
// Thus, the panic is still thrown, but only after the initial logging has
// been completed. It is assumed that |f| does not contain its own internal
// goroutine, as the built in function recover() does not work for panics
// inside goroutines that are not on the main thread.
func RunAsync(ctx context.Context, f func(ctx context.Context) error) <-chan error {
	out := make(chan error)
	go func() {
		defer func(ctx context.Context) {
			if err := recover(); err != nil {
				panicMsg := fmt.Sprintf("panic: %v", err)
				defer panic(panicMsg)

				testing.ContextLog(ctx, panicMsg)

				// GoBigSleepLint: Sleep a little bit to make sure the panic log is flushed.
				if err := testing.Sleep(ctx, 3*time.Second); err != nil {
					testing.ContextLog(ctx, "Failed to sleep while recovering from a panic: ", err)
				}
			}
		}(ctx)
		out <- f(ctx)
	}()
	return out
}
