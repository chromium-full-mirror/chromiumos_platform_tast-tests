// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifiutil

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/crypto/certificate"
	"go.chromium.org/tast-tests/cros/common/network/ping"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/wifi/security"
	"go.chromium.org/tast-tests/cros/remote/wifi/iw"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/dhcp"
	"go.chromium.org/tast-tests/cros/remote/wificell/dutcfg"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/remote/wificell/pcap"
	"go.chromium.org/tast-tests/cros/remote/wificell/router"
	"go.chromium.org/tast-tests/cros/remote/wificell/router/common/support"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

// ContParam holds all parameters for the continuity test.
type ContParam struct {
	ApOpts     [2][]hostapd.Option
	SecConfFac security.ConfigFactory
	EnableFT   bool
	Rounds     int
	Param      interface{}
}

// ContTest holds all variables to be accessible for the whole continuity test.
type ContTest struct {
	tf             *wificell.TestFixture
	r              router.Standard
	pcap           support.Capture
	clientMAC      string
	mac            [2]net.HardwareAddr
	apOps          [2][]hostapd.Option
	ap             [2]*hostapd.Server
	dserv          *dhcp.Server
	ftEnabled      bool
	iface          string
	restoreBgAndFg func() error
	servicePath    string
}

var apID int

var (
	serverIP    = net.IPv4(192, 168, 0, 254)
	startIP     = net.IPv4(192, 168, 0, 1)
	endIP       = net.IPv4(192, 168, 0, 128)
	broadcastIP = net.IPv4(192, 168, 0, 255)
	mask        = net.IPv4Mask(255, 255, 255, 0)
)

// Cert1 defines a certificate used for testing.
var Cert1 = certificate.TestCert1()

func reserveForRelease(ctx context.Context) (context.Context, func()) {
	return ctxutil.Shorten(ctx, 10*time.Second)
}

func hasFTSupport(ctx context.Context, conn *ssh.Conn) (bool, error) {
	phys, _, err := iw.NewRemoteRunner(conn).ListPhys(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to check SME capability")
	}
	for _, p := range phys {
		for _, c := range p.Commands {
			// A DUT which has SME capability should support FT.
			if c == "authenticate" {
				return true, nil
			}
			// A full-mac driver that supports update_ft_ies functions also supports FT.
			if c == "update_ft_ies" {
				return true, nil
			}
		}
	}
	return false, nil
}

func setupPcapOnRouter(ctx context.Context, r support.Capture,
	apName string, apConf *hostapd.Config, ds *destructorStack) error {
	freqOps, err := apConf.PcapFreqOptions()
	if err != nil {
		return errors.Wrap(err, "failed to get Freq Opts")
	}
	capturer, err := r.StartCapture(ctx, apName, apConf.Channel, apConf.Is6GHz, freqOps)
	if err != nil {
		return errors.Wrap(err, "failed to start capturer")
	}
	ds.push(func() error {
		return r.StopCapture(ctx, capturer)
	})
	return nil
}

// ServerIP returns server IP used for the test.
func ServerIP() string {
	return serverIP.String()
}

