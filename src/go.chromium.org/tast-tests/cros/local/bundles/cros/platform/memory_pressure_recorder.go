// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/memory/mempressure"
	"go.chromium.org/tast-tests/cros/local/wpr"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MemoryPressureRecorder,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Record a WPR archive for platform.MemoryPressure",
		Contacts:     []string{"chromeos-memory@google.com"},
		BugComponent: "b:167286",
		Timeout:      60 * time.Minute,
		SoftwareDeps: []string{"chrome"},
		Pre:          wpr.RecordMode("/tmp/archive.wprgo"),
	})
}

// MemoryPressureRecorder runs WPR in recording mode.
func MemoryPressureRecorder(ctx context.Context, s *testing.State) {
	p := &mempressure.RunParameters{
		Mode: wpr.Record,
	}

	if err := mempressure.Run(ctx, s.OutDir(), s.PreValue().(*chrome.Chrome).Browser(), nil, p); err != nil {
		s.Fatal("Run failed: ", err)
	}
}
