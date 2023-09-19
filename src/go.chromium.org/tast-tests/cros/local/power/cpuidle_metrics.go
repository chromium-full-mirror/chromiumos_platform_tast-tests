// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	cp "go.chromium.org/tast-tests/cros/common/power"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const c0State = "C0"

type cpuidleTimeFile struct {
	stateName string
	path      string
}

// computeCpuidleStateFiles returns a mapping from cpus to files
// containing the total time spent in the idle states.
func computeCpuidleStateFiles(ctx context.Context) (map[string][]cpuidleTimeFile, int, error) {
	ret := make(map[string][]cpuidleTimeFile)
	numCpus := 0

	const cpusDir = "/sys/devices/system/cpu/"
	cpuInfos, err := ioutil.ReadDir(cpusDir)
	if err != nil {
		return nil, 0, errors.Wrap(err, "failed to find cpus")
	}

	for _, cpuInfo := range cpuInfos {
		// Match files with name cpu0, cpu1, ....
		if match, err := regexp.MatchString(`^cpu\d+$`, cpuInfo.Name()); err != nil {
			return nil, 0, errors.Wrap(err, "error trying to match cpu name")
		} else if !match {
			continue
		}
		numCpus++

		cpuDir := path.Join(cpusDir, cpuInfo.Name(), "cpuidle")
		cpuidles, err := ioutil.ReadDir(cpuDir)
		if err != nil {
			testing.ContextLogf(ctx, "System does not expose %v, skipping CPU", cpuDir)
			continue
		}

		for _, cpuidle := range cpuidles {
			// Match files with name state0, state1, ....
			if match, err := regexp.MatchString(`^state\d+$`, cpuidle.Name()); err != nil {
				return nil, 0, errors.Wrap(err, "error trying to match idle state name")
			} else if !match {
				continue
			}

			stateName, err := readFirstLine(ctx, path.Join(cpuDir, cpuidle.Name(), "name"))
			if err != nil {
				return nil, 0, errors.Wrap(err, "failed to read cpuidle name")
			}
			latency, err := readFirstLine(ctx, path.Join(cpuDir, cpuidle.Name(), "latency"))
			if err != nil {
				return nil, 0, errors.Wrap(err, "failed to read cpuidle latency")
			}

			if latency == "0" && stateName == "POLL" {
				// C0 state. Kernel stats aren't right, so calculate by
				// subtracting all other states from total time (using epoch
				// timer since we calculate differences in the end anyway).
				// NOTE: Only x86 lists C0 under cpuidle, ARM does not.
				continue
			}

			ret[cpuInfo.Name()] = append(ret[cpuInfo.Name()], cpuidleTimeFile{
				stateName: stateName,
				path:      path.Join(cpuDir, cpuidle.Name(), "time"),
			})
		}
	}

	return ret, numCpus, nil
}

// CpuidleStateMetrics records the C-states of the DUT.
//
// NOTE: The cpuidle timings are measured according to the kernel. They resemble
// hardware cstates, but they might not have a direct correspondence. Furthermore,
// they generally may be greater than the time the CPU actually spends in the
// corresponding cstate, as the hardware may enter shallower states than requested.
type CpuidleStateMetrics struct {
	cpuidleTimeFiles map[string][]cpuidleTimeFile
	numCpus          int
	lastTime         time.Time
	lastStats        map[string](map[string]int64)
	metrics          map[string]perf.Metric
	intervalName     string
	prefix           string
}

// Assert that CpuidleStateMetrics can be used in perf.Timeline.
var _ perf.TimelineDatasource = &CpuidleStateMetrics{}

// NewCpuidleStateMetrics creates a timeline metric to collect C-state numbers.
func NewCpuidleStateMetrics() *CpuidleStateMetrics {
	return &CpuidleStateMetrics{nil, 0, time.Time{}, nil, make(map[string]perf.Metric), "", ""}
}

