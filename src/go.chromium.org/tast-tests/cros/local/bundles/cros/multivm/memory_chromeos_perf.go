// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package multivm

import (
	"context"
	"fmt"
	"sort"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/crostini"
	"go.chromium.org/tast-tests/cros/local/memory"
	"go.chromium.org/tast-tests/cros/local/memory/metrics"
	"go.chromium.org/tast-tests/cros/local/multivm"
	"go.chromium.org/tast-tests/cros/local/resourced"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MemoryChromeOSPerf,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "How much memory can we allocate before each ChromeOS memory pressure level",
		Contacts: []string{
			"arcvm-eng@google.com",
			"cwd@chromium.org",
		},
		BugComponent: "b:885256", // ChromeOS > Platform > System
		Attr:         []string{"group:crosbolt", "crosbolt_nightly"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Pre: multivm.NoVMStarted(),
		}, {
			Name:              "with_bg_arc",
			ExtraSoftwareDeps: []string{"arc"},
			Pre:               multivm.ArcStarted(),
		}, {
			Name:              "with_bg_crostini",
			ExtraData:         []string{crostini.GetContainerMetadataArtifact("bullseye", false), crostini.GetContainerRootfsArtifact("bullseye", false)},
			ExtraSoftwareDeps: []string{"vm_host"},
			ExtraHardwareDeps: crostini.CrostiniStable,
			Pre:               multivm.CrostiniStarted(),
		}, {
			Name: "lacros",
			Pre:  multivm.NoVMLacrosStarted(),
		}, {
			Name:              "with_bg_arc_lacros",
			ExtraSoftwareDeps: []string{"arc", "lacros"},
			Pre:               multivm.ArcLacrosStarted(),
		}, {
			Name:              "with_bg_crostini_lacros",
			ExtraData:         []string{crostini.GetContainerMetadataArtifact("bullseye", false), crostini.GetContainerRootfsArtifact("bullseye", false)},
			ExtraSoftwareDeps: []string{"vm_host", "lacros"},
			ExtraHardwareDeps: crostini.CrostiniStable,
			Pre:               multivm.CrostiniLacrosStarted(),
		}},
		Timeout: 10 * time.Minute,
	})
}

func setAllocatedMetrics(p *perf.Values, allocated []uint64, suffix string) {
	allocatedMetric := perf.Metric{Name: "allocated" + suffix, Unit: "MiB", Direction: perf.BiggerIsBetter, Multiple: true}
	allocatedP90Metric := perf.Metric{Name: fmt.Sprintf("allocated%s_p90", suffix), Unit: "MiB", Direction: perf.BiggerIsBetter}
	var allocatedMiB []float64
	for _, a := range allocated {
		aMiB := float64(a) / float64(memory.MiB)
		p.Append(allocatedMetric, aMiB)
		allocatedMiB = append(allocatedMiB, aMiB)
	}
	sort.Float64s(allocatedMiB)
	p90Index := int(float64(len(allocatedMiB))*0.9) - 1
	p.Set(allocatedP90Metric, allocatedMiB[p90Index])
}

func MemoryChromeOSPerf(ctx context.Context, s *testing.State) {
	pre := s.PreValue().(*multivm.PreData)
	arc := multivm.ARCFromPre(pre)
	p := perf.NewValues()
	rm, err := resourced.NewClient(ctx)
	if err != nil {
		s.Fatal("Failed to create Resource Manager client: ", err)
	}

	margins, err := rm.MemoryMarginsKB(ctx)
	if err != nil {
		s.Fatal("Failed to get memory margins: ", err)
	}
	p.Set(perf.Metric{Name: "critical_margin", Unit: "KiB"}, float64(margins.CriticalKB))
	p.Set(perf.Metric{Name: "moderate_margin", Unit: "KiB"}, float64(margins.ModerateKB))

	c := memory.NewChromeOSAllocator()
	defer c.FreeAll()

	const epsilon = 5 * memory.MiB // We want to be consistently under the critical margin, so make the target available just inside.
	basemem, err := metrics.NewBaseMemoryStats(ctx, arc)
	if err != nil {
		s.Fatal("Failed to retrieve base memory stats: ", err)
	}

	// TODO: wait for system to cool down?

	// How many seconds to spend in each allocation phase.
	const phaseSeconds = 60

	// No memory pressure. Wait for things to settle.
	s.Log("Waiting with no memory pressure")
	if err := testing.Sleep(ctx, phaseSeconds*time.Second); err != nil {
		s.Fatal("Failed to sleep with no memory pressure: ", err)
	}
	s.Log("Logging idle metrics")
	if err := metrics.LogMemoryStats(ctx, basemem, arc, p, s.OutDir(), "_idle"); err != nil {
		s.Error("Failed to collect memory metrics: ", err)
	}
	if err := basemem.Reset(); err != nil {
		s.Error("Failed to reset memory metrics post idle: ", err)
	}

	// Moderate memory pressure.
	s.Log("Allocating to moderate memory pressure")
	if allocatedModerate, err := c.AllocateUntil(
		ctx,
		rm,
		time.Second,
		phaseSeconds,
		margins.ModerateKB*memory.KiB-epsilon,
	); err != nil {
		// NB: on some devices we are at moderate memory pressure when idle, so
		// continue the test, but skip logging the allocated metric. If it's a real
		// failure, we will error again when allocating to the critical margin.
		s.Log("Warning: failed to allocate to moderate margin: ", err)
	} else {
		setAllocatedMetrics(p, allocatedModerate, "_moderate")
	}
	s.Log("Logging moderate metrics")
	if err := metrics.LogMemoryStats(ctx, basemem, arc, p, s.OutDir(), "_moderate"); err != nil {
		s.Error("Failed to collect memory metrics: ", err)
	}
	if err := basemem.Reset(); err != nil {
		s.Error("Failed to reset memory metrics post moderate: ", err)
	}

	// Critical memory pressure.
	s.Log("Allocating to critical memory pressure")
	allocatedCritical, err := c.AllocateUntil(
		ctx,
		rm,
		time.Second,
		phaseSeconds,
		margins.CriticalKB*memory.KiB-epsilon,
	)
	if err != nil {
		s.Fatal("Failed to allocate to critical margin: ", err)
	}
	s.Log("Logging critical metrics")
	if err := metrics.LogMemoryStats(ctx, basemem, arc, p, s.OutDir(), "_critical"); err != nil {
		s.Error("Failed to collect memory metrics: ", err)
	}
	setAllocatedMetrics(p, allocatedCritical, "_critical")

	// Clean up.
	if _, err := c.FreeAll(); err != nil {
		s.Fatal("Failed to free allocated memory: ")
	}
	if err := p.Save(s.OutDir()); err != nil {
		s.Error("Failed saving perf data: ", err)
	}
}
