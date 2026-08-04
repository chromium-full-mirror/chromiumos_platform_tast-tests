// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package debugd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"golang.org/x/exp/slices"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/network/ping"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/remote/wificell/pcap"
	"go.chromium.org/tast-tests/cros/services/cros/debugd"
	policypb "go.chromium.org/tast-tests/cros/services/cros/policy"

	//lint:ignore ST1019 multiple imports with different identifiers help code readability
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	uipb "go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           PacketCaptureWifi,
		LifeCycleStage: testing.LifeCycleInDevelopment,
		Desc:           "Verifies that network packet capture works, and various options function properly",
		Contacts: []string{
			"edgar.change@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent: "b:1318544", // ChromeOS > Software > System Services > Connectivity > General
		// TODO(b/542029943): Re-enable once the test is stable.
		// Attr:         []string{"group:wificell", "wificell_e2e_unstable"},
		TestBedDeps: []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.BluetoothStateNormal, tbdep.PeripheralWifiStateWorking},
		Fixture:     wificell.FixtureID(wificell.TFFeaturesEnroll),
		ServiceDeps: []string{
			wificell.ShillServiceName,
			"tast.cros.policy.PolicyService",
			"tast.cros.debugd.DebugdService",
			"tast.cros.ui.NotificationService",
		},
		SoftwareDeps: []string{"chrome"},
		// Qualcomm WiFi chipsets on Chromebook don't support monitor mode. (b/340125661)
		HardwareDeps: hwdep.D(hwdep.WifiNotQualcomm()),
		// This test contains 3 sub-tests, each of which is allocated 2 minutes of RPC streaming time.
		// Five minutes (the default timeout for remote tests) of runtime is reserved for the remaining test steps.
		// Five minutes is reserved for the enrollment.
		Timeout: 3*rpcStreamingTimeout + 5*time.Minute + enrollmentTimeout,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceDebugPacketCaptureAllowed{}, pci.VerifiedFunctionalityOS),
		},
	})
}

const (
	enrollmentTimeout   = 5 * time.Minute
	rpcStreamingTimeout = 2 * time.Minute
)

