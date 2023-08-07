// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast-tests/cros/local/memory/kernelmeter"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         KernelMemory,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that no errors occur while examining graphics memory usage",
		Attr:         []string{"group:mainline", "group:graphics", "graphics_nightly"},
		HardwareDeps: hwdep.D(hwdep.CPUSocFamily("intel")),
		BugComponent: "b:995569", // ChromeOS > Platform > Graphics > GPU
		SoftwareDeps: []string{"no_qemu"},
		Contacts: []string{
			"chromeos-gfx@google.com",
			"syedfaaiz@google.com",
		},
		Fixture: "chromeGraphics",
		Timeout: 2 * time.Minute,
	})
}
func KernelMemory(ctx context.Context, s *testing.State) {
	// Make sure we can access memory in /proc/meminfo.
	outMem, err := kernelmeter.ReadMemInfo()
	if err != nil {
		s.Fatal("Error encountered while processing /proc/meminfo : ", err)
	}
	s.Log("/proc/meminfo :", outMem)
	/* We want to give this test something well-defined to measure.
	 * For now that will be the CrOS login-screen memory use so we put
	 * the test to sleep for 10 seconds to ensure the state is stable and we
	 * are logged in.
	 */
	bytesUsed, err := graphics.GetSysfsMemory(ctx)
	if err != nil {
		s.Fatal("An error was encountered : ", err)
	}
	testing.ContextLogf(ctx, "Total memory usage was %v bytes ", bytesUsed)
	pv := perf.NewValues()
	pv.Set(perf.Metric{
		Name:      "gpu_bytes_used",
		Unit:      "bytes",
		Direction: perf.BiggerIsBetter,
	}, float64(bytesUsed))
	if err := pv.Save(s.OutDir()); err != nil {
		s.Error("Failed saving perf data: ", err)
	}
}
