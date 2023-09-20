// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package util

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const keyValFileName = "keyval"

// fioResult is a serializable structure representing fio results output.
type fioResult struct {
	Jobs []struct {
		Jobname    string                 `json:"jobname"`
		Read       map[string]interface{} `json:"read"`
		Write      map[string]interface{} `json:"write"`
		Trim       map[string]interface{} `json:"trim"`
		Sync       map[string]interface{} `json:"sync"`
		TotalError int64                  `json:"total_err"`
		FirstError int64                  `json:"first_error"`
	}
	DiskUtil []struct {
		Name string
	} `json:"disk_util"`
}

// fioResultReport is a result report for a single io run for a group.
type fioResultReport struct {
	group  string
	result *fioResult
}

// fioDiskUsageReport is a report of disk lifetime usage.
type fioDiskUsageReport struct {
	name           string
	percentageUsed int64
	bytesWritten   int64
}

// FioResultWriter is a serial processor of fio results.
type FioResultWriter struct {
	resultLock  sync.Mutex
	results     []fioResultReport
	ignoreRead  bool
	ignoreWrite bool
	ignoreSync  bool
	ignoreTrim  bool
}

func (f *FioResultWriter) ignoreResult(read, write, sync, trim bool) {
	f.ignoreRead = read
	f.ignoreWrite = write
	f.ignoreSync = sync
	f.ignoreTrim = trim
}

// ReportOnlyRead makes writer output only read data.
func (f *FioResultWriter) ReportOnlyRead() {
	f.ignoreResult(false, true, true, true)
}

// ReportOnlyWrite makes writer output only write data.
func (f *FioResultWriter) ReportOnlyWrite() {
	f.ignoreResult(true, false, true, true)
}

// ReportOnlyReadWrite makes writer output only read and write data.
func (f *FioResultWriter) ReportOnlyReadWrite() {
	f.ignoreResult(false, false, true, true)
}

func (f *FioResultWriter) reportResults(ctx context.Context, res *fioResult, perfValues *perf.Values) {
	for _, job := range res.Jobs {
		if !f.ignoreRead {
			reportJobRWResult(ctx, job.Read, job.Jobname+"_read", perfValues)
		}
		if !f.ignoreWrite {
			reportJobRWResult(ctx, job.Write, job.Jobname+"_write", perfValues)
		}
		if !f.ignoreTrim {
			reportJobRWResult(ctx, job.Trim, job.Jobname+"_trim", perfValues)
		}
		if !f.ignoreSync {
			reportJobRWResult(ctx, job.Sync, job.Jobname+"_sync", perfValues)
		}
	}
}

// Save processes and saves reported results.
func (f *FioResultWriter) Save(ctx context.Context, path string, writeKeyVal bool) {
	f.resultLock.Lock()
	defer f.resultLock.Unlock()

	perfValues := perf.NewValues()

	for _, report := range f.results {
		f.reportResults(ctx, report.result, perfValues)
	}

	if writeKeyVal {
		if err := perfValues.Save(path); err != nil {
			testing.ContextLog(ctx, "Failed saving perf data: ", err)
		}
	}

	f.results = nil
}

// Report posts a single fio result to the writer.
func (f *FioResultWriter) Report(group string, result *fioResult) {
	f.resultLock.Lock()
	defer f.resultLock.Unlock()
	f.results = append(f.results, fioResultReport{group, result})
}

// reportJobRWResult appends metrics for latency and bandwidth from the current test results
// to the given perf values.
func reportJobRWResult(ctx context.Context, testRes map[string]interface{}, prefix string, perfValues *perf.Values) {
	flatResult, err := flattenNestedResults("", testRes)
	if err != nil {
		testing.ContextLog(ctx, "Error flattening results json: ", err)
		return
	}

	for k, v := range flatResult {
		if strings.Contains(k, "percentile") {
			perfValues.Set(perf.Metric{
				Name:      "_" + prefix + k,
				Unit:      "ns",
				Direction: perf.SmallerIsBetter,
			}, v)
		} else if k == "_bw" {
			perfValues.Set(perf.Metric{
				Name:      "_" + prefix + k,
				Unit:      "KB_per_sec",
				Direction: perf.BiggerIsBetter,
			}, v)
		} else if k == "_iops" {
			perfValues.Set(perf.Metric{
				Name:      "_" + prefix + k,
				Unit:      "iops",
				Direction: perf.BiggerIsBetter,
			}, v)
		}
	}
}

// writeKeyVals writes given key value data to an external file in output directory.
func writeKeyVals(outDir string, keyVals map[string]float64) error {
	if keyVals == nil {
		return errors.New("invalid data to write to keyval file")
	}

	filename := filepath.Join(outDir, keyValFileName)
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return errors.Wrapf(err, "failed to open file: %s", filename)
	}
	defer f.Close()

	// Sorting all results by name before writing to file.
	keys := make([]string, 0, len(keyVals))
	for k := range keyVals {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		if _, err := fmt.Fprintf(f, "%s=%v\n", key, keyVals[key]); err != nil {
			return errors.Wrap(err, "failed to write keyval file")
		}
	}
	return nil
}

// flattenNestedResults flattens nested structures to the root level.
// Names are prefixed with the nested names, i.e. {foo: { bar: {}}} -> {foo<prefix>bar: {}}
// TODO(abergman): can we avoid creating map for each nest level?
func flattenNestedResults(prefix string, nested interface{}) (flat map[string]float64, err error) {
	flat = make(map[string]float64)

	merge := func(to, from map[string]float64) {
		for kt, vt := range from {
			to[kt] = float64(vt)
		}
	}

	switch nested := nested.(type) {
	case map[string]interface{}:
		for k, v := range nested {
			fm1, fe := flattenNestedResults(prefix+"_"+k, v)
			if fe != nil {
				err = fe
				return
			}
			merge(flat, fm1)
		}
	case []interface{}:
		for i, v := range nested {
			fm1, fe := flattenNestedResults(prefix+"_"+strconv.Itoa(i), v)
			if fe != nil {
				err = fe
				return
			}
			merge(flat, fm1)
		}
	default:
		flat[prefix] = nested.(float64)
	}
	return
}
