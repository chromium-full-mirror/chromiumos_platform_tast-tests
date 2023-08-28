// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package diskstats

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast/core/errors"
)

// DiskStatMap is a map from a disk to the int fields of `/sys/block/vd*/stat` of the disk.
type DiskStatMap map[disk][]int

// disk represents a virtio-blk disk attached to ARCVM.
type disk string

const (
	diskSystem disk = "system"
	diskVendor disk = "vendor"
	diskData   disk = "data"
)

// nodeName returns the device node name of the given disk in ARCVM.
func (d disk) nodeName() string {
	switch d {
	case diskSystem:
		return "vda"
	case diskVendor:
		return "vdb"
	case diskData:
		return "vde"
	default:
		return "unknown"
	}
}

func (d disk) isReadonly() bool {
	return d != diskData
}

// diskStat represents a disk stat retrieved from a `/sys/block/vd*/stat`.
type diskStat struct {
	// ios is the number of IOs issued per operation type.
	ios map[operation]int
	// size is the total IO size in kilobytes per operation type.
	size map[operation]int
	// ticks is the total IO time per operation type.
	ticks map[operation]time.Duration
	// totalWait is the total IO time of all operations that corresponds
	// to io_ticks of `/sys/block/vd*/stat`.
	totalWait time.Duration
	// diskActive is the total disk active time that corresponds to
	// `time_in_queue` of `/sys/block/vd*/stat`.
	diskActive time.Duration
}

// operation represents the type of IO operations.
type operation int

const (
	opRead operation = iota
	opWrite
	opFlush
	opDiscard
)

// String implements the String() interface of value.
func (o operation) String() string {
	switch o {
	case opRead:
		return "read"
	case opWrite:
		return "write"
	case opFlush:
		return "flush"
	case opDiscard:
		return "discard"
	default:
		return "unknown_io"
	}
}

func newDiskStat(stats []int) *diskStat {
	// See https://docs.kernel.org/block/stat.html for each field's description.
	return &diskStat{
		ios: map[operation]int{
			opRead:    stats[0],
			opWrite:   stats[4],
			opDiscard: stats[11],
			opFlush:   stats[15],
		},
		size: map[operation]int{
			opRead:    sectorToKiloBytes(stats[2]),
			opWrite:   sectorToKiloBytes(stats[6]),
			opDiscard: sectorToKiloBytes(stats[13]),
		},
		ticks: map[operation]time.Duration{
			opRead:    time.Millisecond * time.Duration(stats[3]),
			opWrite:   time.Millisecond * time.Duration(stats[7]),
			opDiscard: time.Millisecond * time.Duration(stats[14]),
			opFlush:   time.Millisecond * time.Duration(stats[16]),
		},
		totalWait:  time.Millisecond * time.Duration(stats[10]),
		diskActive: time.Millisecond * time.Duration(stats[9]),
	}
}

func sectorToKiloBytes(sector int) int {
	return sector * 512 / 1024 // 1 sector = 512 byte.
}

// CollectDiskStats returns DiskStatMap parsed from `/sys/block/vd*/stat` of all disks.
func CollectDiskStats(ctx context.Context) (DiskStatMap, error) {
	result := map[disk][]int{}
	if isVMEnabled, err := arc.VMEnabled(); err != nil {
		return nil, errors.Wrap(err, "failed to check if vm is enabled")
	} else if !isVMEnabled {
		return result, nil
	}

	disks := []disk{diskSystem, diskVendor}
	if isDataAvailable, err := arc.IsVirtioBlkDataEnabled(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to check if a data is a block")
	} else if isDataAvailable {
		disks = append(disks, diskData)
	}

	for _, d := range disks {
		// We use android-sh by BootstrapCommand instead of adb so that the
		// caller can get the disk stat before setting up adb. Setting adb up
		// takes reasonable seconds and involves non-negligible disk workload.
		path := "/sys/block/" + d.nodeName() + "/stat"
		cmd := arc.BootstrapCommand(ctx, "/system/bin/cat", path)
		statFile, err := cmd.Output(testexec.DumpLogOnError)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read a stat file %s", path)
		}
		statStrings := strings.Fields(string(statFile))
		var stats []int
		for _, statStr := range statStrings {
			stat, err := strconv.Atoi(statStr)
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse a disk stat field")
			}
			stats = append(stats, stat)
		}

		result[d] = stats
	}

	return result, nil
}

// AppendPerfMetrics appends the disk stats calculated from the StatMap to perfValues.
func (m DiskStatMap) AppendPerfMetrics(perfValues *perf.Values, suffix string) {
	for d, stats := range m {
		diskStat := newDiskStat(stats)
		for op := opRead; op <= opDiscard; op++ {
			if d.isReadonly() && op != opRead {
				continue
			}
			perfValues.Append(perf.Metric{
				Name:      fmt.Sprintf("%s_ios_%s%s", op, d, suffix),
				Unit:      "count",
				Direction: perf.SmallerIsBetter,
				Multiple:  true,
			}, float64(diskStat.ios[op]))
			if op != opFlush { // Flush doesn't have size.
				perfValues.Append(perf.Metric{
					Name:      fmt.Sprintf("%s_size_%s%s", op, d, suffix),
					Unit:      "kilobytes",
					Direction: perf.SmallerIsBetter,
					Multiple:  true,
				}, float64(diskStat.size[op]))
			}
			perfValues.Append(perf.Metric{
				Name:      fmt.Sprintf("%s_ticks_%s%s", op, d, suffix),
				Unit:      "milliseconds",
				Direction: perf.SmallerIsBetter,
				Multiple:  true,
			}, float64(diskStat.ticks[op].Milliseconds()))
		}
		perfValues.Append(perf.Metric{
			Name:      fmt.Sprintf("total_wait_%s%s", d, suffix),
			Unit:      "milliseconds",
			Direction: perf.SmallerIsBetter,
			Multiple:  true,
		}, float64(diskStat.totalWait.Milliseconds()))
		perfValues.Append(perf.Metric{
			Name:      fmt.Sprintf("disk_active_%s%s", d, suffix),
			Unit:      "milliseconds",
			Direction: perf.SmallerIsBetter,
			Multiple:  true,
		}, float64(diskStat.diskActive.Milliseconds()))
	}
}
