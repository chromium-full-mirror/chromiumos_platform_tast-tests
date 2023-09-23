// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type audioSynthmarkJitterStressMode int

const audioSynthmarkJitterSynthmarkAPK = "synthmark-1.12.0.apk"

const (
	// audioSynthmarkJitterStressNone run the test without any stress
	audioSynthmarkJitterStressNone audioSynthmarkJitterStressMode = iota
)

type audioSynthmarkJitterParam struct {
	stressMode audioSynthmarkJitterStressMode
	options    []arc.ActivityStartOption
}

type audioSynthmarkJitterPercentile struct {
	p50  float64
	p99  float64
	p100 float64
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         AudioSynthmarkJitter,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Runs Synthmark Jittermark test alongside with cyclic bench on the host to measure scheduling delay with various stress",
		Contacts: []string{
			"chromeos-audio-bugs@google.com", // Audio team
			"pteerapong@chromium.org",        // Author
		},
		// ChromeOS > Platform > Virtualization > ARC++ & ARCVM > ARC Audio
		BugComponent: "b:879188",
		SoftwareDeps: []string{"chrome", "arc"},
		Fixture:      "arcBooted",
		Data:         []string{audioSynthmarkJitterSynthmarkAPK},
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild", "group:audio"},
		Timeout:      15 * time.Minute,
		Params: []testing.Param{{
			Name: "nostress",
			Val: audioSynthmarkJitterParam{
				stressMode: audioSynthmarkJitterStressNone,
			},
		}},
	})
}

// AudioSynthmarkJitter runs Synthmark Jitter Mark test and stores the result.
func AudioSynthmarkJitter(ctx context.Context, s *testing.State) {
	const (
		cleanupTime      = 30 * time.Second
		testDuration     = 30 * time.Second
		cyclicBenchLoops = 6000 // each loop takes around 5ms

		pkg          = "com.sonodroid.synthmark"
		activityName = ".MainActivity"
	)

	param := s.Param().(audioSynthmarkJitterParam)
	a := s.FixtValue().(*arc.PreData).ARC
	cr := s.FixtValue().(*arc.PreData).Chrome
	d := s.FixtValue().(*arc.PreData).UIDevice

	// Reserve time for clean up
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, cleanupTime)
	defer cancel()

	testing.ContextLog(ctx, "Install app")
	if err := a.Install(ctx, s.DataPath(audioSynthmarkJitterSynthmarkAPK)); err != nil {
		s.Fatal("Failed to install app: ", err)
	}
	defer a.Uninstall(cleanupCtx, pkg)

	// create activity
	activity, err := arc.NewActivity(a, pkg, activityName)
	if err != nil {
		s.Fatalf("Failed to create activity %q in package %q: %v", activityName, pkg, err)
	}
	defer activity.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Launch cyclic_bench on the host
	var cyclicBenchOut bytes.Buffer
	cyclicBenchCmd := testexec.CommandContext(ctx, "cyclic_bench.py", "--loops="+strconv.Itoa(cyclicBenchLoops), "--interval=5000", "--policy=rr", "--priority=12", "--json")
	cyclicBenchCmd.Stdout = &cyclicBenchOut
	if err := cyclicBenchCmd.Start(); err != nil {
		s.Fatal("Failed to start cyclic bench on the host: ", err)
	}

	// Launch app
	launchParams := param.options
	launchParams = append(launchParams, arc.WithExtraString("test", "jitter"))
	// num_seconds must be passed as float.
	launchParams = append(launchParams, arc.WithExtraFloat("num_seconds", testDuration.Seconds()))
	if err := activity.Start(ctx, tconn, launchParams...); err != nil {
		s.Fatalf("Failed to start activity %q in package %q: %v", activityName, pkg, err)
	}
	defer activity.Stop(cleanupCtx, tconn)

	testing.ContextLogf(ctx, "Sleeping for %v to wait for the test to finish", testDuration)
	// GoBigSleepLint: Run the test for `testDuration` as a part of measurement.
	testing.Sleep(ctx, testDuration)
	testing.ContextLog(ctx, "Waiting for the test to finish")
	var resultText string
	if err := testing.Poll(ctx, func(ctx context.Context) (err error) {
		resultText, err = d.Object(ui.ID("com.sonodroid.synthmark:id/textViewOutput")).GetText(ctx)
		if err != nil {
			return err
		}
		if !strings.Contains(resultText, "RESULTS_END") {
			return errors.New("result text does not contain RESULTS_END")
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  5 * time.Minute,
		Interval: 1 * time.Second,
	}); err != nil {
		s.Fatal("Failed to wait for RESULTS_END text: ", err)
	}

	// Log result text if the test failed for debugging
	defer func() {
		if s.HasError() {
			s.Logf("Result text = %q", resultText)
		}
	}()

	jitterPercentile, err := getJitterPercentileFromJittermarkResult(resultText)
	if err != nil {
		s.Fatal("Failed to get jitter percentile: ", err)
	}

	// Wait and parse cyclic bench result
	testing.ContextLog(ctx, "Waiting for cyclic bench to finish")
	if err := cyclicBenchCmd.Wait(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to wait for cyclic bench: ", err)
	}
	stats := struct {
		CyclicTestStat []struct {
			ThreadID float64 `json:"thread_id"`
			Min      float64 `json:"min"`
			Median   float64 `json:"median"`
			P99      float64 `json:"p99"`
			Max      float64 `json:"max"`
		} `json:"stats"`
	}{}
	if err := json.Unmarshal(cyclicBenchOut.Bytes(), &stats); err != nil {
		s.Fatal("Failed to parse cyclic bench result as JSON: ", err)
	}
	if len(stats.CyclicTestStat) != 1 {
		s.Fatalf("Cyclic bench result length is %d != 1", len(stats.CyclicTestStat))
	}
	cyclicBenchResult := stats.CyclicTestStat[0] // Get result from the first thread (we ran it with 1 thread)

	// Record perf values
	perfValues := perf.NewValues()
	defer func() {
		if err := perfValues.Save(s.OutDir()); err != nil {
			s.Error("Cannot save perf data: ", err)
		}
	}()

	perfValues.Set(
		perf.Metric{
			Name:      "JitterMark.p50",
			Unit:      "us",
			Direction: perf.SmallerIsBetter,
		}, jitterPercentile.p50)
	perfValues.Set(
		perf.Metric{
			Name:      "JitterMark.p99",
			Unit:      "us",
			Direction: perf.SmallerIsBetter,
		}, jitterPercentile.p99)
	perfValues.Set(
		perf.Metric{
			Name:      "JitterMark.max",
			Unit:      "us",
			Direction: perf.SmallerIsBetter,
		}, jitterPercentile.p100)

	perfValues.Set(
		perf.Metric{
			Name:      "CyclicBench.p50",
			Unit:      "us",
			Direction: perf.SmallerIsBetter,
		}, cyclicBenchResult.Median)
	perfValues.Set(
		perf.Metric{
			Name:      "CyclicBench.p99",
			Unit:      "us",
			Direction: perf.SmallerIsBetter,
		}, cyclicBenchResult.P99)
	perfValues.Set(
		perf.Metric{
			Name:      "CyclicBench.max",
			Unit:      "us",
			Direction: perf.SmallerIsBetter,
		}, cyclicBenchResult.Max)

}

