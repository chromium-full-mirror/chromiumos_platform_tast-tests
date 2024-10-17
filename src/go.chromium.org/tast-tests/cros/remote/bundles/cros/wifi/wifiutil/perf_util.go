// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifiutil

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/network/firewall"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil/perfmanager"
	remotefirewall "go.chromium.org/tast-tests/cros/remote/network/firewall"
	"go.chromium.org/tast-tests/cros/remote/network/iperf"
	remoteiw "go.chromium.org/tast-tests/cros/remote/wifi/iw"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/router/common/support"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

func storePerfResults(ctx context.Context, finalResult *iperf.Result, results iperf.History, tag, outDir string, testType perfmanager.TestType) error {
	perfKeyVal, err := NewKeyValsFile(outDir)
	if err != nil {
		return errors.Wrap(err, "failed to create keyval file")
	}
	defer perfKeyVal.Close()

	var values []iperf.BitRate
	for _, sample := range results {
		values = append(values, sample.Throughput/iperf.Mbps)
	}
	pv := perf.NewValues()

	logPerfValues := func(label string, values []float64, dir perf.Direction, multi bool) {
		pv.Set(perf.Metric{
			Name:      label,
			Unit:      "Mbps",
			Direction: dir,
			Multiple:  multi,
		}, values...)
		testing.ContextLogf(ctx, "%s: %v", label, values)
	}
	logPerfValues(fmt.Sprintf("%s.%s_dev", tag, testType),
		[]float64{float64(finalResult.StdDeviation / iperf.Mbps)}, perf.SmallerIsBetter, false)
	valuesFloat64 := make([]float64, len(values))
	for i, v := range values {
		valuesFloat64[i] = float64(v)
	}
	logPerfValues(fmt.Sprintf("%s.%s", tag, testType), valuesFloat64, perf.BiggerIsBetter, true)
	perfKeyVal.WriteKeyVals(map[string]string{
		fmt.Sprintf("%s_%s__throughput{perf}", tag, testType): fmt.Sprintf("%0.2f+-%0.2f", finalResult.Throughput/iperf.Mbps, finalResult.StdDeviation/iperf.Mbps)})
	perfKeyVal.WriteKeyVals(map[string]string{
		fmt.Sprintf("%s_%s__dev{perf}", tag, testType): fmt.Sprintf("%f", finalResult.StdDeviation/iperf.Mbps)})

	return pv.Save(outDir)
}

// runPerf is meant to run perf test between any kind of the WiFi device.
func runPerf(ctx, cleanUpCtx context.Context, wd1, wd2, wd3 wificell.WiFiDevice,
	ifaceType wificell.IfaceType, outDir, tag string, testType perfmanager.TestType,
	version iperf.Version, port int) (_ *iperf.Result, err error) {

	staIface, _ := wd1.IfName(ctx, wificell.StaIfaceType)

	var peerIfaceType wificell.IfaceType
	switch ifaceType {
	case wificell.StaIfaceType:
		// Assumption is that STA will be talking to AP (infrastructure mode).
		peerIfaceType = wificell.APIfaceType
	default:
		peerIfaceType = ifaceType
	}

	var routerType support.RouterType
	switch wd2.Type() {
	case wificell.RouterDevice:
		router, ok := wd2.(*wificell.RouterData)
		if !ok {
			return nil, errors.Errorf("Router %v is not what it seems", wd2)
		}
		routerType = router.RouterType()
	case wificell.CrOSDevice:
		routerType = support.ChromeOST
	}

	wd1IP, err := wd1.IPv4Addrs(ctx, ifaceType)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get IP address")
	}
	wd2IP, err := wd2.IPv4Addrs(ctx, peerIfaceType)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get IP address")
	}
	testing.ContextLog(ctx, "Dev1 IPv4:", wd1IP)
	testing.ContextLog(ctx, "Dev2 IPv4:", wd2IP)

	// In 2-way setup, we won't get device pointer, so no Conn() either.
	var wd3Conn *ssh.Conn
	if wd3 != nil {
		wd3Conn = wd3.Conn()
	}

	manager, err := perfmanager.NewTestManager(ctx, wd1.Conn(), wd2.Conn(),
		wd3Conn, routerType, wd1IP[0].String(), wd2IP[0].String(), staIface)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get performance test manager")
	}

	testing.ContextLogf(ctx, "Performing [[ %s ]]", testType)
	config, err := manager.Config(routerType, testType, 0)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get the iperf/netperf configuration for test type %s", testType)
	}
	config.Version = version
	config.Port = port
	config.AutoClean = false
	session, err := manager.Session(ctx, testType)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get the iperf/netperf session for test type %s", testType)
	}
	finalResult, results, err := session.Run(ctx, config)
	if err != nil && !errors.Is(err, context.Canceled) {
		return nil, errors.Wrap(err, "failed to run session")
	}
	if len(results) == 0 {
		return nil, errors.Errorf("failed to take measurement for %s", testType)
	}

	return finalResult, storePerfResults(cleanUpCtx, finalResult, results, tag, outDir, testType)
}

