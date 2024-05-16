// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package linuxperf

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/shirou/gopsutil/v3/process"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast/core/testing"
)

// ThreadCount stores the count of Linux Perf events that were sampled on a
// thread.
type ThreadCount struct {
	Name  string `json:"name"`
	Tid   int    `json:"tid"`
	Count uint64 `json:"count"`
}

// ProcessCount stores the count of Linux Perf events that were sampled in a
// process.
type ProcessCount struct {
	Name    string               `json:"name"`
	Pid     int                  `json:"pid"`
	Count   uint64               `json:"count"`
	Threads map[int]*ThreadCount `json:"threads"`
}

// ProcessThreadCounts aggregates the count of events in a Linux Perf profile
// by process and thread.
//
// Example:
//
//	cycles := linuxperf.NewProcessThreadCounts("cpu-cycles")
//
//	if err := linuxperf.Script(ctx, perdDataFile, cycles.OnEvent); err != nil {
//		return outFiles, errors.Wrap(err, "failed to extract metrics from perf data")
//	}
//
//	cycles.SetProcessCountMetrics(
//		perfValues,
//		[]string{"chrome", "crosvm"},
//		"cycles",
//		1.0,
//		"",
//	)
type ProcessThreadCounts struct {
	eventName    string
	processNames map[int]string
	Counts       map[int]*ProcessCount
}

// EventName returns the name of the event that this object is counting.
func (c *ProcessThreadCounts) EventName() string {
	return c.eventName
}

// OnEvent is a function passed to Instance.Script to aggregate profile samples.
func (c *ProcessThreadCounts) OnEvent(event *Event) error {
	// If an event happened on the main thread of a process, use its Comm to
	// name the existing or future ProcessCount. Even if this is not the event
	// type we are looking for.
	if event.Pid == event.Tid {
		// Add the name to the map so that it can be used when a ProcessCount
		// is created.
		c.processNames[event.Pid] = event.Comm

		// Name the process if it already exists.
		if pc, ok := c.Counts[event.Pid]; ok {
			pc.Name = event.Comm
		}
	}

	if event.Name != c.eventName {
		return nil
	}

	// Get or initialize ProcessCount.
	processCount, hasProcessCount := c.Counts[event.Pid]
	if !hasProcessCount {
		name, hasName := c.processNames[event.Pid]
		if !hasName {
			name = "[unknown]"
			// Try to get the process name from /proc. The process might no
			// longer exist so ignore failures.
			if p, err := process.NewProcess(int32(event.Pid)); err == nil {
				if n, err := p.Name(); err == nil {
					name = n
				}
			}
		}
		processCount = &ProcessCount{
			Name:    name,
			Pid:     event.Pid,
			Count:   0,
			Threads: make(map[int]*ThreadCount),
		}
		c.Counts[event.Pid] = processCount
	}

	// Get or initialize ThreadCount.
	threadCount, hasThreadCount := processCount.Threads[event.Tid]
	if !hasThreadCount {
		threadCount = &ThreadCount{
			Name:  event.Comm,
			Tid:   event.Tid,
			Count: 0,
		}
		processCount.Threads[event.Tid] = threadCount
	}

	// For the event we care about, accumulate counts.
	processCount.Count += event.Count
	threadCount.Count += event.Count
	return nil
}

// ThreadNameCount stores the count of Linux Perf events that were sampled on
// all threads with the same name within a process.
type ThreadNameCount struct {
	Name  string `json:"name"`
	Tids  []int  `json:"tids"`
	Count uint64 `json:"count"`
}

// ThreadNameCounts indexes Linux Perf event sample counts on threads within a
// process by name.
type ThreadNameCounts map[string]*ThreadNameCount

type orderedThreadNameCounts []*ThreadNameCount

func (o orderedThreadNameCounts) Len() int {
	return len(o)
}

func (o orderedThreadNameCounts) Less(i, j int) bool {
	return o[j].Count < o[i].Count
}

