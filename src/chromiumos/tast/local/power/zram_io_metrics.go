// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"os"
	"strconv"
	"strings"

	"chromiumos/tast/common/perf"
	"chromiumos/tast/errors"
	"chromiumos/tast/testing"
)

// ZramIOMetrics records the number of I/O processed by the zram.
type ZramIOMetrics struct {
	hasZram      bool
	metrics      map[string]perf.Metric
	intervalName string
}

// Assert that ZramIOMetrics can be used in perf.Timeline.
var _ perf.TimelineDatasource = &ZramIOMetrics{}

// Number of zram device file stats.
const zramStatLength = 17

// zramIOStats reads zram I/O number statistics about the block device zram0.
// The stat file consists of a single line of text containing 17 decimal values separated by spaces.
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
// This function returns a map that only contains:
// ["read":readI/Os, "write":writeI/Os, "flight":in_flight, "discard":discardI/Os, "flush":flushI/Os]
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
		"read":    0,  // read I/O index
		"write":   4,  // write I/O index
		"flight":  8,  // in flight I/O index
		"discard": 11, // discard I/O index
		"flush":   15, // flush I/O index
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
	const zramDevPath = "/dev/zram0"
	// Check if the device has zram.
	if f, err := os.Stat(zramDevPath); err == nil {
		// Comparing the FileMode to determine if the device has zram.
		// See https://pkg.go.dev/os#FileMode for details.
		z.hasZram = (f.Mode() & os.ModeDevice) != 0
	}

	if z.hasZram {
		// Number of read I/0s processed.
		z.metrics["read"] = perf.Metric{Name: "zram_read_IOs", Unit: "requests", Direction: perf.SmallerIsBetter, Multiple: true}
		// Number of write I/0s processed.
		z.metrics["write"] = perf.Metric{Name: "zram_write_IOs", Unit: "requests", Direction: perf.SmallerIsBetter, Multiple: true}
		// Number of discard I/0s processed.
		z.metrics["discard"] = perf.Metric{Name: "zram_discard_IOs", Unit: "requests", Direction: perf.SmallerIsBetter, Multiple: true}
		// Number of flush I/0s processed.
		z.metrics["flush"] = perf.Metric{Name: "zram_flush_IOs", Unit: "requests", Direction: perf.SmallerIsBetter, Multiple: true}
		// Number of I/0s currently in flight.
		z.metrics["flight"] = perf.Metric{Name: "zram_IOs_in_flight", Unit: "requests", Direction: perf.SmallerIsBetter, Multiple: true}
	}
	return nil
}

// Start logs the start of zram IO stats tracker.
// This function is required by perf.Timeline.
func (z *ZramIOMetrics) Start(ctx context.Context) error {
	if z.hasZram {
		testing.ContextLog(ctx, "Start tracking zram IO stats")
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
	if z.hasZram {
		testing.ContextLog(ctx, "Stop tracking zram IO stats")
	}
	return nil
}