// PacketCaptureWifi verifies that network packet capture works, and various options function properly.
func PacketCaptureWifi(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)
	rpc := tf.DUTRPC(wificell.DefaultDUT)

	blob := policy.NewBlob()
	if err := blob.AddPolicy(&policy.DeviceDebugPacketCaptureAllowed{Val: true}); err != nil {
		s.Fatal("Failed to add policy: ", err)
	}
	policyJSON, err := json.Marshal(blob)
	if err != nil {
		s.Fatal("Failed to serialize policies: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	policyService := policypb.NewPolicyServiceClient(rpc.Conn)
	if _, err := policyService.StartChrome(ctx, &policypb.StartChromeRequest{
		PolicyJson:     policyJSON,
		KeepEnrollment: true,
	}); err != nil {
		s.Fatal("Failed to start Chrome with policy: ", err)
	}
	defer policyService.StopChromeAndFakeDMS(cleanupCtx, &emptypb.Empty{})

	// The fundamental frequency of the channel 36 is 5180MHz.
	const channelFor5GHz int = 36
	const fundamentalFrequency int32 = 5180

	apOpts := []hostapd.Option{
		hostapd.Mode(hostapd.Mode80211acPure),
		hostapd.Channel(channelFor5GHz),
		hostapd.HTCaps(hostapd.HTCapHT40Plus),
		hostapd.VHTCenterChannel(42),
		hostapd.VHTChWidth(hostapd.VHTChWidth80),
	}

	ap, err := tf.ConfigureAP(ctx, apOpts, nil)
	if err != nil {
		s.Fatal("Failed to configure the AP: ", err)
	}
	defer tf.DeconfigAP(ctx, ap)
	ctx, cancel = tf.ReserveForDeconfigAP(ctx, ap)
	defer cancel()

	if _, err := tf.ConnectWifiAPFromDUT(ctx, wificell.DefaultDUT, ap); err != nil {
		s.Fatal("Failed to connect to AP: ", err)
	}
	defer tf.CleanDisconnectDUTFromWifi(cleanupCtx, wificell.DefaultDUT)

	if err := tf.DUTWifiClient(wificell.DefaultDUT).WaitForConnected(ctx, ap.Config().SSID, true /* expectedValue */); err != nil {
		s.Fatal("Failed to wait for Wi-Fi to be connected: ", err)
	}

	notificationService := ui.NewNotificationServiceClient(rpc.Conn)
	debugdService := debugd.NewDebugdServiceClient(rpc.Conn)

	const monitoredInterface = "wlan0"
	for _, params := range []struct {
		name string
		req  *debugd.PacketCaptureRequest
	}{
		{
			name: "frequency",
			req: &debugd.PacketCaptureRequest{
				MonitoredInterface: proto.String(monitoredInterface),
				Frequency:          fundamentalFrequency,
			},
		}, {
			name: "ht_location",
			req: &debugd.PacketCaptureRequest{
				MonitoredInterface: proto.String(monitoredInterface),
				Frequency:          fundamentalFrequency,
				HtLocation:         debugd.PacketCaptureRequest_ABOVE,
			},
		}, {
			name: "vht_width",
			req: &debugd.PacketCaptureRequest{
				MonitoredInterface: proto.String(monitoredInterface),
				Frequency:          fundamentalFrequency,
				VhtWidth:           debugd.PacketCaptureRequest_VHTCh_WIDTH_80,
			},
		},
	} {
		s.Run(ctx, params.name, func(ctx context.Context, s *testing.State) {
			streamingCtx, cancelStreamingCtx := context.WithTimeout(ctx, rpcStreamingTimeout)
			defer cancelStreamingCtx()

			stream, err := debugdService.PacketCapture(streamingCtx, params.req)
			if err != nil {
				s.Fatal("Failed to initialize PacketCapture client: ", err)
			}

			packetCaptureFile := filepath.Join(s.OutDir(), fmt.Sprintf("output_%s.pcap", params.name))

			wg, _ := errgroup.WithContext(streamingCtx)
			wg.Go(func() error { return saveStreamingDataToFile(stream, packetCaptureFile) })

			// notificationID is notification ID of the packet capture notification. It is hard-coded in Chrome as
			// DebugdNotificationHandler::kPacketCaptureNotificationId.
			const notificationID = "debugd-packetcapture"

			// Validate that the Packet Capture is actually started.
			if _, err := notificationService.WaitForNotification(ctx, &ui.WaitForNotificationRequest{
				Predicates: []*uipb.WaitPredicate{
					{Value: &uipb.WaitPredicate_IdContains{IdContains: notificationID}},
				},
			}); err != nil {
				s.Fatal("Failed to wait for packet capture notification: ", err)
			}

			// Sending ICMP packets to DUT via the command line tool "ping".
			if err := tf.PingFromRouterID(ctx, int(wificell.DefaultRouter), ping.Count(10)); err != nil {
				s.Fatal(`Failed to send the packets via "ping" command line tool: `, err)
			}

			if err := validatePacketCaptureData(ctx, packetCaptureFile, ap.ServerIP(), rpcStreamingTimeout); err != nil {
				// Check if the streaming side went wrong when the data couldn't be validated successfully.
				cancelStreamingCtx()
				if wg.Wait() != nil {
					s.Error("Failed to received packets data from PacketCapture streaming server: ", wg.Wait())
				}
				s.Fatal("Failed to find packet sent by router: ", err)
			}
		})
	}
}

func saveStreamingDataToFile(stream debugd.DebugdService_PacketCaptureClient, filePath string) error {
	fd, err := os.Create(filePath)
	if err != nil {
		return errors.Wrap(err, "failed to open file for streaming data")
	}
	defer fd.Close()

	for {
		chunk, err := stream.Recv()
		if err != nil {
			if errors.Is(stream.Context().Err(), context.DeadlineExceeded) || errors.Is(stream.Context().Err(), context.Canceled) || err == io.EOF {
				return nil
			}
			return errors.Wrap(err, "failed to receive chunk")
		}

		if _, err := fd.Write(chunk.GetData()); err != nil {
			return errors.Wrap(err, "failed to write chunk to file")
		}
	}
}

func validatePacketCaptureData(ctx context.Context, logFile string, apIP net.IP, timeout time.Duration) error {
	icmpV4filter := pcap.TypeFilter(layers.LayerTypeICMPv4, func(layer gopacket.Layer) bool { return true })
	return testing.Poll(ctx, func(ctx context.Context) error {
		packets, err := pcap.ReadPackets(logFile, icmpV4filter)
		if err != nil {
			return errors.Wrap(err, "failed to read packets")
		}

		if index := slices.IndexFunc(packets, func(packet gopacket.Packet) bool {
			sourceIP := packet.NetworkLayer().NetworkFlow().Src().String()
			return net.ParseIP(sourceIP).Equal(apIP)
		}); index == -1 {
			// TODO(b/366337725): Remove the log after the issue is resolved.
			for _, packet := range packets {
				testing.ContextLogf(ctx, "ICMP packet: source: %q , destination: %q", packet.NetworkLayer().NetworkFlow().Src().String(), packet.NetworkLayer().NetworkFlow().Dst().String())
			}
			return errors.Wrap(err, "failed to find packet sent by router")
		}

		return nil
	}, &testing.PollOptions{Timeout: timeout, Interval: time.Second})
}
