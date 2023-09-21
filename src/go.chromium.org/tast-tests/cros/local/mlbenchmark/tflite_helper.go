// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mlbenchmark

import (
	"bufio"
	"context"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/testing"
)

// TFLiteBackendType is an enum to select the TFLite delegate backend.
type TFLiteBackendType string

const (
	// KCpu is the default, cpu backend.
	KCpu TFLiteBackendType = "cpu"
	// KGpuOpenGl uses the GPU with OpenGL.
	KGpuOpenGl TFLiteBackendType = "gpu_opengl"
	// KGpuOpenCl uses the GPU with OpenCL.
	KGpuOpenCl TFLiteBackendType = "gpu_opencl"

	benchmarkModelCLI     = "benchmark_model"
	powerSamplingInterval = 1 * time.Second

	// minDurationSeconds is the minimum running duration.
	minDurationSeconds = 60
	// runFrequency is the number of times per second that inferences
	// should be performed. The purpose here is to throttle models that
	// are small so that improved performance doesn't result in higher
	// power draw.
	runFrequency = 30
)

// TFLiteBenchmarkParams is used to define a specific TFLite benchmark test.
type TFLiteBenchmarkParams struct {
	DataFilename  string
	GraphFilename string
	Backend       TFLiteBackendType
}

type benchmarkResults struct {
	InitLatency           float64
	FirstInferenceLatency float64
	AvgLatency            float64
	StdDev                float64
	PeakMemoryUsage       float64
	RunCount              int64
}

func parseNumeric(input string) (float64, error) {
	flt, _, err := big.ParseFloat(input, 10, 0, big.ToNearestEven)
	if err != nil {
		return 0, errors.Wrapf(err, "couldn't parse input: %s", input)
	}
	f, _ := flt.Float64()
	return f, nil
}

func parseOutput(output string) (*benchmarkResults, error) {
	// The benchmark_model output will contain several key lines.
	// There will be two that look like detailRe, and one that looks
	// like summaryRe.
	//
	// The first detailRe will be the 'warmup' run, and we can discard it.
	// The second detailRe will be the 'real' run, and we want to take the
	// Average Latency and Std Deviation values from there.
	//
	// There wil only be a single instance of summaryRe, and we will take the
	// Init and First Inference from there.
	//
	// There wil only be a single instance of memoryRe, and we will take the
	// Init and Overall memory usage from there.

	var results benchmarkResults
	var warmupRe = regexp.MustCompile(`^count=([\d\.]+) `)
	var detailRe = regexp.MustCompile(`^count=([\d\.]+) first=[\d\.]+ curr=[\d\.]+ min=[\d\.]+ max=[\d\.]+ avg=([\d\.]+) std=([\d\.]+)`)
	var summaryRe = regexp.MustCompile(`^Inference timings in us: Init: ([\d\.]+), First inference: ([\d\.]+), Warmup \(avg\): [\d\.]+, Inference \(avg\): [\d\.]+`)
	var memoryRe = regexp.MustCompile(`^Overall peak memory footprint \(MB\) via periodic monitoring: ([\d\.]+)`)

	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Split(bufio.ScanLines)
	warmupPassed, detailResultsFound, summaryResultFound, memoryResultFound := false, false, false, false
	for scanner.Scan() {
		// Check to see if the warmup string is output, if so there will be 2 group matches.
		matches := warmupRe.FindStringSubmatch(scanner.Text())
		if len(matches) == 2 {
			// If we've passed the warmup stage, check if this is the detail line. If so, 4 group matches.
			if warmupPassed {
				matches := detailRe.FindStringSubmatch(scanner.Text())
				if len(matches) == 4 {
					var err error
					results.RunCount, err = strconv.ParseInt(matches[1], 10, 64)
					if err != nil {
						return nil, errors.Wrap(err, "couldn't parse RunCount")
					}
					results.AvgLatency, err = parseNumeric(matches[2])
					if err != nil {
						return nil, errors.Wrap(err, "couldn't parse AvgLatency")
					}
					results.StdDev, err = parseNumeric(matches[3])
					if err != nil {
						return nil, errors.Wrap(err, "couldn't parse StdDev")
					}
					detailResultsFound = true
				}
			} else {
				warmupPassed = true
			}
		}

		// Check if the string matches summaryRe. If so, there will be 3 group matches.
		matches = summaryRe.FindStringSubmatch(scanner.Text())
		if len(matches) > 2 {
			var err error
			results.InitLatency, err = parseNumeric(matches[1])
			if err != nil {
				return nil, errors.Wrap(err, "couldn't parse InitLatency")
			}
			results.FirstInferenceLatency, err = parseNumeric(matches[2])
			if err != nil {
				return nil, errors.Wrap(err, "couldn't parse FirstInferenceLatency")
			}
			summaryResultFound = true
		}

		// Check if the string matches memoryRe. If so, there will be 2 group matches.
		matches = memoryRe.FindStringSubmatch(scanner.Text())
		if len(matches) > 1 {
			var err error
			results.PeakMemoryUsage, err = parseNumeric(matches[1])
			if err != nil {
				return nil, errors.Wrap(err, "couldn't parse InitialMemoryUsage")
			}
			memoryResultFound = true
		}
	}

	if !detailResultsFound {
		return nil, errors.New("unable to find the avg latency and stddev in the benchmarking output")
	}
	if !summaryResultFound {
		return nil, errors.New("unable to find the init and first latency in the benchmarking output")
	}
	if !memoryResultFound {
		return nil, errors.New("unable to find the memory usage in the benchmarking output")
	}

	return &results, nil
}

