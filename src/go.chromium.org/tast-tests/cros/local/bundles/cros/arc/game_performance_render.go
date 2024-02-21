// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/gameperformance"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         GamePerformanceRender,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Captures set of performance metrics for the render and upload it to the server",
		Contacts:     []string{"arc-performance@google.com", "khmel@chromium.org", "skuhne@chromium.org"},
		// ChromeOS > Software > ARC++ > Performance
		BugComponent: "b:168382",
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{"chrome"},
		Data:         []string{"ArcGamePerformanceTest.apk"},
		Timeout:      30 * time.Minute,
		Params: []testing.Param{{
			Fixture:           "arcBooted",
			ExtraSoftwareDeps: []string{"android_container"},
		}, {
			Name:              "vm",
			Fixture:           "arcBooted",
			ExtraSoftwareDeps: []string{"android_vm"},
		}, {
			Name:              "vm_pvsched",
			BugComponent:      "b:167279",
			Fixture:           "arcBootedWithPvSchedEnabled",
			ExtraSoftwareDeps: []string{"android_vm"},
			ExtraHardwareDeps: hwdep.D(hwdep.HasParavirtSchedControl()),
		}},
	})
}

func GamePerformanceRender(ctx context.Context, s *testing.State) {
	gameperformance.RunTest(ctx, s, "RenderTest")
}
