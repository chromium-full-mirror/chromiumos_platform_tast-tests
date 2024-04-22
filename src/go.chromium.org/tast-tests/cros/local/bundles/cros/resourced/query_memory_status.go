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
		Func:         QueryMemoryStatus,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks resourced querying memory status",
		Contacts:     []string{"chromeos-memory@google.com", "vovoy@chromium.org"},
		BugComponent: "b:167286", // ChromeOS > Platform > System > Memory Management
		Attr:         []string{"group:mainline"},
		Timeout:      2 * time.Minute,
	})
}

func QueryMemoryStatus(ctx context.Context, s *testing.State) {
	rm, err := resourced.NewClient(ctx)
	if err != nil {
		s.Fatal("Failed to create Resource Manager client: ", err)
	}

	availableKB, err := rm.AvailableMemoryKB(ctx)
	if err != nil {
		s.Fatal("Failed to query available memory: ", err)
	}
	testing.ContextLog(ctx, "GetAvailableMemoryKB returns: ", availableKB)

	foregroundAvailableKB, err := rm.ForegroundAvailableMemoryKB(ctx)
	if err != nil {
		s.Fatal("Failed to query foreground available memory: ", err)
	}
	testing.ContextLog(ctx, "GetForegroundAvailableMemoryKB returns: ", foregroundAvailableKB)

	m, err := rm.MemoryMarginsKB(ctx)
	if err != nil {
		s.Fatal("Failed to query memory margins: ", err)
	}
	testing.ContextLog(ctx, "GetMemoryMarginsKB returns, critical: ", m.CriticalKB, ", moderate: ", m.ModerateKB)

	componentMemoryMargins, err := rm.ComponentMemoryMarginsKB(ctx)
	if err != nil {
		s.Fatal("Failed to query component memory margins: ", err)
	}

	testing.ContextLogf(ctx, "GetComponentMemoryMarginsKB returns %+v", componentMemoryMargins)
}
