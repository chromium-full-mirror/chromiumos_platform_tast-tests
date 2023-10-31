// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/upstart"

	"go.chromium.org/tast-tests/cros/common/testexec"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// runTeensyLatencyTest runs the teensy_latency_test command with arguments
func runTeensyLatencyTest(ctx context.Context, args ...string) (out string, err error) {
	outBytes, err := testexec.CommandContext(ctx, "teensy_latency_test", args...).Output(testexec.DumpLogOnError)
	out = string(outBytes)
	if err != nil {
		return "", errors.Wrapf(err, "failed to run 'teensy_latency_test' with arguments %v", args)
	}
	return out, err
}

// extractMeasuredLatency extract measured latency from teensy_latency_test
func extractMeasuredLatency(ctx context.Context, latencyTestOut string) (latency int, err error) {
	latencyRegexp := regexp.MustCompile(`latency: diff (\d+)us diff`)

	latencyMatches := latencyRegexp.FindStringSubmatch(latencyTestOut)

	if latencyMatches == nil || len(latencyMatches) <= 1 {
		testing.ContextLogf(ctx, "Cannot find pattern %v in output: %s", latencyRegexp, latencyTestOut)
		return 0, errors.New("couldn't find measured latency in output")
	}

	latency, err = strconv.Atoi(latencyMatches[1])
	if err != nil {
		testing.ContextLog(ctx, "Cannot convert regexp match to int: ", latencyMatches)
		return 0, errors.Wrap(err, "failed to convert regexp matches to int")
	}

	return latency, nil
}

// extractReportedLatency extract reported latency from teensy_latency_test
func extractReportedLatency(ctx context.Context, latencyTestOut string) (latency int, err error) {
	latencyRegexp := regexp.MustCompile(`Reported [Ll]atency: (\d+)`)

	latencyMatches := latencyRegexp.FindStringSubmatch(latencyTestOut)

	if latencyMatches == nil || len(latencyMatches) <= 1 {
		testing.ContextLogf(ctx, "Cannot find pattern %v in output: %s", latencyRegexp, latencyTestOut)
		return 0, errors.New("couldn't find reported latency in output")
	}

	latency, err = strconv.Atoi(latencyMatches[1])
	if err != nil {
		testing.ContextLog(ctx, "Cannot convert regexp match to int: ", latencyMatches)
		return 0, errors.Wrap(err, "failed to convert regexp matches to int")
	}

	return latency, nil
}

// RunLatencyTest runs the teensy_latency_test with arguments and extract
// measured and reported latency values
func RunLatencyTest(ctx context.Context, args ...string) (measuredLatency, reportedLatency int, err error) {
	latencyTestOut, err := runTeensyLatencyTest(ctx, args...)
	if err != nil {
		return 0, 0, errors.Wrap(err, "failed to run latency test executable")
	}

	measuredLatency, err = extractMeasuredLatency(ctx, latencyTestOut)
	if err != nil {
		testing.ContextLogf(ctx, "Cannot extract measured latency from output: %s", latencyTestOut)
		return 0, 0, errors.Wrap(err, "failed to extract measured latency from output")
	}

	reportedLatency, err = extractReportedLatency(ctx, latencyTestOut)
	if err != nil {
		testing.ContextLogf(ctx, "Cannot extract reported latency from output: %s", latencyTestOut)
		return 0, 0, errors.Wrap(err, "failed to extract reported latency from output")
	}

	return measuredLatency, reportedLatency, nil
}

// RunCrasLatencyTest runs the latency test by playing on internal speaker from
// CRAS.
// This function automatically selects internal speaker as active.
func RunCrasLatencyTest(ctx context.Context) (measuredLatency, reportedLatency int, err error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()

	// Stop UI in advance for this test to avoid the node being selected by UI.
	if err := upstart.StopJob(ctx, "ui"); err != nil {
		testing.ContextLog(ctx, "Failed to stop ui: ", err)
		return 0, 0, errors.Wrap(err, "failed to stop ui")
	}
	defer upstart.EnsureJobRunning(cleanupCtx, "ui")

	cras, err := RestartCras(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Failed to connect to CRAS: ", err)
		return 0, 0, errors.Wrap(err, "failed to connect to cras")
	}

	if err := cras.SetActiveNodeByType(ctx, "INTERNAL_SPEAKER"); err != nil {
		testing.ContextLog(ctx, "Failed to set internal speaker active: ", err)
		return 0, 0, errors.Wrap(err, "failed to set internal speaker active")
	}

	return RunLatencyTest(ctx, "-c")
}

// RunAlsaLatencyTest runs the latency test by playing on internal speaker from
// ALSA.
// This function finds the ALSA device name from GetNodes to play into the
// internal speaker.
func RunAlsaLatencyTest(ctx context.Context) (measuredLatency, reportedLatency int, err error) {
	// find internal speaker device name
	var inputDev string
	{
		cras, err := NewCras(ctx)
		if err != nil {
			testing.ContextLog(ctx, "Failed to connect to cras: ", err)
			return 0, 0, errors.Wrap(err, "failed to connect to cras")
		}

		nodes, err := cras.GetNodes(ctx)
		if err != nil {
			testing.ContextLog(ctx, "Failed to obtain cras nodes: ", err)
			return 0, 0, errors.Wrap(err, "failed to obtain cras nodes")
		}
		for _, n := range nodes {
			if n.Type == "INTERNAL_SPEAKER" {
				inputDev = n.DeviceName
				break
			}
		}
		if inputDev == "" {
			testing.ContextLogf(ctx, "Failed to find internal speaker device: %+v", nodes)
			return 0, 0, errors.Wrap(err, "failed to find internal speaker device")
		}
		testing.ContextLogf(ctx, "Selected input device: %q", inputDev)
	}

	alsaDev := "hw:" + strings.Split(inputDev, ":")[2]

	return RunLatencyTest(ctx, "-a", alsaDev)
}
