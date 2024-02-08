// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package iperf

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast/core/errors"
)

const (
	localAddrIndex      = 1
	localPortIndex      = 2
	logIDIndex          = 5
	intervalIndex       = 6
	dataTransferedIndex = 7
	jitterIndex         = 9
	percentLossIndex    = 12

	fieldCount    = 9
	fieldCountUDP = 14
)

// Result represents an aggregated set of Iperf results.
type Result struct {
	Duration       time.Duration
	Throughput     BitRate
	ClientToServer BitRate
	ServerToClient BitRate
	PercentLoss    float64
	StdDeviation   BitRate
	Jitter         []time.Duration
}

func isClientToServer(localAddr, localPort string, config *Config) bool {
	// If local address and port match then this data is traveling from client to server.
	// E.g. if port and address both correspond to the server.
	if localAddr == config.ServerIP && localPort == strconv.Itoa(config.Port) {
		return true
	} else if localAddr == config.ClientIP && localPort != strconv.Itoa(config.Port) {
		return true
	}

	// If port and address are mismatched, then this is data traveling in the “reverse” direction.
	return false
}

func newResultFromOutput(ctx context.Context, output string, config *Config) (*Result, error) {
	var totalThroughput float64
	var totalClientToServer float64
	var totalServerToClient float64
	var totalDuration float64
	var totalLoss float64
	var totalJitter []time.Duration

	var allErrors error
	count := 0
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		fields := strings.Split(line, ",")

		// only use client side results for UDP
		if len(fields) < fieldCount || (config.Protocol == ProtocolUDP && len(fields) < fieldCountUDP) {
			continue
		}

		// ignore summary lines
		if logID, err := strconv.Atoi(fields[logIDIndex]); err != nil || logID < 0 {
			continue
		}

		byteCount, err := strconv.ParseFloat(fields[dataTransferedIndex], 64)
		if err != nil {
			allErrors = errors.Wrapf(allErrors, "failed to parse bytes from %q: %v ", fields[dataTransferedIndex], err) // NOLINT
			continue
		}

		duration, err := parseInterval(fields[intervalIndex])
		if err != nil {
			allErrors = errors.Wrapf(allErrors, "failed to parse duration from: %q: %v ", fields[intervalIndex], err) // NOLINT
			continue
		}

		var loss, jitter float64
		if config.Protocol == ProtocolUDP {
			// The extra counters are (starting from index 8): speed, jitter (ms), pkt lost, datagrams, % loss, Out of order.
			// As taken from: https://sourceforge.net/p/iperf/code/HEAD/tree/tags/2.0.5/src/ReportCSV.c#l85
			loss, err = strconv.ParseFloat(fields[percentLossIndex], 64)
			if err != nil {
				allErrors = errors.Wrapf(allErrors, "failed to parse loss from %q: %v ", fields[percentLossIndex], err) // NOLINT
				continue
			}
			jitter, err = strconv.ParseFloat(fields[jitterIndex], 64)
			if err != nil {
				allErrors = errors.Wrapf(allErrors, "failed to parse jitter from %q: %v ", fields[jitterIndex], err) // NOLINT
				continue
			}
		}

		totalDuration += duration
		totalThroughput += byteCount / duration
		totalLoss += loss
		totalJitter = append(totalJitter, time.Duration(jitter*float64(time.Millisecond)))

		localAddr := fields[localAddrIndex]
		localPort := fields[localPortIndex]
		if isClientToServer(localAddr, localPort, config) {
			totalClientToServer += byteCount / duration
		} else {
			totalServerToClient += byteCount / duration
		}

		count++
	}

	// OpenWrt iperf clients and the ones connected to OpenWrt iperf server
	// don't show server side results for UDP. The fallback approach to
	// calculate the throughput is to use client side results, although
	// they are missing packet loss columns.
	if config.Protocol == ProtocolUDP && count == 0 {
		for _, line := range lines {
			fields := strings.Split(line, ",")
			if len(fields) != fieldCount {
				continue
			}

			// ignore summary lines
			if logID, err := strconv.Atoi(fields[logIDIndex]); err != nil || logID < 0 {
				continue
			}

			byteCount, err := strconv.ParseFloat(fields[dataTransferedIndex], 64)
			if err != nil {
				allErrors = errors.Wrapf(allErrors, "failed to parse bytes from %q: %v ", fields[dataTransferedIndex], err) // NOLINT
				continue
			}

			duration, err := parseInterval(fields[intervalIndex])
			if err != nil {
				allErrors = errors.Wrapf(allErrors, "failed to parse duration from: %q: %v ", fields[intervalIndex], err) // NOLINT
				continue
			}

			totalDuration += duration
			totalThroughput += byteCount / duration

			localAddr := fields[localAddrIndex]
			localPort := fields[localPortIndex]
			if isClientToServer(localAddr, localPort, config) {
				totalClientToServer += byteCount / duration
			} else {
				totalServerToClient += byteCount / duration
			}

			count++
		}
	}

	averageDuration := totalDuration
	expectedCount := config.PortCount
	if config.Bidirectional {
		expectedCount *= 2
		averageDuration /= 2
	}

	if count != expectedCount {
		return nil, errors.Wrapf(allErrors, "missing data: got %v lines, want %v; iperf client command output: %s", count, expectedCount, output)
	}

	if totalDuration == 0.0 {
		return nil, errors.Wrapf(allErrors, "invalid total duration: got %f, want > 0.0", totalDuration)
	}

	// Get the total duration for each port.
	averageDuration = averageDuration / float64(config.PortCount)
	return &Result{
		Duration:       time.Duration(averageDuration),
		PercentLoss:    totalLoss / float64(count),
		Throughput:     8 * BitRate(totalThroughput),
		ClientToServer: 8 * BitRate(totalClientToServer),
		ServerToClient: 8 * BitRate(totalServerToClient),
		Jitter:         totalJitter,
	}, nil
}