// ContinuityTestInitialSetup performs the initial setup of the test environment.
func ContinuityTestInitialSetup(ctx context.Context, tf *wificell.TestFixture, param ContParam) (context.Context, *ContTest, DestructorStackDestroyF, error) {
	ct := &ContTest{tf: tf}
	ds, destroyIfNotExported := newDestructorStack()
	defer destroyIfNotExported()

	var err error
	ctx, ct.restoreBgAndFg, err = ct.tf.WifiClient().TurnOffBgAndFgscan(ctx)
	if err != nil {
		return ctx, nil, nil, errors.Wrap(err, "failed to turn off the background and/or foreground scan")
	}
	ds.push(func() (err error) {
		if err := ct.restoreBgAndFg(); err != nil {
			return errors.Wrap(err, "failed to restore the background and/or foreground scan config")
		}
		return nil
	})

	ct.clientMAC, err = tf.ClientHardwareAddr(ctx)
	if err != nil {
		return ctx, nil, nil, errors.Wrap(err, "unable to get DUT MAC address")
	}

	ftResp, err := ct.tf.WifiClient().GetGlobalFTProperty(ctx, &empty.Empty{})
	if err != nil {
		return ctx, nil, nil, errors.Wrap(err, "failed to get the global FT property")
	}
	ct.ftEnabled = ftResp.Enabled

	// TODO(b/190630644): Add HWDeps to cover such devices.
	if param.EnableFT {
		supported, err := hasFTSupport(ctx, tf.DUTConn(wificell.DefaultDUT))
		if err != nil {
			return ctx, nil, nil, errors.Wrap(err, "failed to check FT support")
		}
		if !supported {
			return ctx, nil, nil, errors.New("unable to run FT test on device not supporting FT")
		}
	}

	// Turn on the global FT.
	if _, err := tf.WifiClient().SetGlobalFTProperty(ctx, &wifi.SetGlobalFTPropertyRequest{Enabled: param.EnableFT}); err != nil {
		return ctx, nil, nil, errors.Wrap(err, "failed to set the global FT property")
	}
	ds.push(func() (err error) {
		if _, err := ct.tf.WifiClient().SetGlobalFTProperty(ctx, &wifi.SetGlobalFTPropertyRequest{Enabled: ct.ftEnabled}); err != nil {
			return errors.Wrapf(err, "failed to set global FT property back to %v", ct.ftEnabled)
		}
		return nil
	})

	ct.r, err = tf.StandardRouter()
	if err != nil {
		return ctx, nil, nil, errors.Wrap(err, "failed to get router")
	}
	ct.pcap = tf.PcapRouter()

	br, err := tf.GetBridgesOnRouterID(wificell.DefaultRouter)
	if err != nil {
		return ctx, nil, nil, errors.Wrap(err, "failed to get bridge names on router")
	}

	// Bind the two ends of the veth to the two bridges.
	for i := 0; i < 2; i++ {
		ct.mac[i], err = hostapd.RandomMAC()
		if err != nil {
			return ctx, nil, nil, errors.Wrap(err, "failed to get a random mac address")
		}
	}

	var (
		id0 = hex.EncodeToString(ct.mac[0])
		id1 = hex.EncodeToString(ct.mac[1])
	)
	const (
		key0 = "1f1e1d1c1b1a191817161514131211100f0e0d0c0b0a09080706050403020100"
		key1 = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
		mdID = "a1b2"
	)

	ssid := hostapd.RandomSSID("TAST_ROAM_CONT_")

	// Basic config
	for i := 0; i < 2; i++ {
		ct.apOps[i] = append(ct.apOps[i], hostapd.SSID(ssid), hostapd.BSSID(ct.mac[i].String()), hostapd.Bridge(br[i]))
		ct.apOps[i] = append(ct.apOps[i], param.ApOpts[i]...)
	}

	// FT Enabled
	if param.EnableFT {
		ct.apOps[0] = append(ct.apOps[0], hostapd.MobilityDomain(mdID), hostapd.NASIdentifier(id0), hostapd.R1KeyHolder(id0),
			hostapd.R0KHs(fmt.Sprintf("%s %s %s", ct.mac[1], id1, key0)),
			hostapd.R1KHs(fmt.Sprintf("%s %s %s", ct.mac[1], ct.mac[1], key1)))
		ct.apOps[1] = append(ct.apOps[1], hostapd.MobilityDomain(mdID), hostapd.NASIdentifier(id1), hostapd.R1KeyHolder(id1),
			hostapd.R0KHs(fmt.Sprintf("%s %s %s", ct.mac[0], id0, key1)),
			hostapd.R1KHs(fmt.Sprintf("%s %s %s", ct.mac[0], ct.mac[0], key0)))
	}

	// Security enabled
	if param.SecConfFac != nil {
		ap0SecConf, err := param.SecConfFac.Gen()
		if err != nil {
			return ctx, nil, nil, errors.Wrap(err, "failed to generate security config")
		}
		ct.apOps[0] = append(ct.apOps[0], hostapd.SecurityConfig(ap0SecConf))
		ap1SecConf, err := param.SecConfFac.Gen()
		if err != nil {
			return ctx, nil, nil, errors.Wrap(err, "failed to generate security config")
		}
		ct.apOps[1] = append(ct.apOps[1], hostapd.SecurityConfig(ap1SecConf))
	}

	ap0Conf, err := hostapd.NewConfig(ct.apOps[0]...)
	if err != nil {
		return ctx, nil, nil, errors.Wrap(err, "failed to generate the hostapd config for AP0")
	}
	ap0Name := tf.UniqueAPName()
	if err = setupPcapOnRouter(ctx, ct.r, ap0Name, ap0Conf, ds); err != nil {
		return ctx, nil, nil, errors.Wrap(err, "failed to setup pcap")
	}

	if err = setupPcapOnRouter(ctx, ct.pcap, "monitor"+ap0Name, ap0Conf, ds); err != nil {
		return ctx, nil, nil, errors.Wrap(err, "failed to setup monitor")
	}

	ct.iface, err = ct.tf.DUTWifiClient(wificell.DefaultDUT).Interface(ctx)
	if err != nil {
		return ctx, nil, nil, errors.Wrap(err, "failed to get interface from the DUT")
	}

	dutCapturer, err := pcap.StartCapturer(ctx, tf.DUTConn(wificell.DefaultDUT), "dut", ct.iface, "/var/log/")
	if err != nil {
		return ctx, nil, nil, errors.Wrap(err, "failed to start DUT capturer")
	}
	ds.push(func() error {
		return dutCapturer.Close(ctx)
	})

	testing.ContextLog(ctx, "Starting the first AP on ", br[0])

	ct.ap[0], err = ct.r.StartHostapd(ctx, ap0Name, ap0Conf)
	if err != nil {
		return ctx, nil, nil, errors.Wrap(err, "failed to start the hostapd server on the first AP")
	}
	ds.push(func() error {
		if err := ct.r.StopHostapd(ctx, ct.ap[0]); err != nil {
			return errors.Wrap(err, "failed to stop the hostapd server on the first AP")
		}
		return nil
	})

	testing.ContextLogf(ctx, "Starting the DHCP server on %s, serverIP=%s", br[0], serverIP)
	ct.dserv, err = ct.r.StartDHCP(ctx, ap0Name, br[0], startIP, endIP, serverIP, broadcastIP, mask, nil)
	if err != nil {
		return ctx, nil, nil, errors.Wrap(err, "failed to start the DHCP server")
	}
	ds.push(func() error {
		if err := ct.r.StopDHCP(ctx, ct.dserv); err != nil {
			return errors.Wrap(err, "failed to stop the DHCP server")
		}
		return nil
	})

	connResp, err := tf.ConnectWifi(ctx, ct.ap[0].Config().SSID, dutcfg.ConnSecurity(ct.ap[0].Config().SecurityConfig))
	if err != nil {
		return ctx, nil, nil, errors.Wrap(err, "failed to connect to the AP")
	}
	ds.push(func() error {
		if err := ct.tf.CleanDisconnectWifi(ctx); err != nil {
			return errors.Wrap(err, "failed to disconnect from the AP")
		}
		return nil
	})
	ct.servicePath = connResp.ServicePath

	if err := tf.PingFromDUT(ctx, serverIP.String(), ping.Count(3), ping.Interval(0.3)); err != nil {
		return ctx, nil, nil, errors.Wrap(err, "failed to ping from the DUT")
	}
	testing.ContextLog(ctx, "Connected to the first AP")

	return ctx, ct, ds.export().destroy, nil
}

