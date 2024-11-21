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
	marginsBefore, err := rm.ComponentMemoryMarginsKB(ctx)
	if err != nil {
		s.Fatal("Failed to query memory margins: ", err)
	}

	var (
		defaultMargins = resourced.MarginsBps{Moderate: 4000, Critical: 1200, CriticalProtected: 600}
		newMargins     = resourced.MarginsBps{Moderate: 8000, Critical: 2400, CriticalProtected: 1200}
	)

	// Set the new memory margins.
	if err = rm.SetMemoryMargins(ctx, newMargins); err != nil {
		s.Fatal("Failed to set memory margins: ", err)
	}

	// Query the new memory margin after setting.
	marginsAfter, err := rm.ComponentMemoryMarginsKB(ctx)
	if err != nil {
		s.Fatal("Failed to query memory margins: ", err)
	}

	// Restore to the default memory margins.
	if err = rm.SetMemoryMargins(ctx, defaultMargins); err != nil {
		s.Fatal("Failed to set memory margins to default: ", err)
	}

	s.Logf("Margins KB before, modereate: %d, critical: %d, critical protected: %d", marginsBefore.ChromeModerateKB, marginsBefore.ChromeCriticalKB, marginsBefore.ChromeCriticalProtectedKB)
	s.Logf("Margins KB after, modereate: %d, critical: %d, critical protected: %d", marginsAfter.ChromeModerateKB, marginsAfter.ChromeCriticalKB, marginsAfter.ChromeCriticalProtectedKB)
}