// parseInterval returns the duration from an Iperf interval or -1 if it was unable to parse.
func parseInterval(interval string) (float64, error) {
	bounds := strings.Split(interval, "-")
	if len(bounds) != 2 {
		return 0, errors.Errorf("unable to split duration interval: %v", interval)
	}

	start, err := strconv.ParseFloat(bounds[0], 64)
	if err != nil {
		return 0, errors.Errorf("unable to parse duration start: %v", bounds[0])
	}

	end, err := strconv.ParseFloat(bounds[1], 64)
	if err != nil {
		return 0, errors.Errorf("unable to parse duration end: %v", bounds[1])
	}

	duration := end - start
	if duration == 0 {
		return 0, errors.Errorf("parsed interval is empty: %s", interval)
	}

	return duration, nil
}

// NewResultFromHistory returns the average from a set of results.
func NewResultFromHistory(samples []*Result) (*Result, error) {
	count := len(samples)
	if count == 0 {
		return nil, errors.New("received empty samples slice")
	}

	var totalDuration time.Duration
	var meanThroughput float64
	var meanClientToServer float64
	var meanServerToClient float64
	var meanLoss float64
	var jitter []time.Duration
	var stdDev float64

	for _, sample := range samples {
		totalDuration += sample.Duration
		meanThroughput += float64(sample.Throughput) / float64(count)
		meanServerToClient += float64(sample.ServerToClient) / float64(count)
		meanClientToServer += float64(sample.ClientToServer) / float64(count)
		meanLoss += sample.PercentLoss / float64(count)
		jitter = append(jitter, sample.Jitter...)
	}

	for _, sample := range samples {
		stdDev += math.Pow(float64(sample.Throughput)-meanThroughput, 2)
	}

	stdDev = math.Sqrt(stdDev / float64(count))

	return &Result{
		Duration:       totalDuration,
		Throughput:     BitRate(meanThroughput),
		ClientToServer: BitRate(meanClientToServer),
		ServerToClient: BitRate(meanServerToClient),
		PercentLoss:    meanLoss,
		Jitter:         jitter,
		StdDeviation:   BitRate(stdDev),
	}, nil
}
