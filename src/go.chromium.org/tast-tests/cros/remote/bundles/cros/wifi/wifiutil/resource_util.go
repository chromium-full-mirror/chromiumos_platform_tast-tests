// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifiutil

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

// ResourceInfoCounter is a collection of a single process-related data, in the [counter]value format.
type ResourceInfoCounter map[string]int

// ResourceThreshold defines maximum acceptable change thresholds.
type ResourceThreshold ResourceInfoCounter

func (ric ResourceInfoCounter) keysSorted() []string {
	// The data read from map via range comes in a random order.
	// Thus we need to get all keys first, sort them and only then return.
	var ret []string
	for k := range ric {
		ret = append(ret, k)
	}
	sort.Strings(ret)
	return ret
}

type resourceInfoData struct {
	command  string
	counters ResourceInfoCounter
}

// ResourceInfo is a collection of resource data, in the [process][counter]value format.
type ResourceInfo map[string]*resourceInfoData

func (ri ResourceInfo) keysSorted() []string {
	// The data read from map via range comes in a random order.
	// Thus we need to get all keys first, sort them and only then return.
	var ret []string
	for k := range ri {
		ret = append(ret, k)
	}
	sort.Strings(ret)
	return ret
}

// loadMemInfo loads memory-related information into ResourceInfo.
func (ri ResourceInfo) loadMemInfo(ctx context.Context, conn *ssh.Conn, cmdList string) error {
	out, err := conn.CommandContext(ctx, "ps", "--no-headers", "-C", cmdList, "-o", "pid,comm,vsz").Output()
	if err != nil {
		return errors.Wrapf(err, "failed to run ps --no-headers -C %s -o pid,comm,vsz", cmdList)
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines[:len(lines)-1] {
		tokens := strings.Fields(line)
		if len(tokens) < 2 {
			return errors.Errorf("unexpected output of ps command, wanted two values, got %q", line)
		}
		pid := tokens[0] // There's no added value from keeping it as int.
		vsz, err := strconv.Atoi(tokens[2])
		if err != nil {
			return errors.Wrapf(err, "failed to convert %q to int", tokens[2])
		}

		ri[pid] = &resourceInfoData{command: tokens[1], counters: make(ResourceInfoCounter)}
		ri[pid].counters["vsz"] = vsz
	}
	return nil
}

// loadFdNum stores number of opened file descriptors into ResourceInfo.
func (ri ResourceInfo) loadFdNum(ctx context.Context, conn *ssh.Conn) error {
	for _, pid := range ri.pids() {
		out, err := conn.CommandContext(ctx, "lsof", "-p", pid).Output()
		if err != nil {
			return errors.Wrapf(err, "failed to run lsof %s", pid)
		}
		_, ok := ri[pid]
		if !ok {
			return errors.Errorf("pid %s not found", pid)
		}
		ri[pid].counters["fd"] = strings.Count(string(out), "\n") - 1
	}
	return nil
}

// pids returns slice of process IDs stored in the given ResourceInfo.
func (ri ResourceInfo) pids() []string {
	pids := make([]string, 0, len(ri))
	for p := range ri {
		pids = append(pids, p)
	}
	return pids
}

// String returns string representation of the whole ResourceInfo.
// Because ResourceInfo is a map of *pointers*, a simple Sprintf won't work
// (it will just print pointers to data). And once you provide custom print function,
// you need take care of sorting, as range(map) spews out members a in random order.
func (ri ResourceInfo) String() string {
	var procs []string
	for _, pid := range ri.keysSorted() {
		var dataStr []string
		for _, ctr := range ri[pid].counters.keysSorted() {
			dataStr = append(dataStr, fmt.Sprintf("%v:%v", ctr, ri[pid].counters[ctr]))
		}
		procs = append(procs, fmt.Sprintf("[%s(%s): [%s]]", pid, ri[pid].command, strings.Join(dataStr, " ")))
	}
	return fmt.Sprintf("{ResourceInfo: %s}", strings.Join(procs, " "))
}

// GetResourceInfo gets current resource counters values.
func GetResourceInfo(ctx context.Context, conn *ssh.Conn, cmdList string) (ResourceInfo, error) {
	var info = make(ResourceInfo)
	err := info.loadMemInfo(ctx, conn, cmdList)
	if err != nil {
		return ResourceInfo{}, err
	}
	err = info.loadFdNum(ctx, conn)
	if err != nil {
		return ResourceInfo{}, err
	}
	return info, nil
}

// ValidatePids checks if a diff between two sets of pids hasn't changed.
func ValidatePids(ri1, ri2 ResourceInfo) error {
	// For a couple of pid values, reflect has negligible performance hit.
	if diff := cmp.Diff(ri1.pids(), ri2.pids(), cmpopts.SortSlices(func(a, b string) bool { return a < b })); diff != "" {
		return errors.Errorf("pid sets: %q and %q are not equal (%s), suspecting crash", ri1.pids(), ri2.pids(), diff)
	}
	return nil
}

// ValidateResourceInfo checks if a diff between two set of counters is kept within certain thresholds.
// Positive threshold means percentage difference. Negative threshold means absolute difference. 0 threshold means no difference.
func ValidateResourceInfo(ctx context.Context, ri1, ri2 ResourceInfo, thr ResourceThreshold) error {
	if err := ValidatePids(ri1, ri2); err != nil {
		return err
	}
	for process := range ri2 {
		for counter, threshold := range thr {
			val1, ok := (ri1[process].counters)[counter]
			if !ok {
				testing.ContextLogf(ctx, "counter %s not found in %s1", counter, process)
				continue
			}
			val2, ok := (ri2[process].counters)[counter]
			if !ok {
				testing.ContextLogf(ctx, "counter %s not found in %s2", counter, process)
				continue
			}
			cmd := ri2[process].command
			if threshold == 0 {
				// Threshold == 0 means there should be no change.
				if val2 != val1 {
					return errors.Errorf("Expecting %s of %s(%s) value: %d, got %d",
						counter, process, cmd, val1, val2)
				}
			} else if threshold < 0 {
				// Threshold < 0 means compare the absolute difference.
				if math.Abs(float64(val2-val1)) > math.Abs(float64(threshold)) {
					return errors.Errorf("unexpected value increase in %s of %s(%s) from value: %d to %d, exceeds %f threshold",
						counter, process, cmd, val1, val2, math.Abs(float64(threshold)))
				}
			} else {
				// Threshold > 0 means compare the percentage difference.
				if (val2-val1)*100/val1 > threshold {
					return errors.Errorf("unexpected value increase in %s of %s(%s) from value: %d to %d, exceeds %d%% threshold",
						counter, process, cmd, val1, val2, threshold)
				}
			}
		}
	}
	return nil
}

// writeResourceInfoTSV stores the content of provided ResourceInfo slice to a file handle.
func writeResourceInfoTSV(ctx context.Context, f *os.File, ri []ResourceInfo) error {
	// Check for empty slice. If it misses data series or process list, there's no point in saving anything.
	if len(ri) == 0 || len(ri[0]) == 0 {
		return errors.New("Empty data series")
	}
	// Get the labels from maps.
	procKeys := ri[0].keysSorted()
	ctrKeys := ri[0][procKeys[0]].counters.keysSorted()

	// Prepare header.
	seriesLabelParts := []string{"Round"}
	for _, proc := range procKeys {
		for _, ctr := range ctrKeys {
			seriesLabelParts = append(seriesLabelParts, fmt.Sprintf("%s(%s):%s", proc, ri[0][proc].command, ctr))
		}
	}
	if _, err := f.WriteString(strings.Join(seriesLabelParts, "\t") + "\n"); err != nil {
		return errors.Wrap(err, "failed to write to tsv file (disk full?)")
	}

	// Dump data.
	for i, info := range ri {
		// Check for timeouts.
		if err := ctx.Err(); err != nil {
			return err
		}
		// First the round number.
		row := []string{strconv.Itoa(i)}
		for _, proc := range procKeys {
			for _, ctr := range ctrKeys {
				row = append(row, strconv.Itoa((info[proc].counters)[ctr]))
			}
		}
		if _, err := f.WriteString(strings.Join(row, "\t") + "\n"); err != nil {
			return errors.Wrap(err, "failed to write to tsv file (disk full?)")
		}
	}
	return nil
}

// ReportResources stores the content of provided ResourceInfo slice to a TSV file.
func ReportResources(ctx context.Context, ri []ResourceInfo, outDir, filename string) error {
	const tsvOutputDir = "tsvs"

	testing.ContextLog(ctx, "Writing .tsv files")
	if err := os.MkdirAll(filepath.Join(outDir, tsvOutputDir), 0755); err != nil {
		return errors.Wrap(err, "cannot create tsv directory")
	}

	resultFileName := filepath.Join(outDir, tsvOutputDir, filename)
	f, err := os.Create(resultFileName)
	if err != nil {
		return errors.Wrap(err, "cannot create tsv file")
	}
	defer f.Close()
	if err := writeResourceInfoTSV(ctx, f, ri); err != nil {
		return errors.Wrap(err, "cannot dump resources into file")
	}
	return nil
}