// ContinuityTestSetupFinalize finalizes the setup of the test environment.
func (ct *ContTest) ContinuityTestSetupFinalize(ctx context.Context) (context.Context, DestructorStackDestroyF, error) {
	testing.ContextLog(ctx, "Starting the second AP")
	ds, destroyIfNotExported := newDestructorStack()
	defer destroyIfNotExported()
	ap1Name := ct.tf.UniqueAPName()
	ap1Conf, err := hostapd.NewConfig(ct.apOps[1]...)
	if err != nil {
		return ctx, nil, errors.Wrap(err, "failed to generate the hostapd config for AP1")
	}

	if err = setupPcapOnRouter(ctx, ct.r, ap1Name, ap1Conf, ds); err != nil {
		return ctx, nil, errors.Wrap(err, "failed to setup pcap")
	}

	if err = setupPcapOnRouter(ctx, ct.pcap, "monitor"+ap1Name, ap1Conf, ds); err != nil {
		return ctx, nil, errors.Wrap(err, "failed to setup monitor")
	}

	ct.ap[1], err = ct.r.StartHostapd(ctx, ap1Name, ap1Conf)
	if err != nil {
		return ctx, nil, errors.Wrap(err, "failed to start the hostapd server on the second AP")
	}
	ds.push(func() error {
		if err := ct.r.StopHostapd(ctx, ct.ap[1]); err != nil {
			return errors.Wrap(err, "failed to stop the hostapd server on the second AP")
		}
		return nil
	})

	return ctx, ds.export().destroy, nil
}

