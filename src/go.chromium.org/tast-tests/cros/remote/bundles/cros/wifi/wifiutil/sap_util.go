// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifiutil

import (
	"context"
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"go.chromium.org/tast-tests/cros/common/network/ping"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/wifi/security"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/dutcfg"
	"go.chromium.org/tast-tests/cros/remote/wificell/tethering"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/ctxutil"
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

// SAPOnOffStressTestcase defines test parameters for Sof AP On/Off Stress Testcase.
type SAPOnOffStressTestcase struct {
	PrintableName string
	TetheringOpts []tethering.Option
	SecConfFac    security.ConfigFactory
	UseWpaCliAPI  bool // Use wpa_cli API to setup tethering.
}

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

// SAPAssocStressRound connects peer DUT to the softAP on the main DUT, then confirms connection by running a short ping burst.
func SAPAssocStressRound(ctx context.Context, tf *wificell.TestFixture, tetheringConf *tethering.Config) (retErr error) {
	_, err := tf.ConnectWifiFromDUT(ctx, wificell.PeerDUT1, tetheringConf.SSID, dutcfg.ConnSecurity(tetheringConf.SecConf))
	if err != nil {
		return errors.Wrap(err, "failed to connect to Soft AP")
	}
	// Defer disconnect just in case something breaks.
	defer func(ctx context.Context) {
		err = tf.DisconnectDUTFromWifi(ctx, wificell.PeerDUT1)
		if retErr != nil {
			// We can't overwrite ret value.
			if err != nil {
				// Double error, just log disconnect's one.
				testing.ContextLog(ctx, "Encountered error when disconnecting that cannot be returned: ", err)
			}
		} else {
			retErr = err
		}
	}(ctx)
	ctx, cancel := tf.ReserveForDisconnect(ctx)
	defer cancel()

	addrsReq := &wifi.GetIPv4AddrsRequest{
		InterfaceName: shillconst.ApInterfaceName,
	}
	addrsResp, err := tf.DUTWifiClient(wificell.DefaultDUT).GetIPv4Addrs(ctx, addrsReq)
	if err != nil {
		return errors.Wrap(err, "failed to get the IPv4 addresses")
	}
	if len(addrsResp.Ipv4) == 0 {
		return errors.New("no IP address returned")
	}
	addr, _, err := net.ParseCIDR(addrsResp.Ipv4[0])
	if err != nil {
		return errors.Wrapf(err, "failed to parse IP address %s", addrsResp.Ipv4[0])
	}
	res, err := tf.PingFromSpecificDUT(ctx, wificell.PeerDUT1, addr.String(), ping.Interval(0.1), ping.Count(3))
	if err != nil {
		return errors.Wrap(err, "failed to ping from Companion DUT to DUT")
	}
	// 50% * 3 packets means we're OK with losing one packet due to a random event, but not more packets.
	if err = wificell.VerifyPingResults(res, 50.0); err != nil {
		return errors.Wrap(err, "ping loss unsatisfactory")
	}
	return nil
}

// SAPOnOffStressTest runs Soft AP On/Off Stress Test.
func SAPOnOffStressTest(ctx context.Context, s *testing.State, tf *wificell.TestFixture, tc SAPOnOffStressTestcase,
	rounds int, thresholds ResourceThreshold, processes string, pv *perf.Values) error {
	iface, err := tf.DUTClientInterface(ctx, wificell.DefaultDUT)
	if err != nil {
		return errors.Wrap(err, "DUT: failed to get the client WiFi interface")
	}
	options := append([]tethering.Option{tethering.PriIface(iface)}, tc.TetheringOpts...)
	fac := tc.SecConfFac
	tf.UseWpaCliAPI(tc.UseWpaCliAPI)
	resInfo, err := GetResourceInfo(ctx, tf.DUT(wificell.DefaultDUT).Conn(), processes)
	if err != nil {
		return errors.Wrap(err, "failed to get resource info")
	}
	resInfos := []ResourceInfo{resInfo}

	// Make sure output is recorded even in case of error, this might be the reason of the issue.
	defer func(ctx context.Context) {
		if err := ReportResources(ctx, resInfos, s.OutDir(), fmt.Sprintf("%s.tsv", "sap_on_off_"+tc.PrintableName)); err != nil {
			s.Error("Failed to write resources report: ", err)
		}
	}(ctx)
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	var startupTimes, shutdownTimes []float64
	// We're running in a simple loop instead of s.Run() on purpose, we want to bail out on the first error.
	for i := 0; i < rounds; i++ {
		testing.ContextLogf(ctx, "Tethering round #%v", i+1)

		startupTime, shutdownTime, err := SAPOnOffStressRound(ctx, tf, options, fac)
		if err != nil {
			return errors.Wrap(err, "failure during stress round")
		}
		// Convert to a standard understandable by perf.
		startupTimes = append(startupTimes, float64(startupTime.Milliseconds())/1000)
		shutdownTimes = append(shutdownTimes, float64(shutdownTime.Milliseconds())/1000)
		resInfo, err := GetResourceInfo(ctx, tf.DUT(wificell.DefaultDUT).Conn(), processes)
		if err != nil {
			return errors.Wrap(err, "failed to get resource info")
		}
		// Check if pid of processes changed.
		if err := ValidatePids(resInfos[0], resInfo); err != nil {
			return errors.Wrap(err, "error while validating PIDs")
		}
		resInfos = append(resInfos, resInfo)
	}
	testing.ContextLog(ctx, "Start: ", resInfos[0].String())
	testing.ContextLog(ctx, "End:   ", resInfos[len(resInfos)-1].String())
	if err := ValidateResourceInfo(ctx, resInfos[0], resInfos[len(resInfos)-1], thresholds); err != nil {
		return errors.Wrap(err, "resource validation failed")
	}

	// Calculate the fastest, slowest, and average startup time.
	SummarizeExecutionTime(ctx, "sap_on_off_stress_startup_time_"+tc.PrintableName, pv, startupTimes)

	// Calculate the fastest, slowest, and average shutdown time.
	SummarizeExecutionTime(ctx, "sap_on_off_stress_shutdown_time_"+tc.PrintableName, pv, shutdownTimes)

	return nil
}

// SAPOnOffStressRound sets up tethering, makes sure teardown is run then runs actions from Assoc Stress round.
func SAPOnOffStressRound(ctx context.Context, tf *wificell.TestFixture, tetheringOpts []tethering.Option,
	secConfFac security.ConfigFactory) (startupTime, shutdownTime time.Duration, retErr error) {
	// Configure AP according to the testcase specs.
	tetheringConf, tetheringResp, err := tf.StartTethering(ctx, wificell.DefaultDUT, tetheringOpts, secConfFac)
	if err != nil {
		return 0, 0, errors.Wrap(err, "failed to start tethering session on DUT")
	}
	startupTime = tetheringResp.ExecutionTime.AsDuration()
	defer func(ctx context.Context) {
		tetheringResp, err = tf.StopTethering(ctx, wificell.DefaultDUT, tetheringConf)
		if retErr != nil {
			// We can't overwrite ret value.
			if err != nil {
				// Double error, just log disconnect's one.
				testing.ContextLog(ctx, "Encountered error when stopping tethering that cannot be returned: ", err)
			}
		} else {
			retErr = err
		}
		shutdownTime = tetheringResp.ExecutionTime.AsDuration()
	}(ctx)
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	testing.ContextLog(ctx, "Tethering session started")

	// Rest of the round is functionally identical to SAPAssocStressRound.
	err = SAPAssocStressRound(ctx, tf, tetheringConf)
	if err != nil {
		return 0, 0, errors.Wrap(err, "failed to associate to DUT")
	}
	return startupTime, 0, nil
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

// SummarizeExecutionTime summarizes stats for the execution time.
func SummarizeExecutionTime(ctx context.Context, name string, pv *perf.Values, samples []float64) {
	fastest := math.Inf(1)
	slowest := math.Inf(-1)
	var total float64
	for _, t := range samples {
		fastest = math.Min(fastest, t)
		slowest = math.Max(slowest, t)
		total += t
	}
	average := total / float64(len(samples))
	testing.ContextLogf(ctx, "%s (seconds): fastest=%f, slowest=%f, average=%f", name, fastest, slowest, average)

	pv.Set(perf.Metric{
		Name:      name,
		Variant:   "Fastest",
		Unit:      "seconds",
		Direction: perf.SmallerIsBetter,
	}, fastest)
	pv.Set(perf.Metric{
		Name:      name,
		Variant:   "Slowest",
		Unit:      "seconds",
		Direction: perf.SmallerIsBetter,
	}, slowest)
	pv.Set(perf.Metric{
		Name:      name,
		Variant:   "Average",
		Unit:      "seconds",
		Direction: perf.SmallerIsBetter,
	}, average)

}
