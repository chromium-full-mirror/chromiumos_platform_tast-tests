// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package iperf

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
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

// String implements stringer interface to facilitate logging.
func (r Result) String() string {
	if len(r.Jitter) > 0 {
		sort.Slice(r.Jitter, func(i, j int) bool {
			return r.Jitter[i] < r.Jitter[j]
		})
		// Pick 90th percentile value. We want to exclude top 10% of recorded jitters,
		// so we can be pretty convinced that 90% of our traffic fits under the maximum acceptable jitter threshold.
		jitter := r.Jitter[(len(r.Jitter)-1)*9/10]
		return fmt.Sprintf("{Duration: %v, Throughput: %.0f+-%.3fMbps (S->C: %.0fMbps C->S: %.0fMbps), Loss: %3.2f%%, Jitter: %v}",
			r.Duration, r.Throughput/Mbps, r.StdDeviation/Mbps, r.ServerToClient/Mbps, r.ClientToServer/Mbps, r.PercentLoss*100.0, jitter)
	}
	return fmt.Sprintf("{Duration: %v, Throughput: %.0f+-%.3fMbps (S->C: %.0fMbps C->S: %.0fMbps), Loss: %3.2f%%}",
		r.Duration, r.Throughput/Mbps, r.StdDeviation/Mbps, r.ServerToClient/Mbps, r.ClientToServer/Mbps, r.PercentLoss*100.0)
}

// Equals facilitates comparisons as DeepEqual doesn't care about proper floats comparison.
func (r *Result) Equals(p *Result) (bool, error) {
	const delta = 0.0000001
	if r == nil && p == nil {

		return true, nil
	}
	if r == nil || p == nil {
		return false, errors.New("one of the pointers is nil")
	}
	if r.Duration.Nanoseconds()-p.Duration.Nanoseconds() > 1 {
		return false, errors.Errorf("Duration differs too much: %v vs %v", r.Duration.Nanoseconds(), p.Duration.Nanoseconds())
	}
	if math.Abs(float64(r.Throughput-p.Throughput)) > delta {
		return false, errors.Errorf("Throughput differs too much: %v vs %v", r.Throughput, p.Throughput)
	}
	if math.Abs(float64(r.ClientToServer-p.ClientToServer)) > delta {
		return false, errors.Errorf("C->S Throughput differs too much: %v vs %v", r.ClientToServer, p.ClientToServer)
	}
	if math.Abs(float64(r.ServerToClient-p.ServerToClient)) > delta {
		return false, errors.Errorf("S->C Throughput differs too much: %v vs %v", r.ServerToClient, p.ServerToClient)
	}
	if math.Abs(float64(r.PercentLoss-p.PercentLoss)) > delta {
		return false, errors.Errorf("Loss differs too much: %v vs %v", r.PercentLoss, p.PercentLoss)
	}
	if math.Abs(float64(r.StdDeviation-p.StdDeviation)) > delta {
		return false, errors.Errorf("St. dev differs too much: %v vs %v", r.StdDeviation, p.StdDeviation)
	}
	// Ignore jitter results for now they are incomparable.
	return true, nil
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
	switch config.Version {
	case Version2:
		return newResultFromV2Output(ctx, output, config)
	case Version3:
		return newResultFromV3Output(ctx, output, config)
	default:
		return nil, errors.Errorf("unknown iperf version: %v", config.Version)
	}
}

func newResultFromV2Output(ctx context.Context, output string, config *Config) (*Result, error) {
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

func newResultFromV3Output(ctx context.Context, output string, config *Config) (*Result, error) {
	switch config.Protocol {
	case ProtocolUDP:
		return newResultFromV3UDPOutput(ctx, output, config)
	case ProtocolTCP:
		return newResultFromV3TCPOutput(ctx, output, config)
	default:
		return nil, errors.Errorf("unable to parse protocol %v", config.Protocol)
	}
}

func newResultFromV3TCPOutput(ctx context.Context, output string, config *Config) (*Result, error) {
	totalResult := Result{}
	// [SUM]   0.00-10.02  sec  77.4 MBytes  64.8 Mbits/sec                  receiver
	summaryRE := regexp.MustCompile(`\[SUM]\s*([\d\.]*-[\d\.]*)\s*sec\s*([\d\.]*)\s*(\w)Bytes\s*[\d\.]*\s*\wbits/sec\s*[\d\.]*\s*receiver`)

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if summaryRE.MatchString(line) {
			matches := summaryRE.FindStringSubmatch(line)
			if len(matches) < 4 {
				return nil, errors.Errorf("not enough matches :%+v", matches)
			}
			duration, _ := parseInterval(matches[1])
			bytes, _ := strconv.ParseFloat(matches[2], 64)
			switch matches[3] {
			case "G":
				bytes *= 1024 * 1024 * 1024
			case "M":
				bytes *= 1024 * 1024
			case "K":
				bytes *= 1024
			}
			tput := math.Round(bytes / duration)
			totalResult.Duration += time.Duration(duration * float64(time.Second))
			totalResult.Throughput += BitRate(tput)
		}
	}
	return &Result{
		Duration:       totalResult.Duration,
		Throughput:     8 * totalResult.Throughput,
		ClientToServer: 8 * totalResult.ClientToServer,
		ServerToClient: 8 * totalResult.ServerToClient,
	}, nil

}