// ContinuityRound runs one round of the test: scans if necessary and attempts roaming.
func (ct *ContTest) ContinuityRound(ctx context.Context, round int) error {
	var props [][]*wificell.ShillProperty = [][]*wificell.ShillProperty{{{
		Property:       shillconst.ServicePropertyWiFiRoamState,
		ExpectedValues: []interface{}{shillconst.RoamStateConfiguration},
		Method:         wifi.ExpectShillPropertyRequest_ON_CHANGE,
	}, {
		Property:       shillconst.ServicePropertyWiFiRoamState,
		ExpectedValues: []interface{}{shillconst.RoamStateReady},
		Method:         wifi.ExpectShillPropertyRequest_ON_CHANGE,
	}, {
		Property:       shillconst.ServicePropertyWiFiRoamState,
		ExpectedValues: []interface{}{shillconst.RoamStateIdle},
		Method:         wifi.ExpectShillPropertyRequest_ON_CHANGE,
	}, {
		Property:       shillconst.ServicePropertyWiFiBSSID,
		ExpectedValues: []interface{}{ct.mac[1].String()},
		Method:         wifi.ExpectShillPropertyRequest_CHECK_ONLY,
	}}, {{
		Property:       shillconst.ServicePropertyWiFiRoamState,
		ExpectedValues: []interface{}{shillconst.RoamStateConfiguration},
		Method:         wifi.ExpectShillPropertyRequest_ON_CHANGE,
	}, {
		Property:       shillconst.ServicePropertyWiFiRoamState,
		ExpectedValues: []interface{}{shillconst.RoamStateReady},
		Method:         wifi.ExpectShillPropertyRequest_ON_CHANGE,
	}, {
		Property:       shillconst.ServicePropertyWiFiRoamState,
		ExpectedValues: []interface{}{shillconst.RoamStateIdle},
		Method:         wifi.ExpectShillPropertyRequest_ON_CHANGE,
	}, {
		Property:       shillconst.ServicePropertyWiFiBSSID,
		ExpectedValues: []interface{}{ct.mac[0].String()},
		Method:         wifi.ExpectShillPropertyRequest_CHECK_ONLY,
	}}}

	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	waitForProps, err := ct.tf.WifiClient().ExpectShillProperty(waitCtx, ct.servicePath, props[round%2], []string{shillconst.ServicePropertyIsConnected})
	if err != nil {
		return errors.Wrap(err, "failed to create a property watcher")
	}
	testing.ContextLogf(ctx, "Round %d. Requesting roam from %s to %s", round+1, ct.mac[round%2], ct.mac[(round+1)%2])

	testing.ContextLogf(ctx, "Sending BSS TM Request from AP %s to DUT %s", ct.mac[round%2], ct.clientMAC)
	req := hostapd.BSSTMReqParams{Neighbors: []string{ct.mac[(round+1)%2].String()}}
	if err := ct.ap[round%2].SendBSSTMRequest(ctx, ct.clientMAC, req); err != nil {
		return errors.Wrap(err, "failed to send BSS TM Request")
	}

	monitorResult, err := waitForProps()
	if err != nil {
		return errors.Wrap(err, "failed to wait for the properties")
	}

	// Check that we don't disconnect along the way here, in case we're ping-ponging around APs.
	for _, ph := range monitorResult {
		if ph.Name == shillconst.ServicePropertyIsConnected {
			if !ph.Value.(bool) {
				return errors.Wrap(err, "failed to stay connected during the roaming process")
			}
		}
	}

	dutState, err := ct.tf.WifiClient().QueryService(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to query service")
	}
	if dutState.Wifi.Bssid != ct.mac[(round+1)%2].String() {
		return errors.Wrapf(err, "unexpected BSSID: got %s, want %s", dutState.Wifi.Bssid, ct.mac[(round+1)%2])
	}
	return nil
}

// Router returns current router object.
func (ct *ContTest) Router() router.Standard {
	return ct.r
}