// Setup determines what C-states are supported and which CPUs should be queried.
func (cs *CpuidleStateMetrics) Setup(ctx context.Context, prefix, intervalName string) error {
	cpuidleTimeFiles, numCpus, err := computeCpuidleStateFiles(ctx)
	if err != nil {
		return errors.Wrap(err, "error finding cpuidles")
	}
	cs.cpuidleTimeFiles = cpuidleTimeFiles
	cs.numCpus = numCpus
	cs.intervalName = intervalName
	cs.prefix = prefix
	return nil
}

// readCpuidleStateTimes reads the cpuidle timings and return a mapping from cpu idle states and cpu names
// to the time spent in the state & cpu pairs so far.
func readCpuidleStateTimes(ctx context.Context, cpuidleTimeFiles map[string][]cpuidleTimeFile) (map[string](map[string]int64), time.Time, error) {
	ret := make(map[string](map[string]int64))

	// Open all CPU files in advance so reading from the files can be as fast as possible.
	openFiles := make(map[string]*os.File)
	fileNum := 0
	for cpuName, files := range cpuidleTimeFiles {
		ret[cpuName] = make(map[string]int64)
		fileNum += len(files)
		for _, file := range files {
			f, err := os.Open(file.path)
			if err != nil {
				return nil, time.Time{}, errors.Wrapf(err, "failed to open CPU file %s", file.path)
			}
			defer f.Close()
			openFiles[file.path] = f
		}
	}

	type readResult struct {
		cpuName   string
		stateName string
		t         int64
		endTime   time.Time
	}
	readResults := make(chan *readResult, fileNum)
	readErrs := make(chan error, fileNum)

	var wg sync.WaitGroup
	wg.Add(fileNum)

	readCPUFile := func(ctx context.Context, cpuName string, file cpuidleTimeFile) {
		defer wg.Done()
		data := make([]byte, 30) // Make a big enough buffer to read int64 data.
		l, err := openFiles[file.path].Read(data)
		if err != nil {
			readErrs <- err
			return
		}
		fileReadingEnd := time.Now()
		if l == 0 {
			readErrs <- errors.Errorf("found no content in %q", file.path)
			return
		}
		t, err := strconv.ParseInt(strings.TrimSpace(string(data[0:l])), 10, 64)
		if err != nil {
			readErrs <- err
			return
		}
		readResults <- &readResult{cpuName, file.stateName, t, fileReadingEnd}
	}
	for cpuName, files := range cpuidleTimeFiles {
		for _, file := range files {
			// Read files with go routines in the effort to get a more accurate snapshot.
			go readCPUFile(ctx, cpuName, file)
		}
	}

	wg.Wait()
	close(readResults)
	close(readErrs)

	if len(readErrs) > 0 {
		// Return with the first error.
		return nil, time.Time{}, errors.Wrap(<-readErrs, "failed to read cpuidle timing")
	}

	// Find the latest reading end time as the snapshot end time.
	var endTime time.Time

	for r := range readResults {
		ret[r.cpuName][r.stateName] = r.t
		if endTime.Before(r.endTime) {
			endTime = r.endTime
		}
	}
	return ret, endTime, nil
}