func (o orderedThreadNameCounts) Swap(i, j int) {
	scratch := o[i]
	o[i] = o[j]
	o[j] = scratch
}

// Sort ThreadNameCounts by sampled event count, most events first.
func (c ThreadNameCounts) Sort() []*ThreadNameCount {
	var res orderedThreadNameCounts
	for _, threadCount := range c {
		res = append(res, threadCount)
	}
	sort.Sort(res)
	return res
}

// ProcessNameCount stores the count of Linux Perf events that were sampled in
// all processes with the same name.
type ProcessNameCount struct {
	Name    string           `json:"name"`
	Pids    []int            `json:"pids"`
	Count   uint64           `json:"count"`
	Threads ThreadNameCounts `json:"threads"`
}

// ProcessNameCounts indexes Linux Perf event sample counts by process name.
type ProcessNameCounts map[string]*ProcessNameCount

type orderedProcessNameCounts []*ProcessNameCount

func (o orderedProcessNameCounts) Len() int {
	return len(o)
}

func (o orderedProcessNameCounts) Less(i, j int) bool {
	return o[j].Count < o[i].Count
}

func (o orderedProcessNameCounts) Swap(i, j int) {
	scratch := o[i]
	o[i] = o[j]
	o[j] = scratch
}

// Sort ProcessNameCounts by samples event count, most events first.
func (c ProcessNameCounts) Sort() []*ProcessNameCount {
	var res orderedProcessNameCounts
	for _, processCount := range c {
		res = append(res, processCount)
	}
	sort.Sort(res)
	return res
}

// ByName aggregates counts by process and thread name, merging counts from
// processes and threads with different PIDs/TIDs but with the same name.
func (c *ProcessThreadCounts) ByName(mapName func(string) string) ProcessNameCounts {
	res := make(map[string]*ProcessNameCount)
	for _, processCount := range c.Counts {
		name := mapName(processCount.Name)
		resProcess, hasResProcess := res[name]
		if !hasResProcess {
			resProcess = &ProcessNameCount{
				Name:    name,
				Pids:    nil,
				Count:   0,
				Threads: make(map[string]*ThreadNameCount),
			}
			res[name] = resProcess
		}
		resProcess.Pids = append(resProcess.Pids, processCount.Pid)
		resProcess.Count += processCount.Count

		for _, threadCount := range processCount.Threads {
			name := mapName(threadCount.Name)
			resThread, hasResThread := resProcess.Threads[name]
			if !hasResThread {
				resThread = &ThreadNameCount{
					Name:  name,
					Tids:  nil,
					Count: 0,
				}
				resProcess.Threads[name] = resThread
			}

			resThread.Tids = append(resThread.Tids, threadCount.Tid)
			resThread.Count += threadCount.Count
		}
	}
	return res
}

// Total count of all events sampled.
func (c *ProcessThreadCounts) Total() uint64 {
	total := uint64(0)
	for _, p := range c.Counts {
		total += p.Count
	}
	return total
}

// Log a summary of counts by process and thread name. Processes and threads
// with counts below threshold will have their counts contribute to an "Other"
// line and not be logged directly.
func (c *ProcessThreadCounts) Log(ctx context.Context, threshold uint64) {
	countToPercent := 100.0 / float64(c.Total())
	pOther := uint64(0)
	for _, p := range c.ByName(func(s string) string { return s }).Sort() {
		if p.Count >= threshold {
			testing.ContextLogf(ctx, "%-24s %16d (%.2f%%)", p.Name, p.Count, float64(p.Count)*countToPercent)
			tOther := uint64(0)
			for _, t := range p.Threads.Sort() {
				if t.Count >= threshold {
					testing.ContextLogf(ctx, "  - %-20s %16d (%.2f%%)", t.Name, t.Count, float64(t.Count)*countToPercent)
				} else {
					tOther += t.Count
				}
			}
			if tOther > 0 {
				testing.ContextLogf(ctx, "  - Other Threads        %16d (%.2f%%)", tOther, float64(tOther)*countToPercent)
			}
		} else {
			pOther += p.Count
		}
	}
	if pOther > 0 {
		testing.ContextLogf(ctx, "Other Processes          %16d (%.2f%%)", pOther, float64(pOther)*countToPercent)
	}
}

