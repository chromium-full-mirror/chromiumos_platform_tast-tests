// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifiutil

import (
	"context"
	"fmt"
	"math"
	"net"
	"time"

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
	"go.chromium.org/tast/core/testing"
)

// SAPOnOffStressTestcase defines test parameters for Sof AP On/Off Stress Testcase.
type SAPOnOffStressTestcase struct {
	PrintableName string
	TetheringOpts []tethering.Option
	SecConfFac    security.ConfigFactory
	UseWpaCliAPI  bool // Use wpa_cli API to setup tethering.
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
