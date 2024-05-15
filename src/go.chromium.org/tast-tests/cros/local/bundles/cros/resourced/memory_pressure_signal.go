// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package resourced

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/resourced"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MemoryPressureSignal,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks resourced memory pressure signal",
		Contacts:     []string{"chromeos-memory@google.com", "vovoy@chromium.org"},
		BugComponent: "b:167286", // ChromeOS > Platform > System > Memory Management
		Attr:         []string{"group:mainline"},
		Timeout:      2 * time.Minute,
	})
}

func MemoryPressureSignal(ctx context.Context, s *testing.State) {
	rm, err := resourced.NewClient(ctx)
	if err != nil {
		s.Fatal("Failed to create Resource Manager client: ", err)
	}

	// Check MemoryPressureChrome signal is sent.
	ctxWatcher, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	watcher, err := rm.NewChromePressureWatcher(ctxWatcher)
	if err != nil {
		s.Fatal("Failed to create PressureWatcher: ", err)
	}
	defer watcher.Close(ctx)

	select {
	case sig := <-watcher.Signals:
		testing.ContextLogf(ctx, "Got MemoryPressureChrome signal, level: %d, delta: %d", sig.Level, sig.Delta)
	case <-ctxWatcher.Done():
		s.Fatal("Didn't get MemoryPressureChrome signal: ", err)
	}
}
