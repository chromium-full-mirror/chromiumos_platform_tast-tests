// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"net"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/qos"
	"go.chromium.org/tast-tests/cros/local/network"
	"go.chromium.org/tast-tests/cros/local/network/capture"
	"go.chromium.org/tast-tests/cros/local/network/hwsim"
	patchpanel "go.chromium.org/tast-tests/cros/local/network/patchpanel_client"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/l4server"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// qosAppMonitorTestCase struct encapsulates parameters for each QosAppMonitor
// test.
type qosAppMonitorTestCase struct {
	// APK of test app used in this test.
	apk qos.APK
	// DSCP expected in packet when the test app sets a socket and sends
	// packets in a default setting.
	expectDSCP uint8
	// Network family for this test. One of {l4server.TCP4, l4server.TCP6,
	// l4server.UDP4, l4server.UDP6}.
	family l4server.Family
}

const (
	// Test messages used to set packet payload.
	testMessage1         = "QosAppMonitorTest1"
	testMessage2         = "QosAppMonitorTest2"
	recordingTestMessage = "QosAppMonitorRecordingTest"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         QosAppMonitor,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks if ARC QoS App monitor works correctly",
		Contacts:     []string{"cros-networking@google.com", "chuweih@google.com"},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "wifi", "arc", "no_android_p"},
		Timeout:      10 * time.Minute,
		Fixture:      "shillSimulatedWiFiWithArcBooted",
		Params: []testing.Param{
			{
				Name: "game_app_tcp4",
				Val: qosAppMonitorTestCase{
					apk:        qos.GameAPK,
					expectDSCP: qos.RealtimeInteractiveDSCP,
					family:     l4server.TCP4,
				},
			}, {
				Name: "game_app_tcp6",
				Val: qosAppMonitorTestCase{
					apk:        qos.GameAPK,
					expectDSCP: qos.RealtimeInteractiveDSCP,
					family:     l4server.TCP6,
				},
			}, {
				Name: "game_app_udp4",
				Val: qosAppMonitorTestCase{
					apk:        qos.GameAPK,
					expectDSCP: qos.RealtimeInteractiveDSCP,
					family:     l4server.UDP4,
				},
			}, {
				Name: "game_app_udp6",
				Val: qosAppMonitorTestCase{
					apk:        qos.GameAPK,
					expectDSCP: qos.RealtimeInteractiveDSCP,
					family:     l4server.UDP6,
				},
			}, {
				Name: "news_app_tcp4",
				Val: qosAppMonitorTestCase{
					apk:        qos.NewsAPK,
					expectDSCP: qos.DefaultDSCP,
					family:     l4server.TCP4,
				},
			}, {
				Name: "news_app_tcp6",
				Val: qosAppMonitorTestCase{
					apk:        qos.NewsAPK,
					expectDSCP: qos.DefaultDSCP,
					family:     l4server.TCP6,
				},
			}, {
				Name: "news_app_udp4",
				Val: qosAppMonitorTestCase{
					apk:        qos.NewsAPK,
					expectDSCP: qos.DefaultDSCP,
					family:     l4server.UDP4,
				},
			}, {
				Name: "news_app_udp6",
				Val: qosAppMonitorTestCase{
					apk:        qos.NewsAPK,
					expectDSCP: qos.DefaultDSCP,
					family:     l4server.UDP6,
				},
			}, {
				Name: "productivity_app_tcp4",
				Val: qosAppMonitorTestCase{
					apk:        qos.ProductivityAPK,
					expectDSCP: qos.DefaultDSCP,
					family:     l4server.TCP4,
				},
			}, {
				Name: "productivity_app_tcp6",
				Val: qosAppMonitorTestCase{
					apk:        qos.ProductivityAPK,
					expectDSCP: qos.DefaultDSCP,
					family:     l4server.TCP6,
				},
			}, {
				Name: "productivity_app_udp4",
				Val: qosAppMonitorTestCase{
					apk:        qos.ProductivityAPK,
					expectDSCP: qos.DefaultDSCP,
					family:     l4server.UDP4,
				},
			}, {
				Name: "productivity_app_udp6",
				Val: qosAppMonitorTestCase{
					apk:        qos.ProductivityAPK,
					expectDSCP: qos.DefaultDSCP,
					family:     l4server.UDP6,
				},
			},
		},
	})
}

