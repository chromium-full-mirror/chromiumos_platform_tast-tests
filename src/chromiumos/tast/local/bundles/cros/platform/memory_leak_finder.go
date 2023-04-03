// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"context"
	"time"

	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/memory/mempressure"
	"chromiumos/tast/testing"
)

type memoryLeakFinderParams struct {
	bt browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         MemoryLeakFinder,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Open tabs until discard several times; comparing the max tabs each time",
		Contacts:     []string{"chromeos-memory@google.com", "dianders@google.com"},
		BugComponent: "b:167286",
		Attr:         []string{"group:crosbolt", "crosbolt_memory_nightly"},
		Timeout:      180 * time.Minute,
		Data: []string{
			mempressure.CompressibleData,
			mempressure.WPRArchiveName,
		},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Val: memoryLeakFinderParams{bt: browser.TypeAsh},
		}, {
			Name:              "lacros",
			Val:               memoryLeakFinderParams{bt: browser.TypeLacros},
			ExtraSoftwareDeps: []string{"lacros"},
		}},
	})
}

// MemoryLeakFinder is the main test function.
func MemoryLeakFinder(ctx context.Context, s *testing.State) {
	bt := s.Param().(memoryLeakFinderParams).bt

	testEnv, err := mempressure.NewTestEnv(ctx, s.OutDir(), false, false, bt, s.DataPath(mempressure.WPRArchiveName))
	if err != nil {
		s.Fatal("Failed creating the test environment: ", err)
	}
	defer testEnv.Close(ctx)

	p := &mempressure.RunParameters{
		PageFilePath:             s.DataPath(mempressure.CompressibleData),
		PageFileCompressionRatio: 0.40,
		OpenCloseRepeatCount:     5,
	}

	if err := mempressure.Run(ctx, s.OutDir(), testEnv.Browser(), testEnv.ARC(), p); err != nil {
		s.Fatal("Run failed: ", err)
	}
}
