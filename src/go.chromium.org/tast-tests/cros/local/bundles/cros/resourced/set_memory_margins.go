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
		Func:         SetMemoryMargins,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks resourced setting memory margins",
		Contacts:     []string{"chromeos-memory@google.com", "vovoy@chromium.org"},
		BugComponent: "b:167286", // ChromeOS > Platform > System > Memory Management
		Attr:         []string{"group:mainline", "informational"},
		Timeout:      2 * time.Minute,
	})
}

func SetMemoryMargins(ctx context.Context, s *testing.State) {
	rm, err := resourced.NewClient(ctx)
	if err != nil {
		s.Fatal("Failed to create Resource Manager client: ", err)
	}

	// Query the original memory margins for comparison.
	marginBefore, err := rm.MemoryMarginsKB(ctx)
	if err != nil {
		s.Fatal("Failed to query memory margins: ", err)
	}

	// Set the new memory margins.
	const (
		defaultCritical uint32 = 520
		defaultModerate uint32 = 4000
		newCritical     uint32 = defaultCritical * 2
		newModerate     uint32 = defaultModerate * 2
	)
	if err = rm.SetMemoryMarginsBps(ctx, newCritical, newModerate); err != nil {
		s.Fatal("Failed to set memory margins: ", err)
	}

	// Query the new memory margin after setting.
	marginAfter, err := rm.MemoryMarginsKB(ctx)
	if err != nil {
		s.Fatal("Failed to query memory margins: ", err)
	}

	// Restore to the default memory margins.
	if err = rm.SetMemoryMarginsBps(ctx, defaultCritical, defaultModerate); err != nil {
		s.Fatal("Failed to set memory margins to default: ", err)
	}

	// The new memory margins should be larger.
	if marginAfter.CriticalKB <= marginBefore.CriticalKB {
		s.Fatalf("Unexpected critical margin after setting, before: %d, after: %d", marginBefore.CriticalKB, marginAfter.CriticalKB)
	}
	if marginAfter.ModerateKB <= marginBefore.ModerateKB {
		s.Fatalf("Unexpected moderate margin after setting, before: %d, after: %d", marginBefore.ModerateKB, marginAfter.ModerateKB)
	}
}