func QosAppMonitor(ctx context.Context, s *testing.State) {
	// Set up a short context for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Enable QoS feature for testing.
	pc, err := patchpanel.New(ctx)
	if err != nil {
		s.Fatal("Failed to create patchpanel client: ", err)
	}
	restoreQosFeature, err := pc.SetQosEnableWithRestore(ctx, true)
	if err != nil {
		s.Fatal("Failed to enable QoS in patchpanel: ", err)
	}
	defer restoreQosFeature(cleanupCtx)

	// Disable portal detection and set service order for testing.
	manager, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed creating shill manager proxy: ", err)
	}
	if err := manager.DisablePortalDetection(ctx); err != nil {
		s.Fatal("Failed to disable portal detection: ", err)
	}
	defer func() {
		if err := manager.EnablePortalDetection(cleanupCtx); err != nil {
			s.Error("Failed to re-enable portal detection: ", err)
		}
	}()

	restoreServiceOrder, err := manager.SetServiceOrderWithRestore(ctx, []string{"vpn", "wifi", "ethernet", "cellular"})
	if err != nil {
		s.Fatal("Failed to set WiFi service order: ", err)
	}
	defer restoreServiceOrder(cleanupCtx)

	a := s.FixtValue().(*hwsim.ShillSimulatedWiFi).ARC
	// Disable ethernet in ARC for testing.
	if err := a.Command(ctx, "dumpsys", "wifi", "transports", "-eth").Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to disable ethernet in ARC: ", err)
	}
	defer func() {
		if err := a.Command(cleanupCtx, "dumpsys", "wifi", "transports", "eth").Run(testexec.DumpLogOnError); err != nil {
			s.Error(err, "failed to re-enable ethernet in ARC")
		}
	}()

	// Asking powerd to keep the display on to keep test app responsive to requests.
	if err := testexec.CommandContext(ctx, "set_power_policy", "--screen_wake_lock=1").Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to change power policy: ", err)
	}
	defer func() {
		if err := testexec.CommandContext(cleanupCtx, "set_power_policy", "reset").Run(testexec.DumpLogOnError); err != nil {
			s.Error("Failed to reset power policy: ", err)
		}
	}()

	// Set up a WiFi router environment.
	pool := subnet.NewPool()
	simWiFi := s.FixtValue().(*hwsim.ShillSimulatedWiFi)
	wifi, err := virtualnet.CreateWifiRouterEnv(ctx, simWiFi.AP[0], manager, pool, virtualnet.EnvOptions{EnableDHCP: true, RAServer: true})
	if err != nil {
		s.Fatal("Failed to create WiFi router environment: ", err)
	}
	defer func() {
		if err := wifi.Cleanup(cleanupCtx); err != nil {
			s.Error("Failed to clean up virtual WiFi router: ", err)
		}
	}()

	if err := wifi.Service.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to WiFi: ", err)
	}
	if err := wifi.Service.WaitForConnectedOrError(ctx); err != nil {
		s.Fatal("Failed to wait for to WiFi connected status: ", err)
	}

	routerAddrs, err := wifi.Router.WaitForVethInAddrs(ctx, true /*ipv4*/, true /*ipv6*/)
	if err != nil {
		s.Fatal("Failed to get inner addresses from router environment: ", err)
	}

	var pingAddrs []string
	pingAddrs = append(pingAddrs, routerAddrs.IPv4Addr.String())
	for _, ip := range routerAddrs.IPv6Addrs {
		pingAddrs = append(pingAddrs, ip.String())
	}

	// Ping and make sure connection has been established.
	for _, target := range pingAddrs {
		if err := ping.ExpectPingSuccessWithTimeout(ctx, target, "chronos", 10*time.Second); err != nil {
			s.Errorf("network verification failed: %v is not reachable as user chronos on host: %v", target, err)
		}
	}

	// Set up L4 server based on IP family.
	var addr net.IP
	networkFam := s.Param().(qosAppMonitorTestCase).family
	if networkFam == l4server.TCP6 || networkFam == l4server.UDP6 {
		addr = routerAddrs.IPv6Addrs[0]
	} else {
		addr = routerAddrs.IPv4Addr
	}
	port := network.UnusedOrRandomPort(ctx, networkFam)
	server := l4server.New(networkFam, port, l4server.WithAddr(addr.String()), l4server.WithMsgHandler(l4server.Reflector()))
	if err := wifi.Router.StartServer(ctx, networkFam.String(), server); err != nil {
		s.Fatalf("Failed to start %s server: %v", networkFam, err)
	}

	// Install test app.
	app, err := qos.NewTestApp(s.Param().(qosAppMonitorTestCase).apk, a)
	if err != nil {
		s.Fatal("Failed to initialize a new test app: ", err)
	}
	defer func() {
		if err := app.TearDown(cleanupCtx); err != nil {
			s.Error("Failed to tear down test app: ", err)
		}
	}()

	if err := app.InstallAndStart(ctx); err != nil {
		s.Fatal("Failed to install and start test app: ", err)
	}

	// Start packet capture.
	capturer := capture.NewCapturer(s.FixtValue().(*hwsim.ShillSimulatedWiFi).Client[0])
	if err := capturer.Start(ctx); err != nil {
		s.Fatal("Failed to start capture: ", err)
	}
	defer capturer.Stop()

	// When expected DSCP is default DSCP, we need to check only packets with
	// default DSCP are received within a specific period, otherwise we need to
	// check any packet with expected DSCP is received within a specific
	// period.
	targetDSCP := s.Param().(qosAppMonitorTestCase).expectDSCP
	var checkFunc func(ctx context.Context, app *qos.TestApp, msg, addr string, mark uint8, packets chan *capture.Packet) error
	if targetDSCP == qos.DefaultDSCP {
		checkFunc = checkForConsistentPacketMarks
	} else {
		checkFunc = checkForTargetPacketMarks
	}

	if err := app.SetupSocket(ctx, networkFam, addr.String(), port); err != nil {
		s.Error(err, "failed to set socket")
	}

	if err := checkFunc(ctx, app, testMessage1, addr.String(), targetDSCP, capturer.Packets()); err != nil {
		s.Error("Failed to check packets marks: ", err)
	}

	// For productivity apps, we need to check packets are marked with web
	// conference DSCP only when audio/video is being used and DSCP of packets
	// is changed back to default DSCP after audio/video is stopped.
	if s.Param().(qosAppMonitorTestCase).apk == qos.ProductivityAPK {
		if err := app.StartRecording(ctx); err != nil {
			s.Error(err, "failed to start recording")
		}
		if err := checkForTargetPacketMarks(ctx, app, recordingTestMessage, addr.String(), qos.MediaConferencingDSCP, capturer.Packets()); err != nil {
			s.Error("Failed to wait for packets with target DSCP: ", err)
		}
		if err := app.StopRecording(ctx); err != nil {
			s.Error(err, "failed to stop recording")
		}
		if err := checkForTargetPacketMarks(ctx, app, recordingTestMessage, addr.String(), qos.DefaultDSCP, capturer.Packets()); err != nil {
			s.Error("Failed to wait for packets with target DSCP: ", err)
		}
	}

	// For IPv4 network family, we need to test for both situations that:
	// (1) Setting up IPv4 socket with its original IPv4 address
	// (2) Setting up IPv6 socket with its IPv4-mapped IPv6 address.
	// For IPv4 network family, we only need to test for that they can work
	// properly with socket of the their own type.
	if networkFam == l4server.TCP4 {
		if err := app.SetupSocket(ctx, l4server.TCP6, addr.String(), port); err != nil {
			s.Error(err, "failed to set socket")
		}
		if err := checkFunc(ctx, app, testMessage2, addr.String(), targetDSCP, capturer.Packets()); err != nil {
			s.Error("Failed to check packets marks: ", err)
		}
	}
	if networkFam == l4server.UDP4 {
		if err := app.SetupSocket(ctx, l4server.UDP6, addr.String(), port); err != nil {
			s.Error(err, "failed to set socket")
		}
		if err := checkFunc(ctx, app, testMessage2, addr.String(), targetDSCP, capturer.Packets()); err != nil {
			s.Error("Failed to check packets marks: ", err)
		}
	}
}