// Start collects initial cpuidle numbers which we can use to
// compute the residency between now and the first Snapshot.
func (cs *CpuidleStateMetrics) Start(ctx context.Context) error {
	if cs.numCpus == 0 {
		return nil
	}

	stats, statTime, err := readCpuidleStateTimes(ctx, cs.cpuidleTimeFiles)
	if err != nil {
		return errors.Wrap(err, "failed to collect initial metrics")
	}

	cs.metrics[c0State] = perf.Metric{
		Name:      cs.prefix + cp.CPUIdleMetricType + "cpu-" + c0State,
		Unit:      cp.CPUIdleMetricTypeUnit,
		Direction: perf.SmallerIsBetter,
		Multiple:  true,
		Interval:  cs.intervalName}

	for cpuName, perCPUStats := range stats {

		// Per-cpu stats
		cs.metrics[cpuName+"-"+c0State] = perf.Metric{
			Name:      cs.prefix + cp.CPUIdleMetricType + cpuName + "-" + c0State,
			Unit:      cp.CPUIdleMetricTypeUnit,
			Direction: perf.SmallerIsBetter,
			Multiple:  true,
			Interval:  cs.intervalName}

		for stateName := range perCPUStats {
			// Per-cpu stats
			cs.metrics[cpuName+"-"+stateName] = perf.Metric{
				Name:      cs.prefix + cp.CPUIdleMetricType + cpuName + "-" + stateName,
				Unit:      cp.CPUIdleMetricTypeUnit,
				Direction: perf.SmallerIsBetter,
				Multiple:  true,
				Interval:  cs.intervalName}

			if _, isPresent := cs.metrics[stateName]; !isPresent {
				// Aggregated metrics of all the cpus
				cs.metrics[stateName] = perf.Metric{
					Name:      cs.prefix + cp.CPUIdleMetricType + "cpu-" + stateName,
					Unit:      cp.CPUIdleMetricTypeUnit,
					Direction: perf.SmallerIsBetter,
					Multiple:  true,
					Interval:  cs.intervalName}
			}
		}

	}

	cs.lastStats = stats
	cs.lastTime = statTime
	return nil
}

// Snapshot computes the cpuidle residency between this and
// the previous snapshot, and reports them as metrics.
func (cs *CpuidleStateMetrics) Snapshot(ctx context.Context, values *perf.Values) error {
	if cs.numCpus == 0 {
		return nil
	}

	stats, statTime, err := readCpuidleStateTimes(ctx, cs.cpuidleTimeFiles)
	if err != nil {
		return errors.Wrap(err, "failed to collect metrics")
	}

	diffs := make(map[string](map[string]int64))
	for cpuName, perCPUStats := range stats {
		diffs[cpuName] = make(map[string]int64)
		for stateName, stat := range perCPUStats {
			diffs[cpuName][stateName] = stat - cs.lastStats[cpuName][stateName]
		}
	}

	timeSlice := statTime.Sub(cs.lastTime).Microseconds()
	total := timeSlice * int64(cs.numCpus)
	c0Residency := total
	// Total time spent in a state by all the cpus
	totalResidency := make(map[string]int64)

	for cpuName, perCPUDiffs := range diffs {
		perCPUC0Residency := timeSlice

		var perCPUTotalResidency int64
		var adjustRate float64
		for _, diff := range perCPUDiffs {
			perCPUTotalResidency += diff
		}
		if perCPUTotalResidency > timeSlice {
			// This could happen if the values read from system CPU files are not the realtime ones.
			adjustRate = float64(timeSlice) / float64(perCPUTotalResidency)
		}

		for stateName, diff := range perCPUDiffs {
			if adjustRate > 0 {
				// Adjust each state proportionally.
				diff = int64(float64(diff) * adjustRate)
			}
			values.Append(cs.metrics[cpuName+"-"+stateName], (float64(diff)/float64(timeSlice))*100)
			c0Residency -= diff
			perCPUC0Residency -= diff

			totalResidency[stateName] += diff
		}
		values.Append(cs.metrics[cpuName+"-"+c0State], (float64(perCPUC0Residency)/float64(timeSlice))*100)
	}

	for stateName, diff := range totalResidency {
		values.Append(cs.metrics[stateName], (float64(diff)/float64(total))*100)
	}
	values.Append(cs.metrics[c0State], (float64(c0Residency)/float64(total))*100)

	cs.lastStats = stats
	cs.lastTime = statTime
	return nil
}

// Stop does nothing.
func (cs *CpuidleStateMetrics) Stop(ctx context.Context, values *perf.Values) error {
	return nil
}