// sanitizeMetricString sanitizes a string so that it can be used in a metric
// name.
func sanitizeMetricString(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case 'a' <= r && r <= 'z':
		case 'A' <= r && r <= 'Z':
		case '0' <= r && r <= '9':
		case r == '.':
		case r == '_':
		case r == '-':
		default:
			// Trim every rune that doesn't match the above cases.
			return -1
		}
		return r
	}, s)
}

func (c *ProcessThreadCounts) writeProcessCountMetrics(processList []string, unit string, scale float64, suffix string, logFunc func(s perf.Metric, vs ...float64), multiple bool) {
	processSet := make(map[string]bool)
	for _, name := range processList {
		processSet[sanitizeMetricString(name)] = true
	}

	otherCount := uint64(0)
	totalCount := uint64(0)
	for _, processCount := range c.ByName(sanitizeMetricString) {
		totalCount += processCount.Count
		if !processSet[processCount.Name] {
			otherCount += processCount.Count
			continue
		}
		logFunc(
			perf.Metric{
				Name:      fmt.Sprintf("perf_proc_%s%s", processCount.Name, suffix),
				Unit:      unit,
				Direction: perf.SmallerIsBetter,
				Multiple:  multiple,
			},
			float64(processCount.Count)*scale,
		)
	}

	logFunc(
		perf.Metric{
			Name:      fmt.Sprintf("perf_other_proc%s", suffix),
			Unit:      unit,
			Direction: perf.SmallerIsBetter,
			Multiple:  multiple,
		},
		float64(otherCount)*scale,
	)
	logFunc(
		perf.Metric{
			Name:      fmt.Sprintf("perf_total_proc%s", suffix),
			Unit:      unit,
			Direction: perf.SmallerIsBetter,
			Multiple:  multiple,
		},
		float64(totalCount)*scale,
	)
}

// SetProcessCountMetrics writes the following metrics to a perf.Values
// containing the counts of Linux Perf event sampled:
//   - perf_proc_{process name}{suffix} for each process in processList.
//   - perf_other_proc{suffix} with a sum of all other processes.
//   - perf_total_proc{suffix} with a sum of all event counts.
//
// Metrics are logged with perf.Values.Set, with the passed unit, and multiplied
// by the passed scale.
//
// Note that {process name} is sanitized to be a valid metric name. If two
// process names are sanitized to the same name, then only one metric is logged.
func (c *ProcessThreadCounts) SetProcessCountMetrics(p *perf.Values, processList []string, unit string, scale float64, suffix string) {
	c.writeProcessCountMetrics(processList, unit, scale, suffix, p.Set, false)
}

// AppendProcessCountMetrics writes the following metrics to a perf.Values
// containing the counts of Linux Perf event sampled:
//   - perf_proc_{process name}{suffix} for each process in processList.
//   - perf_other_proc{suffix} with a sum of all other processes.
//   - perf_total_proc{suffix} with a sum of all event counts.
//
// Metrics are logged with perf.Values.Append, with the passed unit, and
// multiplied by the passed scale.
//
// Note that {process name} is sanitized to be a valid metric name. If two
// process names are sanitized to the same name, then only one metric is logged.
func (c *ProcessThreadCounts) AppendProcessCountMetrics(p *perf.Values, processList []string, unit string, scale float64, suffix string) {
	c.writeProcessCountMetrics(processList, unit, scale, suffix, p.Append, true)
}

// NewProcessThreadCounts categorizes Linux Perf event counts by the process and
// thread that the sample occurred in.
func NewProcessThreadCounts(eventName string) *ProcessThreadCounts {
	return &ProcessThreadCounts{eventName, make(map[int]string), make(map[int]*ProcessCount)}
}
