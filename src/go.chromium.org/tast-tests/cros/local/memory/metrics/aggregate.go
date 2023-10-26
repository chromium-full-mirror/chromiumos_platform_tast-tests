// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package metrics

import (
	"fmt"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/memory"
	memoryarc "go.chromium.org/tast-tests/cros/local/memory/arc"
)

// ReportSummaryMetrics combines metrics taken from various sources, computes
// and logs aggregate metrics.
func ReportSummaryMetrics(vmEnabled bool, hostSummary *memory.HostSummary, vmSummary *memoryarc.VMSummary, zramSummary *memory.ZramSummary, p *perf.Values, suffix string) {

	totalCachedVMKernel := vmSummary.CachedKernel
	totalCachedVMAll := vmSummary.CachedKernel + vmSummary.CachedPss
	var totalCachedVMOnly uint64
	if vmEnabled {
		totalCachedVMKernel += hostSummary.HostCached
		totalCachedVMOnly = totalCachedVMAll
		totalCachedVMAll += hostSummary.HostCached
	}
	total := float64(hostSummary.MemTotal - hostSummary.MemFree - totalCachedVMKernel)
	totalNoCache := float64(hostSummary.MemTotal - hostSummary.MemFree - totalCachedVMAll)

	// This may be negative.
	zramSavings := int64(zramSummary.OrigDataSize) - int64(zramSummary.MemUsedTotal)

	p.Set(
		perf.Metric{
			Name:      fmt.Sprintf("total_memory_used%s", suffix),
			Unit:      "KiB",
			Direction: perf.SmallerIsBetter,
		},
		total,
	)

	p.Set(
		perf.Metric{
			Name:      fmt.Sprintf("total_memory_nocache%s", suffix),
			Unit:      "KiB",
			Direction: perf.SmallerIsBetter,
		},
		totalNoCache,
	)

	p.Set(
		perf.Metric{
			Name:      fmt.Sprintf("total_memory_nocache_noswap%s", suffix),
			Unit:      "KiB",
			Direction: perf.SmallerIsBetter,
		},
		totalNoCache+float64(zramSavings),
	)

	p.Set(
		perf.Metric{
			Name:      fmt.Sprintf("linux_memory_used%s", suffix),
			Unit:      "KiB",
			Direction: perf.SmallerIsBetter,
		},
		float64(hostSummary.LinuxUsed),
	)

	p.Set(
		perf.Metric{
			Name:      fmt.Sprintf("linux_memory_used_novmcache%s", suffix),
			Unit:      "KiB",
			Direction: perf.SmallerIsBetter,
		},
		float64(hostSummary.LinuxUsed-totalCachedVMOnly),
	)

	p.Set(
		perf.Metric{
			Name:      fmt.Sprintf("linux_memory_used_novmcache_noswap%s", suffix),
			Unit:      "KiB",
			Direction: perf.SmallerIsBetter,
		},
		float64(hostSummary.LinuxUsed-totalCachedVMOnly)+float64(zramSavings),
	)

	p.Set(
		perf.Metric{
			Name:      fmt.Sprintf("total_memory_used_noswap%s", suffix),
			Unit:      "KiB",
			Direction: perf.SmallerIsBetter,
		},
		total+float64(zramSavings),
	)

	if crosvmArc := hostSummary.CategoryMetrics["crosvm_arcvm"]; crosvmArc != nil {
		p.Set(
			perf.Metric{
				Name:      fmt.Sprintf("crosvm_overhead%s", suffix),
				Unit:      "KiB",
				Direction: perf.SmallerIsBetter,
			},
			float64(crosvmArc.Pss+crosvmArc.PssSwap-crosvmArc.PssGuest),
		)
	}
}
