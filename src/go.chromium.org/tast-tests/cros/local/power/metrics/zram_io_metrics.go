// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package metrics

import (
	"context"
	"os"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/perf"
	cp "go.chromium.org/tast-tests/cros/common/power"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// ZramIOMetrics records the number of I/O processed by the zram.
type ZramIOMetrics struct {
	hasZram      bool
	metrics      map[string]perf.Metric
	intervalName string
	startRead    float64
	startWrite   float64
	prefix       string
}

// Assert that ZramIOMetrics can be used in perf.Timeline.
var _ perf.TimelineDatasource = &ZramIOMetrics{}

// zramIOStats reads zram I/O number statistics about the block device zram0.
// The stat file (Kernel v6.2.0) consists of a single line of text containing 17 decimal values separated by spaces.
// The fields are:
// 0. read I/Os
// 1. read merges
// 2. read sectors
// 3. read ticks
// 4. write I/Os
// 5. write merges
// 6. write sectors
// 7. write ticks
// 8. in_flight
// 9. io_ticks
// 10. time_in_queue
// 11. discard I/Os
// 12. discard merges
// 13. discard sectors
// 14. discard ticks
// 15. flush I/Os
// 16. flush ticks
// See https://www.kernel.org/doc/html/latest/block/stat.html for details.
// Older versions of kernel may only contain the first 15 or 11 values.
// https://git.kernel.org/pub/scm/linux/kernel/git/stable/linux.git/tree/Documentation/block/stat.rst?h=v5.4.231
// https://git.kernel.org/pub/scm/linux/kernel/git/stable/linux.git/tree/Documentation/block/stat.txt?h=v4.14.305
//
// This function returns a map that only contains:
// ["read":readI/Os, "write":writeI/Os, "flight":in_flight]
// where key is the name of metrics as string, aligned with keys of ZramIOMetrics metrics map
// and value is the read result.
func zramIOStats(ctx context.Context) (map[string]float64, error) {
	const zramStatPath = "/sys/block/zram0/stat"
	out, err := os.ReadFile(zramStatPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read zram IO stats")
	}
	statsRaw := strings.Fields(string(out))

	// zramStatIdx stores the index postiion of each stat in zram stat file's read.
	zramStatIdx := map[string]int{
		"read":   0, // read I/O index
		"write":  4, // write I/O index
		"flight": 8, // in flight I/O index
	}
	statsRead := make(map[string]float64)

	for key, value := range zramStatIdx {
		parseRead, err := strconv.ParseFloat(statsRaw[value], 64)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse %s: %s in zram I/O stat", key, statsRaw[value])
		}
		statsRead[key] = parseRead
	}
	return statsRead, nil
}

// NewZramIOMetrics creates the struct to store Zram IO metrics.
func NewZramIOMetrics() *ZramIOMetrics {
	newMetrics := &ZramIOMetrics{
		hasZram:      false,
		metrics:      make(map[string]perf.Metric),
		intervalName: "",
	}
	return newMetrics
}

// Setup creates the metric.
func (z *ZramIOMetrics) Setup(ctx context.Context, prefix, intervalName string) error {
	z.prefix = prefix

	const zramDevPath = "/dev/zram0"
	// Check if the device has zram.
	if f, err := os.Stat(zramDevPath); err == nil {
		// Comparing the FileMode to determine if the device has zram.
		// See https://pkg.go.dev/os#FileMode for details.
		z.hasZram = (f.Mode() & os.ModeDevice) != 0
	}

	if z.hasZram {
		// Number of read I/Os processed.
		z.metrics["read"] = perf.Metric{
			Name:      prefix + cp.ZramMetricType + "zram_read_IOs",
			Unit:      cp.ZramMetricTypeUnit,
			Direction: perf.SmallerIsBetter,
			Multiple:  true,
			Interval:  intervalName,
		}
		// Number of write I/Os processed.
		z.metrics["write"] = perf.Metric{
			Name:      prefix + cp.ZramMetricType + "zram_write_IOs",
			Unit:      cp.ZramMetricTypeUnit,
			Direction: perf.SmallerIsBetter,
			Multiple:  true,
			Interval:  intervalName,
		}
		// Number of I/Os in flight.
		z.metrics["flight"] = perf.Metric{
			Name:      prefix + cp.ZramMetricType + "zram_IOs_in_flight",
			Unit:      cp.ZramMetricTypeUnit,
			Direction: perf.SmallerIsBetter,
			Multiple:  true,
			Interval:  intervalName,
		}
	}
	return nil
}

// Start logs the start of zram IO stats tracker.
// This function is required by perf.Timeline.
func (z *ZramIOMetrics) Start(ctx context.Context) error {
	if z.hasZram {
		testing.ContextLog(ctx, "Start tracking zram IO stats")

		readResult, err := zramIOStats(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to read complete Zram I/O stats")
		}

		z.startRead = readResult["read"]
		z.startWrite = readResult["write"]
	} else {
		testing.ContextLog(ctx, "The device does not have zram")
	}
	return nil
}

// Snapshot takes one snapshot of zram I/O stats.
func (z *ZramIOMetrics) Snapshot(ctx context.Context, values *perf.Values) error {
	if !z.hasZram {
		return nil
	}
	readResult, err := zramIOStats(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to read complete Zram I/O stats")
	}
	for k, v := range z.metrics {
		values.Append(v, readResult[k])
	}
	return nil
}

// Stop logs the stop of zram IO stats tracker.
// This function is required by perf.Timeline. It does not need to make another snapshot.
func (z *ZramIOMetrics) Stop(ctx context.Context, values *perf.Values) error {
	if !z.hasZram {
		return nil
	}

	testing.ContextLog(ctx, "Stop tracking zram IO stats")

	readResult, err := zramIOStats(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to read complete Zram I/O stats")
	}

	values.Set(perf.Metric{
		Name:      z.prefix + cp.ZramMetricType + "zram_read_IOs_diff",
		Unit:      cp.ZramMetricTypeUnit,
		Direction: perf.SmallerIsBetter,
	}, readResult["read"]-z.startRead)

	values.Set(perf.Metric{
		Name:      z.prefix + cp.ZramMetricType + "zram_write_IOs_diff",
		Unit:      cp.ZramMetricTypeUnit,
		Direction: perf.SmallerIsBetter,
	}, readResult["write"]-z.startWrite)
	return nil
}
