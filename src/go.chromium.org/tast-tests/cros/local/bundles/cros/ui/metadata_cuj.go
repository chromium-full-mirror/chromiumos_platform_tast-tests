// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/cuj"

	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: MetadataCUJ,
		Desc: "Uploads metadata for CUJ tests",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"ramsaroop@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Attr:         []string{"group:cuj"},
		Timeout:      time.Minute,
		HardwareDeps: hwdep.D(hwdep.Model("fleex")),
	})
}

// MetadataCUJ uploads metadata for tests that the
// cros-sw-perf team monitors.
func MetadataCUJ(ctx context.Context, s *testing.State) {
	if err := cuj.GenerateMetadataFile(ctx); err != nil {
		s.Fatal("Failed to upload metadata: ", err)
	}
}