// Cleanup helps clean up from unwanted processes.
func Cleanup(ctx context.Context, processes []string, devs []wificell.WiFiDevice) error {
	var err error
	for _, dev := range devs {
		for _, process := range processes {
			errors.Join(err, dev.Conn().CommandContext(ctx, "killall", "-9", "-q", process).Run())
		}
	}
	return err
}

// P2PPerf runs perf test using P2P interfaces.
func P2PPerf(ctx, cleanUpCtx context.Context, tf *wificell.TestFixture, p2pGO, p2pClient wificell.P2PWiFiDevice,
	outDir, tag string, testType perfmanager.TestType, version iperf.Version) (*iperf.Result, error) {
	// We assume here that cleanUpCtx is an uncacellable copy of ctx, but we have no guarantees
	// that the original context has been already shortened.
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	p2pGOIPAddress, err := p2pGO.IPv4Addrs(ctx, wificell.P2PIfaceType)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get P2P GO ip address")
	}

	p2pClientIPAddress, err := p2pClient.IPv4Addrs(ctx, wificell.P2PIfaceType)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get P2P GO ip address")
	}

	// p2pGOFirewallParams is a set of parameters needed for unblocking p2p tcp traffic on the p2p GO.
	var p2pGOFirewallParams = []firewall.RuleOption{
		firewall.OptionWait(5),
		firewall.OptionSource(p2pGOIPAddress[0].String()),
		firewall.OptionProto(firewall.L4ProtoTCP),
		firewall.OptionMatch(firewall.L4ProtoTCP),
		firewall.OptionJumpTarget(firewall.TargetAccept),
	}

	// p2pClientFirewallParams is a set of parameters needed for unblocking p2p tcp traffic on the p2p client.
	var p2pClientFirewallParams = []firewall.RuleOption{
		firewall.OptionWait(5),
		firewall.OptionSource(p2pClientIPAddress[0].String()),
		firewall.OptionProto(firewall.L4ProtoTCP),
		firewall.OptionMatch(firewall.L4ProtoTCP),
		firewall.OptionJumpTarget(firewall.TargetAccept),
	}

	firewallRunnerGO := remotefirewall.NewRemoteRunner(p2pGO.Conn())
	if err := firewallRunnerGO.ExecuteCommand(ctx, append(p2pGOFirewallParams, firewall.OptionAppendRule(firewall.InputChain))...); err != nil {
		return nil, errors.Wrap(err, "failed to set P2P GO iptable rule")
	}
	defer func(ctx context.Context) {
		err = errors.Join(err, firewallRunnerGO.ExecuteCommand(ctx,
			append(p2pGOFirewallParams, firewall.OptionDeleteRule(firewall.InputChain))...))
	}(cleanUpCtx)

	firewallRunnerClient := remotefirewall.NewRemoteRunner(p2pClient.Conn())
	if err := firewallRunnerClient.ExecuteCommand(ctx, append(p2pClientFirewallParams, firewall.OptionAppendRule(firewall.InputChain))...); err != nil {
		return nil, errors.Wrap(err, "failed to set P2P Client iptable rule")
	}
	defer func(ctx context.Context) {
		err = errors.Join(err, firewallRunnerClient.ExecuteCommand(ctx,
			append(p2pClientFirewallParams, firewall.OptionDeleteRule(firewall.InputChain))...))
	}(cleanUpCtx)

	freq, err := p2pGO.P2PFrequency(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get P2P frequency")
	}
	tag = fmt.Sprintf("%v_%v_MHz", tag, freq)

	return runPerf(ctx, cleanUpCtx, p2pGO, p2pClient, nil, wificell.P2PIfaceType, outDir, tag, testType, version, 5003)
}