func populateMetrics(results *benchmarkResults, p *perf.Values) {
	p.Set(perf.Metric{
		Name:      "init_latency",
		Unit:      "ms",
		Direction: perf.SmallerIsBetter,
		Multiple:  false},
		results.InitLatency/1000)
	p.Set(perf.Metric{
		Name:      "first_inference_latency",
		Unit:      "ms",
		Direction: perf.SmallerIsBetter,
		Multiple:  false},
		results.FirstInferenceLatency/1000)
	p.Set(perf.Metric{
		Name:      "avg_latency",
		Unit:      "ms",
		Direction: perf.SmallerIsBetter,
		Multiple:  false},
		results.AvgLatency/1000)
	p.Set(perf.Metric{
		Name:      "std_dev",
		Unit:      "sigma",
		Direction: perf.SmallerIsBetter,
		Multiple:  false},
		results.StdDev)
	p.Set(perf.Metric{
		Name:      "peak_memory",
		Unit:      "bytes",
		Direction: perf.SmallerIsBetter,
		Multiple:  false},
		results.PeakMemoryUsage*1024*1024)
}

func buildBenchmarkArgs(graphFileName string, backend TFLiteBackendType) map[string]string {
	m := make(map[string]string)

	m["--graph"] = DataPath(graphFileName)
	m["--min_secs"] = strconv.FormatInt(minDurationSeconds, 10)
	m["--run_frequency"] = strconv.FormatInt(runFrequency, 10)
	m["--report_peak_memory_footprint"] = "true"

	if backend == KGpuOpenGl {
		m["--use_gpu"] = "true"
		m["--gpu_backend"] = "gl"
	}
	if backend == KGpuOpenCl {
		m["--use_gpu"] = "true"
		m["--gpu_backend"] = "cl"
	}

	return m
}

func executeBenchmark(ctx context.Context, graphFileName string, backend TFLiteBackendType, p *perf.Values) error {
	var cmd = BuildCommand(ctx, benchmarkModelCLI, buildBenchmarkArgs(graphFileName, backend))

	testing.ContextLog(ctx, "Benchmark command: ", shutil.EscapeSlice(cmd.Args))
	output, err := cmd.CombinedOutput()
	outputStr := string(output[:])
	if err != nil {
		return errors.Wrapf(err, " benchmark failed, log output: %s", outputStr)
	}

	results, err := parseOutput(outputStr)
	if err != nil {
		return errors.Wrapf(err, " failed to parse benchmark output: %s", outputStr)
	}
	testing.ContextLogf(ctx, "Results: %+v", results)

	populateMetrics(results, p)

	return nil
}

// RunTFLiteBenchmark will run the `benchmark_model` tool for a given `graphFileName` model file, using
// the defined `backend` (e.g. CPU / GPU).
func RunTFLiteBenchmark(ctx context.Context, testName, dataFilePath, graphFileName string, backend TFLiteBackendType) error {
	if err := UnpackData(ctx, dataFilePath); err != nil {
		return errors.Wrap(err, "couldn't unpack test data")
	}

	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return errors.New("couldn't determine test output dir")
	}

	r := power.NewRecorder(ctx, powerSamplingInterval, outDir, testName)
	defer r.Close(cleanupCtx)

	if err := r.Cooldown(ctx); err != nil {
		return errors.Wrap(err, "cooldown failed")
	}
	if err := r.Start(ctx); err != nil {
		return errors.Wrap(err, "failed to start power metrics recording)")
	}

	p := perf.NewValues()
	if err := executeBenchmark(ctx, graphFileName, backend, p); err != nil {
		return errors.Wrap(err, "benchmark failed")
	}

	if err := r.Finish(ctx, p); err != nil {
		return errors.Wrap(err, "failed to finish collecting power metrics")
	}

	return nil
}
