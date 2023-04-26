// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package async contains functions useful for spawning panic logging goroutines.
package async

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast/core/testing"
)

// Run runs |f| inside a goroutine. If |f| panics, we log the panic message,
// sleep briefly to give the log time to be visible, and then re-panic.
// The panic message is constructed as follows:
// panic: |prefix| panicked; |panic message|
// For example, `Power Timeline panicked: index out of bounds`
//
// Thus, the panic is still thrown, but only after the initial logging has
// been completed. It is assumed that |f| does not contain its own internal
// goroutine, as the built in function recover() does not work for panics
// inside goroutines that are not on the main thread.
//
// The reason this function is necessary is that in tast-tests that use
// goroutines, if a spawned goroutine panics, the whole test bundle crashes
// and all logs of why the panic occurred are lost. |Run| logs this panic to
// make these issues easier to debug.
//
// This function relies on |ctx| remaining open for the full duration of the
// function |f|. If |ctx| is closed early (for example because the caller goes
// out of scope), then |f| is terminated early.
func Run(ctx context.Context, f func(ctx context.Context), prefix string) {
	go func() {
		defer func(ctx context.Context) {
			if err := recover(); err != nil {
				panicMsg := fmt.Sprintf("panic: %s panicked; %v", prefix, err)
				defer panic(panicMsg)

				testing.ContextLog(ctx, panicMsg)

				// Sleep a little bit to make sure the panic log is flushed.
				if err := testing.Sleep(ctx, 3*time.Second); err != nil {
					testing.ContextLog(ctx, "Failed to sleep while recovering from a panic: ", err)
				}
			}
		}(ctx)
		f(ctx)
	}()
}