// checkForConsistentPacketMarks lets app send packets with msg, and verifies
// that all the sent packets have target DSCP mark within a specific perioid.
func checkForConsistentPacketMarks(ctx context.Context, app *qos.TestApp, msg, addr string, mark uint8, packets chan *capture.Packet) error {
	testing.ContextLogf(ctx, "Check that only packets with DSCP %v is received", mark)
	// Send a message to make sure that the connection is alive.
	if err := sendAndCheckPacketDSCP(ctx, app, packets, msg, addr, mark); err != nil {
		return errors.Wrap(err, "failed to wait for packet")
	}
	for i := 1; i <= 5; i++ {
		// GoBigSleepLint: there can be a delay between 1) the connection is
		// established in in ARC and 2) QoS service starts to apply DSCP on the
		// packet of this connection, so add a sleep here to make sure that
		// connection exists longer than the maximum delay (~20 seconds).
		testing.Sleep(ctx, 8*time.Second)
		if err := sendAndCheckPacketDSCP(ctx, app, packets, msg, addr, mark); err != nil {
			return errors.Wrap(err, "failed to wait for packet")
		}
	}
	testing.ContextLog(ctx, "No packet with unexpected DSCP was found, check succeed")
	return nil
}

// checkForTargetPacketMarks checks that any packet with target DSCP is
// received after sending out a message within a certain period, and following
// several packets also have target DSCP to confirm that the connection is
// correctly established.
func checkForTargetPacketMarks(ctx context.Context, app *qos.TestApp, msg, addr string, mark uint8, packets chan *capture.Packet) error {
	testing.ContextLogf(ctx, "Waiting for packets with DSCP %v is received", mark)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := sendAndCheckPacketDSCP(ctx, app, packets, msg, addr, mark); err != nil {
			return errors.Wrap(err, "no packets with target DSCP is found")
		}
		testing.ContextLogf(ctx, "Packet with DSCP %v received", mark)
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 5 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to wait for packet with target DSCP")
	}
	// After getting the packet with target DSCP, we want to check if following
	// packets also have the same DSCP, in order to check that the connection
	// is correcrly marked and is not lost.
	for i := 1; i <= 5; i++ {
		if err := sendAndCheckPacketDSCP(ctx, app, packets, msg, addr, mark); err != nil {
			return errors.Wrap(err, "failed to check packets with the same DSCP")
		}
	}
	testing.ContextLog(ctx, "Verified that the connection is correctly marked")
	return nil
}