func newResultFromV3UDPOutput(ctx context.Context, output string, config *Config) (*Result, error) {
	// [  5][TX-C]   0.00-10.04  sec  47.3 MBytes  39.5 kbits/sec  0.386 ms  30528/64799 (47%)  receiver
	summaryRE := regexp.MustCompile(`\[\s*\d*]\[([\w-]*)]\s*([\d\.]*-[\d\.]*)\s*sec\s*([\d\.]*)\s*(\w)Bytes\s*([\d\.]*)\s*\wbits/sec\s*([\d\.]*)\s*(\w*)\s*(\d*)/(\d*)\s*\([\de+\.]*%\)\s*(\w+)`)
	partialRE := regexp.MustCompile(`\[\s*\d*]\[[\w-]*]\s*[\d\.]*-[\d\.]*\s*sec\s*[\d\.]*\s*\w*\s*[\d\.]*\s*\wbits/sec\s*([\de+\.]*)\s*(\w*)`)
	totalResult := Result{}
	var totalDgrams, totalLoss uint64
	var allErrors error
	count := 0

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if summaryRE.MatchString(line) {
			matches := summaryRE.FindStringSubmatch(line)
			if len(matches) < 11 {
				return nil, errors.Errorf("not enough matches :%+v", matches)
			}
			if matches[10] == "sender" {
				continue
			}
			duration, _ := parseInterval(matches[2])
			bytes, _ := strconv.ParseFloat(matches[3], 64)
			switch matches[4] {
			case "G":
				bytes *= 1024 * 1024 * 1024
			case "M":
				bytes *= 1024 * 1024
			case "k":
				bytes *= 1024
			}
			loss, _ := strconv.ParseUint(matches[8], 10, 64)
			dgrams, _ := strconv.ParseUint(matches[9], 10, 64)
			tput := math.Round(bytes / duration)
			totalResult.Duration += time.Duration(duration * float64(time.Second))
			totalResult.Throughput += BitRate(tput)
			switch matches[1] {
			case "TX-C":
				totalResult.ClientToServer += BitRate(tput)
			case "RX-C":
				totalResult.ServerToClient += BitRate(tput)
			}
			totalDgrams += dgrams
			totalLoss += loss

			count++
		} else if partialRE.MatchString(line) {
			matches := partialRE.FindStringSubmatch(line)
			jitter, _ := strconv.ParseFloat(matches[1], 64)
			switch matches[2] {
			case "ms":
				totalResult.Jitter = append(totalResult.Jitter, time.Duration(jitter*float64(time.Millisecond)))
			case "us":
				totalResult.Jitter = append(totalResult.Jitter, time.Duration(jitter*float64(time.Microsecond)))
			}
		}
	}
	expectedCount := config.PortCount
	if config.Bidirectional {
		expectedCount *= 2
		totalResult.Duration /= 2
	}

	if count != expectedCount {
		return nil, errors.Join(allErrors, errors.Errorf("missing data: got %v lines, want %v; iperf client command output: %s", count, expectedCount, output))
	}

	if totalResult.Duration == 0.0 {
		return nil, errors.Wrapf(allErrors, "invalid total duration: got %v, want > 0.0", totalResult.Duration)
	}

	// Get the total duration for each port.
	totalResult.Duration = totalResult.Duration / time.Duration(config.PortCount)
	return &Result{
		Duration:       totalResult.Duration,
		PercentLoss:    float64(totalLoss) / float64(totalDgrams),
		Throughput:     8 * totalResult.Throughput,
		ClientToServer: 8 * totalResult.ClientToServer,
		ServerToClient: 8 * totalResult.ServerToClient,
		Jitter:         totalResult.Jitter,
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