// PerfRouter returns router device that will run perf.
func PerfRouter(tf *wificell.TestFixture) wificell.WiFiDevice {
	routerType := tf.Router().RouterType()
	if routerType == support.LegacyT {
		return tf.PcapDevice()
	}
	return tf.RouterDevice(wificell.DefaultRouter)
}

// InfraPerf runs perf test using Infrastructure (STA) connection.
func InfraPerf(ctx, cleanUpCtx context.Context, tf *wificell.TestFixture, wd wificell.WiFiDevice, powerSave bool, apIface *wificell.APIface,
	outDir, tag string, perfTestType perfmanager.TestType, version iperf.Version) (_ *iperf.Result, err error) {
	// We assume here that cleanUpCtx is an uncacellable copy of ctx, but we have no guarantees
	// that the original context has been already shortened.
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	wdAddress, err := wd.IPv4Addrs(ctx, wificell.P2PIfaceType)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get P2P GO ip address")
	}

	// firewallParams is a set of parameters needed for unblocking tcp traffic on the WiFi device.
	var firewallParams = []firewall.RuleOption{
		firewall.OptionWait(5),
		firewall.OptionSource(wdAddress[0].String()),
		firewall.OptionProto(firewall.L4ProtoTCP),
		firewall.OptionMatch(firewall.L4ProtoTCP),
		firewall.OptionJumpTarget(firewall.TargetAccept),
	}

	firewallRunner := remotefirewall.NewRemoteRunner(wd.Conn())
	if err := firewallRunner.ExecuteCommand(ctx, append(firewallParams,
		firewall.OptionAppendRule(firewall.InputChain))...); err != nil {
		return nil, errors.Wrap(err, "failed to set WiFi device iptables rule")
	}
	defer func(ctx context.Context) {
		err = errors.Join(err, firewallRunner.ExecuteCommand(ctx,
			append(firewallParams, firewall.OptionDeleteRule(firewall.InputChain))...))
	}(cleanUpCtx)

	// Create configTag which is used in the keyval.
	allTags := []string{tag}
	psTag := "PSon"
	if !powerSave {
		psTag = "PSoff"
	}
	allTags = append(allTags, psTag)
	apConfigDesc := apIface.Config().PerfDesc()
	allTags = append(allTags, apConfigDesc)
	allTags = append(allTags, tf.Router().RouterModel())
	configTag := strings.Join(allTags, "_")

	routerType := tf.Router().RouterType()
	iwr := remoteiw.NewRemoteRunner(wd.Conn())
	staIface, err := wd.IfName(ctx, wificell.StaIfaceType)

	signalLevel, err := iwr.WifiInterfaceSignalLevel(ctx, staIface)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get the signal level of the WiFi interface")
	}
	signalDesc := apConfigDesc + "_signal{perf}"
	perfKeyVal, err := NewKeyValsFile(outDir)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create keyval file")
	}
	defer perfKeyVal.Close()

	// Write perf keyval.
	perfKeyVal.WriteKeyVals(map[string]string{signalDesc: signalLevel})
	pv := perf.NewValues()
	defer func() {
		err = errors.Join(err, pv.Save(outDir))
	}()

	var ret *iperf.Result
	if err := tf.AssertNoDisconnect(ctx, wificell.DefaultDUT, func(ctx context.Context) error {
		if routerType == support.LegacyT {
			ret, err = runPerf(ctx, cleanUpCtx, wd, tf.RouterDevice(wificell.DefaultRouter), tf.PcapDevice(),
				wificell.StaIfaceType, outDir, configTag, perfTestType, version, 5001)
		} else {
			ret, err = runPerf(ctx, cleanUpCtx, wd, tf.RouterDevice(wificell.DefaultRouter), nil,
				wificell.StaIfaceType, outDir, configTag, perfTestType, version, 5001)
		}
		return err
	}); err != nil {
		return ret, errors.Wrap(err, "failed to run performance test")
	}
	return ret, nil
}