// sendAndCheckPacketDSCP sends out a message and checks if the outgoing packet
// whose destination address is |addr| has target DSCP within the duration of
// given context.
func sendAndCheckPacketDSCP(ctx context.Context, app *qos.TestApp, packets chan *capture.Packet, msg, addr string, target uint8) error {
	// Create a shortened context to bound the packet checks. As
	// the connection is already done, it shouldn't be necessary
	// to wait a long time.
	deadline := time.Now().Add(5 * time.Second)
	shortCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	if err := app.SendMessage(shortCtx, msg); err != nil {
		return errors.Wrap(err, "send message failed")
	}
	for {
		select {
		case p := <-packets:
			var dstAddr string
			if ip := p.IPv4; ip != nil {
				dstAddr = ip.DstIP.String()
			}
			if ip := p.IPv6; ip != nil {
				dstAddr = ip.DstIP.String()
			}
			// If dstAddr is null, which means it is not an IP packet, or
			// when dstAddr does not match, which means it is not the
			// outgoing packet we care about, skip.
			if dstAddr != addr {
				continue
			}

			if p.Application == nil || p.Application.Payload() == nil {
				continue
			}
			// When sending out a message there will an incoming and outgoing
			// packet, and we only want to check outgoing packet.
			if strings.Contains(string(p.Application.Payload()), msg) {
				if qos.HasDSCP(p, target) {
					return nil
				}
				return errors.New("DSCP of received package does not match")
			}
		case <-shortCtx.Done():
			return errors.New("timeout waiting for packets")
		}
	}
}
