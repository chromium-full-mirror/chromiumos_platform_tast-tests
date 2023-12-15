// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kernel

import (
	"context"
	"path/filepath"

	"go.chromium.org/tast/core/testing"
)

type fileExistParams struct {
	glob     string
	minCount int
	maxCount int
}

func init() {
	testing.AddTest(&testing.Test{
		Func: FileExist,
		Desc: "Ensures certain files exist",
		Contacts: []string{
			"baseos-perf@google.com",
			"dianders@chromium.org",
			"joelaf@chromium.org",
		},
		BugComponent: "b:167279",
		Attr:         []string{"group:mainline"},

		Params: []testing.Param{{
			// This test exists because `latency_sensitive` is added
			// by a CHROMIUM patch (which itself is based on some
			// ANDROID patches) and nothing else in the system has
			// an obvious failure if that patch is missing. However,
			// without that patch certain tests will fail for
			// very-difficult-to-debug reasons.
			//
			// If/when the CHROMIUM patch is replaced with an
			// upstream solution to accomplish similar goals then
			// we'd want to drop this test.
			Name: "latency_sensitive",
			Val: fileExistParams{
				glob: "/proc/self/task/*/latency_sensitive",

				// All of the tast threads should have the
				// latency_sensitive attribute. We really only
				// need to see one of them as a proof that the
				// patch is there, so set no maximum since we
				// don't care how many threads "golang" creates
				// to run a tast test.
				minCount: 1,
				maxCount: -1,
			},

			// Technically, non-ARM systems could have the
			// `latency_sensitive` attribute if they have
			// UCLAMP_TASK enabled. However, right now we only have
			// it enabled on ARM.
			ExtraSoftwareDeps: []string{"arm"},
		}},
	})
}

// FileExist checks that a parameterized file exists.
func FileExist(ctx context.Context, s *testing.State) {
	param := s.Param().(fileExistParams)

	paths, err := filepath.Glob(param.glob)
	if err != nil {
		s.Fatalf("Failed to glob for %s: %v", param.glob, err)
	}
	if len(paths) < param.minCount {
		s.Fatalf("Glob for %s didn't match min (%d < %d)",
			param.glob, len(paths), param.minCount)
	}
	if param.maxCount != -1 && len(paths) > param.maxCount {
		s.Fatalf("Glob for %s didn't match max (%d > %d)",
			param.glob, len(paths), param.maxCount)
	}

	for _, path := range paths {
		testing.ContextLogf(ctx, "Matched %s", path)
	}
}
