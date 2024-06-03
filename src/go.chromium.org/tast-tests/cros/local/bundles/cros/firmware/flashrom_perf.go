// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/flashrom"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast/core/testing"
)

type params struct {
	regions  []string
	deadline float64
}

func init() {
	testing.AddTest(&testing.Test{
		Func: FlashromPerf,
		Desc: "Flashrom SPI flash E2E tests",
		Contacts: []string{
			"chromeos-faft@google.com",
			"roccochen@chromium.org", // CrOS Flashrom Maintainer
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// b:238260020 - disable aged (>1y) unpromoted informational tests
		// Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"flashrom"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{{
			Val: params{
				regions:  nil, // nil implies a full read.
				deadline: 21000,
			},
		}, {
			Name: "fmap",
			Val: params{
				regions:  []string{"FMAP"},
				deadline: 2000,
			},
		}, {
			Name: "gbb",
			Val: params{
				regions:  []string{"GBB"},
				deadline: 2700,
			},
		}, {
			Name: "rw_vpd",
			Val: params{
				regions:  []string{"RW_VPD"},
				deadline: 2300,
			},
		}, {
			Name: "rw_elog",
			Val: params{
				regions:  []string{"RW_ELOG"},
				deadline: 2300,
			},
		}, {
			Name: "coreboot",
			Val: params{
				regions:  []string{"COREBOOT"},
				deadline: 6000,
			},
		}},
	})
}

// FlashromPerf runs the flashrom utility to verify various expected behaviour
// is maintained.  The function times Flashrom total execution time for a
// complete probe and read of a given region of the SPI flash.
func FlashromPerf(ctx context.Context, s *testing.State) {
	readTime := perf.Metric{
		Name:      "flashrom_region_read_time",
		Unit:      "ms",
		Direction: perf.SmallerIsBetter,
	}
	p := s.Param().(params)

	perf := perf.NewValues()
	duration := testFlashromReadTime(ctx, s, p.regions)
	perf.Set(readTime, duration)

	const lowerBound = 100 // sub-process execution should take some time.

	if err := perf.Save(s.OutDir()); err != nil {
		s.Error("Failed saving perf data: ", err)
	}
	if duration <= lowerBound || duration >= p.deadline {
		s.Errorf("Flashrom read time for regions %q was outside time-bounds, %v < expected < %v ms, got = %v ms",
			strings.Join(p.regions, ", "), lowerBound, p.deadline, duration)
	}
}

func testFlashromReadTime(ctx context.Context, s *testing.State, regions []string) float64 {
	flashromStart := time.Now()

	opTempFile, err := ioutil.TempFile("", "dump_"+strings.Join(regions, "")+"_*.bin")
	if err != nil {
		s.Fatal("Failed creating temp file: ", err)
	}
	defer os.Remove(opTempFile.Name())

	var flashromConfig flashrom.Config
	flashromInstance, ctx, shutdown, _, err := flashromConfig.
		FlashromInit(flashrom.VerbosityInfo).
		ProgrammerInit(flashrom.ProgrammerHost, "").
		Probe(ctx)

	// Full shutdown is a symmetric operation to FlashromInit and needs to be called
	// regardless of whether err is nil (to do a cleanup).
	defer func() {
		if err := shutdown(); err != nil {
			s.Error("Failed to shutdown flashromInstance: ", err)
		}
	}()

	if err != nil {
		s.Fatal("Flashrom probe failed, unable to build flashrom instance: ", err)
	}

	out, err := flashromInstance.Read(ctx, opTempFile.Name(), regions)

	if err != nil {
		s.Logf("Flashrom read output: %s", string(out))
		s.Fatal("Flashrom read failed: ", err)
	}

	flashromElapsed := time.Since(flashromStart)

	return float64(flashromElapsed.Milliseconds())
}