// getJitterPercentileFromJittermarkResult parses jitter data from Jittermark result text.
// See example text in the unit test.
func getJitterPercentileFromJittermarkResult(resultText string) (audioSynthmarkJitterPercentile, error) {
	// Get jitter data from the first CSV block
	csvRegex := regexp.MustCompile(`(?s)CSV_BEGIN(.*?)CSV_END`)
	match := csvRegex.FindStringSubmatch(resultText)
	if match == nil {
		return audioSynthmarkJitterPercentile{}, errors.New("failed to find CSV data")
	}

	r := csv.NewReader(strings.NewReader(match[1]))
	r.FieldsPerRecord = -1 // Allow variable number of fields
	records, err := r.ReadAll()
	if err != nil {
		return audioSynthmarkJitterPercentile{}, errors.Wrap(err, "failed to read CSV data")
	}

	type jitterDataBin struct {
		usec        float64
		wakeupCount int64
	}
	var jitterData []jitterDataBin
	var totalWakeupCount int64
	// Parse each line into a jitter data bin and also calculate totalWakeupCount.
	// Ignore the first line (headers), and last line (CSV_END).
	for i := 1; i < len(records)-1; i++ {
		msecStr := strings.TrimSpace(records[i][2])
		msec, err := strconv.ParseFloat(msecStr, 64)
		if err != nil {
			return audioSynthmarkJitterPercentile{}, errors.Wrapf(err, "failed to parse msec %q to float at line %v", msecStr, i)
		}
		wakeupCountStr := strings.TrimSpace(records[i][3])
		wakeupCount, err := strconv.ParseInt(wakeupCountStr, 10, 64)
		if err != nil {
			return audioSynthmarkJitterPercentile{}, errors.Wrapf(err, "failed to parse wakeupCount %q to int at line %v", wakeupCountStr, i)
		}
		jitterData = append(jitterData, jitterDataBin{msec * 1000, wakeupCount})
		totalWakeupCount += wakeupCount
	}

	// Calculate percentile.
	// Note that jitterData is already sorted by usec in ascending order.
	// To calculate the percentile, go through each data bin, adding the wakeup count to cumulativeWakeupCount.
	// Once cumulativeWakeupCount reaches X% of totalWakeupCount, the usec of that bin is the X-th percentile.
	res := audioSynthmarkJitterPercentile{
		p50:  -1,
		p99:  -1,
		p100: -1,
	}
	var cumulativeWakeupCount int64
	for _, jitter := range jitterData {
		cumulativeWakeupCount += jitter.wakeupCount
		if res.p50 < 0 && cumulativeWakeupCount >= totalWakeupCount*50/100 {
			res.p50 = jitter.usec
		}
		if res.p99 < 0 && cumulativeWakeupCount >= totalWakeupCount*99/100 {
			res.p99 = jitter.usec
		}
		if res.p100 < 0 && cumulativeWakeupCount >= totalWakeupCount {
			res.p100 = jitter.usec
		}
	}
	return res, nil
}
