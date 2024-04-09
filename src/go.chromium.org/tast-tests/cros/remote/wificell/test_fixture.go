// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wificell

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/network/firewall"
	"go.chromium.org/tast-tests/cros/common/network/ping"
	"go.chromium.org/tast-tests/cros/common/network/protoutil"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/pkcs11/netcertstore"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/utils"
	"go.chromium.org/tast-tests/cros/common/wifi/arping"
	"go.chromium.org/tast-tests/cros/common/wifi/p2p"
	"go.chromium.org/tast-tests/cros/common/wifi/security"
	"go.chromium.org/tast-tests/cros/common/wifi/security/base"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast-tests/cros/common/wifi/wpacli"
	"go.chromium.org/tast-tests/cros/remote/hwsec"
	remotefirewall "go.chromium.org/tast-tests/cros/remote/network/firewall"
	"go.chromium.org/tast-tests/cros/remote/network/iperf"
	remoteping "go.chromium.org/tast-tests/cros/remote/network/ping"
	remotearping "go.chromium.org/tast-tests/cros/remote/wifi/arping"
	"go.chromium.org/tast-tests/cros/remote/wifi/iw"
	remotewpacli "go.chromium.org/tast-tests/cros/remote/wifi/wpacli"
	"go.chromium.org/tast-tests/cros/remote/wificell/attenuator"
	"go.chromium.org/tast-tests/cros/remote/wificell/dutcfg"
	"go.chromium.org/tast-tests/cros/remote/wificell/framesender"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	ap "go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/remote/wificell/pcap"
	"go.chromium.org/tast-tests/cros/remote/wificell/router"
	"go.chromium.org/tast-tests/cros/remote/wificell/router/ax"
	"go.chromium.org/tast-tests/cros/remote/wificell/router/common"
	"go.chromium.org/tast-tests/cros/remote/wificell/router/common/support"
	"go.chromium.org/tast-tests/cros/remote/wificell/router/legacy"
	"go.chromium.org/tast-tests/cros/remote/wificell/router/openwrt"
	"go.chromium.org/tast-tests/cros/remote/wificell/router/ubuntu"
	"go.chromium.org/tast-tests/cros/remote/wificell/tethering"
	"go.chromium.org/tast-tests/cros/services/cros/bluetooth"
	"go.chromium.org/tast-tests/cros/services/cros/cellular"
	"go.chromium.org/tast-tests/cros/services/cros/power"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/wlan"
	"go.chromium.org/tast/core/timing"
)

// Tast service name constants.
const (
	// ShillServiceName is the service needed by TestFixture and any test that
	// uses the shill service directly.
	ShillServiceName = "tast.cros.wifi.ShillService"

	// BluetoothServiceName is the name of the bluetooth service needed by
	// TestFixture.
	BluetoothServiceName = "tast.cros.bluetooth.BluetoothService"

	// PowerSetupServiceName is the name of the power service that setup the
	// device for power measurement.
	PowerSetupServiceName = "tast.cros.power.DeviceSetupService"

	// PowerRecorderServiceName is the name of the power service that records
	// power measurement.
	PowerRecorderServiceName = "tast.cros.power.RecorderService"

	// CellularServiceName is the name of the cellular service needed by TestFixture.
	CellularServiceName = "tast.cros.cellular.RemoteCellularService"

	// AutomationServiceName is the name of the automation service.
	AutomationServiceName = "tast.cros.ui.AutomationService"

	// BrowserChromeServiceName is the name of the browser chrome service.
	BrowserChromeServiceName = "tast.cros.browser.ChromeService"

	// OsSettingsServiceName is the name of the os settings service.
	OsSettingsServiceName = "tast.cros.chrome.uiauto.ossettings.OsSettingsService"

	// QuickSettingsServiceName is the name of the quick settings service.
	QuickSettingsServiceName = "tast.cros.chrome.uiauto.quicksettings.QuickSettingsService"

	// ChromeUIServiceName is the name of the chrome ui service.
	ChromeUIServiceName = "tast.cros.ui.ChromeUIService"

	// WifiUIServiceName is the name of the Wi-Fi ui service.
	WifiUIServiceName = "tast.cros.wifi.WifiService"
)

// P2PDevice is used as p2p device type.
type P2PDevice int32

// P2P devices (options for Group Owner (GO) and client).
const (
	P2PDeviceDUT P2PDevice = iota
	P2PDeviceCompanionDUT
	// TODO(b/231261132): add Android phones as GO/Client options.
)

// DutIdx is the type used for DUT index in TestFixture.duts.
//
// Use one of the known constants below rather than initializing this directly.
type DutIdx int

// Known DutIdx values.
const (
	// DefaultDUT is the DutIdx of the default dut.
	DefaultDUT DutIdx = 0

	// PeerDUT1 is the DutIdx of the first peer/companion dut.
	PeerDUT1 DutIdx = 1
)

// RouterIdx is the type used for router index in TestFixture.routers.
type RouterIdx int

// Known RouterIdx values.
const (
	// DefaultRouter is the RouterIdx of the default router.
	DefaultRouter RouterIdx = 0
)

// Other miscellaneous constants.
const (
	// The allowed packets loss percentage for the ping command.
	pingLossThreshold float64 = 20

	// The allowed packets loss percentage for the arping command.
	arpingLossThreshold float64 = 30

	// The amount of time to wait before reconnecting after a router reboot.
	routerPostRebootWaitTime = 5 * time.Second

	// DUTRebootTimeout specifies the time duration of a DUT is ready for testing after the reboot triggers.
	// It could take up to 5 minutes to reboot a DUT, especially for low-end devices.
	DUTRebootTimeout = 5 * time.Minute

	// ChromeFeatureHotspot is the name of the hotspot feature flag.
	ChromeFeatureHotspot = "Hotspot"

	// ChromeFeatureExperimentalHotspot is the name of the hotspot experimental functionality flag.
	ChromeFeatureExperimentalHotspot = "TetheringExperimentalFunctionality"
)

// bridgeData holds all variables related to bridges on a router for multiple
// interfaces on a single hostapd.
type bridgeData struct {
	r  router.StandardWithBridge
	br string
}

// bridgeVeth holds all variables related to bridges and veth on a router for
// TFFeaturesBridgeAndVeth fixture
type bridgeVethData struct {
	r    router.StandardWithBridgeAndVeth
	br   []string
	veth []string
}

// TODO(b/234845693): make that an independent structure.
type routerData struct {
	target string
	host   *ssh.Conn
	object router.Base
	br     *bridgeData
	brveth *bridgeVethData
}

// TestFixture sets up the context for a basic WiFi test.
type TestFixture struct {
	options *TFOptions

	// Wificell devices.
	// Duts and the pcap must always be initialized, but routers may be empty if
	// TFOptions.RequirePrimaryRouter is false in options.
	duts         []*dutData
	routers      []*routerData
	pcap         *routerData
	pcapIsRouter bool
	attenuator   *attenuator.Attenuator

	// The following parameters (with prefix p2p*) are used with P2P tests.
	p2pGO     P2PWiFiDevice
	p2pClient P2PWiFiDevice

	apID              int
	seederIfaces      []*APIface
	seederSSID        string
	capturers         map[*APIface]map[int]*pcap.Capturer
	tetheringCapturer *pcap.Capturer
	useWpaCliAPI      bool

	// aps is a set of APs useful for deconfiguring all APs, which some tests require.
	aps map[*APIface]struct{}

	// Power test parameters.
	idlePowerValues     *perf.Values
	powerSetupClient    power.DeviceSetupServiceClient
	powerRecorderClient power.RecorderServiceClient
	powerCleanup        func(context.Context) error
}

// NewTestFixture creates a TestFixture.
// The TestFixture contains a gRPC connection to the DUT and a SSH connection to the router.
// The method takes two context: ctx and daemonCtx, the first one is the context for the operation and
// daemonCtx is for the spawned daemons.
// After the caller gets the TestFixture instance, it should reserve time for Close() the TestFixture:
//
//	tf, err := NewTestFixture(ctx, ...)
//	if err != nil {...}
//	defer tf.Close(ctx)
//	ctx, cancel := tf.ReserveForClose(ctx)
//	defer cancel()
//	...
func NewTestFixture(fullCtx, daemonCtx context.Context, options *TFOptions) (ret *TestFixture, retErr error) {
	fullCtx, st := timing.Start(fullCtx, "NewTestFixture")
	defer st.End()

	testing.ContextLog(fullCtx, "[WIFICELL_FIXTURE] NewTestFixture :: START")
	defer testing.ContextLog(fullCtx, "[WIFICELL_FIXTURE] NewTestFixture :: END")

	// Initialize fixture with provided options.
	testing.ContextLogf(fullCtx, "Fixture options: %s", options)
	if err := options.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid fixture options")
	}
	tf := &TestFixture{
		options:      options,
		duts:         append(make([]*dutData, 0), options.duts...),
		capturers:    make(map[*APIface]map[int]*pcap.Capturer),
		aps:          make(map[*APIface]struct{}),
		useWpaCliAPI: true,
	}

	// Automatically close if we have any initialization errors (beyond options).
	defer func() {
		if retErr != nil {
			if err := tf.Close(fullCtx); err != nil {
				testing.ContextLog(fullCtx, "Failed to Close after NewTestFixture failure: ", err)
			}
		}
	}()

	ctx, cancel := tf.ReserveForClose(fullCtx)
	defer cancel()

	if err := tf.initializeDuts(ctx, daemonCtx); err != nil {
		return nil, err
	}
	if err := tf.initializePrimaryRouters(ctx, daemonCtx); err != nil {
		return nil, err
	}
	if err := tf.initializePcapRouter(ctx, daemonCtx); err != nil {
		return nil, err
	}
	if err := tf.initializeAttenuator(ctx); err != nil {
		return nil, err
	}

	// Seed the random as we have some randomization. e.g. default SSID.
	rand.Seed(time.Now().UnixNano())

	// Reinitialize state of routers (including the pcap).
	if err := tf.ReinitRouters(ctx, true); err != nil {
		return nil, err
	}

	return tf, nil
}

func (tf *TestFixture) initializeDuts(ctx, daemonCtx context.Context) error {
	if len(tf.duts) == 0 {
		// This should not occur, but this is an extra step to catch a regression.
		return errors.New("tf.duts is nil")
	}
	for idx, d := range tf.duts {
		var err error
		d.rpc, err = rpc.Dial(daemonCtx, d.dut, d.rpcHint)
		if err != nil {
			return errors.Wrap(err, "failed to connect to the RPC service on the DUT")
		}
		d.wifiClient = &WifiClient{
			ShillServiceClient: wifi.NewShillServiceClient(d.rpc.Conn),
		}
		d.bluetoothClient = bluetooth.NewBluetoothServiceClient(d.rpc.Conn)

		// Set DUT to use bluez bluetooth stack and enable bluetooth.
		if _, err := d.bluetoothClient.SetBluetoothStack(ctx, &bluetooth.SetBluetoothStackRequest{
			StackType: bluetooth.BluetoothStackType_BLUETOOTH_STACK_TYPE_BLUEZ,
		}); err != nil {
			return errors.Wrap(err, "failed to set DUT bluetooth stack to bluez")
		}
		if _, err := d.bluetoothClient.Enable(ctx, &empty.Empty{}); err != nil {
			return errors.Wrap(err, "failed to enable bluetooth on DUT")
		}

		if _, err := d.wifiClient.InitDUT(ctx, &wifi.InitDUTRequest{WithUi: tf.options.EnableDutUI}); err != nil {
			return errors.Wrap(err, "failed to InitDUT")
		}

		info, err := d.wifiClient.GetDeviceInfo(ctx, &empty.Empty{})
		if err == nil {
		}
		testing.ContextLogf(ctx, "DUT#%d added: %+v", idx, info.Name)
		d.chipset = wlan.DeviceID(info.Id)

		if tf.options.EnableCellular && DutIdx(idx) == DefaultDUT {
			d.cellularClient = cellular.NewRemoteCellularServiceClient(d.rpc.Conn)
		}

		if tf.options.SetDutWifiLogging {
			d.originalLogLevel, d.originalLogTags, err = tf.getLoggingConfig(ctx, d.wifiClient)
			if err != nil {
				return err
			}
			if err := tf.setLoggingConfig(ctx, d.wifiClient, tf.options.DutWifiLogLevel, tf.options.DutWifiLogTags); err != nil {
				return err
			}
		}
	}
	return nil
}

func (tf *TestFixture) initializePrimaryRouters(ctx, daemonCtx context.Context) error {
	testing.ContextLog(ctx, "Initializing primary routers")
	tf.routers = nil

	// Resolve router targets.
	routerTargets := append(make([]string, 0), tf.options.PrimaryRouterTargets...)
	if len(routerTargets) == 0 {
		// Wificell precondition always provides us with router name, but we need
		// to handle case when the fixture is created from outside the precondition.
		testing.ContextLog(ctx, "No router target specified, attempting resolve default router target hostname based off of the dut hostname")
		defaultRouterTarget, err := tf.resolveCompanionHostname(utils.CompanionSuffixRouter)
		if err != nil {
			if tf.options.RequirePrimaryRouter {
				return errors.Wrap(err, "router is required, no router target specified, and failed to produce a valid default router target hostname based off of the dut hostname")
			}
			testing.ContextLog(ctx, "No router target specified and failed to produce a valid default router target hostname based off of the dut hostname: ", err)
			testing.ContextLog(ctx, "Fixture option RequirePrimaryRouter is false, continuing without a router target")
		} else {
			testing.ContextLogf(ctx, "Using default router target %q", defaultRouterTarget)
			routerTargets = append(routerTargets, defaultRouterTarget)
		}
	}

	// Connect to and initialize primary routers.
	for i, target := range routerTargets {
		rd := &routerData{target: target}
		tf.routers = append(tf.routers, rd)
		testing.ContextLogf(ctx, "Adding router %q as router[%d]", rd.target, i)
		routerHost, err := tf.connectCompanion(ctx, rd.target, true /* allow retry */)
		if err != nil {
			return errors.Wrapf(err, "failed to connect to the router %s", rd.target)
		}
		rd.host = routerHost
		routerObj, err := tf.newRouter(ctx, daemonCtx, rd.host, rd.target)
		if err != nil {
			return errors.Wrap(err, "failed to create a router object")
		}
		testing.ContextLogf(ctx, "Successfully instantiated %s router controller for router[%d]", routerObj.RouterType().String(), i)
		rd.object = routerObj
	}

	testing.ContextLogf(ctx, "Initialized %d primary routers", len(tf.routers))
	return nil
}

// initializePcapRouter initializes tf.pcap. A non-nil error guarantees that
// tf.pcap is fully configured router device capable of packet capture.
func (tf *TestFixture) initializePcapRouter(ctx, daemonCtx context.Context) error {
	tf.pcap = nil
	testing.ContextLog(ctx, "Initializing pcap router")

	// Identify host to use as pcap.
	pcapTarget := tf.options.PcapRouterTarget
	usingDefaultPcapTarget := false
	if tf.options.UseFirstRouterAsPcap {
		if len(tf.routers) == 0 {
			return errors.New("fixture option UseFirstRouterAsPcap is enabled, but there are no routers")
		}
		routerPcap := tf.routers[DefaultRouter]
		testing.ContextLogf(ctx, "Fixture option UseFirstRouterAsPcap is enabled, using router[%d] %q as pcap", DefaultRouter, routerPcap.object.RouterName())
		tf.pcap = routerPcap
		tf.pcapIsRouter = true
	} else if pcapTarget == "" {
		testing.ContextLog(ctx, "No pcap target specified, attempting resolve default pcap target hostname based off of the dut hostname")
		defaultPcapTarget, err := tf.resolveCompanionHostname(utils.CompanionSuffixPcap)
		if err != nil {
			testing.ContextLog(ctx, "No pcap target specified and failed to produce a valid default pcap target hostname based off of the dut hostname: ", err)
			testing.ContextLogf(ctx, "Falling back to using router[%d] as pcap", DefaultRouter)
			if len(tf.routers) == 0 {
				return errors.Errorf("failed to fallback to using router[%d] as pcap: no routers are configured", DefaultRouter)
			}
			tf.pcap = tf.routers[DefaultRouter]
			tf.pcapIsRouter = true
		} else {
			testing.ContextLogf(ctx, "Using default pcap target %q", defaultPcapTarget)
			pcapTarget = defaultPcapTarget
			usingDefaultPcapTarget = true
		}
	}
	if !tf.pcapIsRouter {
		// Check for pcap duplicates on router list.
		for i, rd := range tf.routers {
			// We're checking only hostnames, as these should be autogenerated by precondition.
			// If hostnames are supplied manually, testing lab should guarantee that
			// no two devices names point to the same device.
			if pcapTarget == rd.target {
				testing.ContextLogf(ctx, "Fixture pcap target %q already configured as router[%d], will use router[%d] also as the pcap", pcapTarget, i, i)
				tf.pcap = rd
				tf.pcapIsRouter = true
				break
			}
		}
	}

	// Initialize the pcap router controller if we aren't reusing an existing one.
	if !tf.pcapIsRouter {
		testing.ContextLogf(ctx, "Adding router %q as pcap", pcapTarget)
		var err error
		pcapRouterHost, err := tf.connectCompanion(ctx, pcapTarget, false /* no retry when DNS not found */)
		if err != nil {
			// We want to fall back to using router[DefaultRouter] as pcap iff the default
			// pcap hostname is invalid. Fail here if it's not the case.
			if !usingDefaultPcapTarget || !tf.errIsInvalidHost(err) {
				return errors.Wrap(err, "failed to connect to pcap")
			}
			testing.ContextLogf(ctx, "Failed to connect to default pcap target %q, falling back to using router[%d] as pcap", pcapTarget, DefaultRouter)
			if len(tf.routers) == 0 {
				return errors.Errorf("failed to fallback to using router[%d] as pcap: no routers are configured", DefaultRouter)
			}
			tf.pcap = tf.routers[DefaultRouter]
			tf.pcapIsRouter = true
		} else {
			rd := &routerData{
				host:   pcapRouterHost,
				target: pcapTarget,
			}
			tf.pcap = rd
			routerObj, err := tf.newRouter(ctx, daemonCtx, rd.host, rd.target)
			if err != nil {
				return errors.Wrap(err, "failed to create a router object for pcap")
			}
			rd.object = routerObj
			testing.ContextLogf(ctx, "Successfully instantiated %s router controller for pcap", rd.object.RouterType().String())
		}
	}

	// Validate that the pcap actually supports packet capture.
	if _, ok := tf.pcap.object.(support.Capture); !ok {
		return errors.Errorf("failed to initialize pcap %q: router type %q does not support Capture", tf.pcap.object.RouterName(), tf.pcap.object.RouterType().String())
	}

	testing.ContextLogf(ctx, "Initialized pcap router as %q", tf.pcap.object.RouterName())
	return nil
}

func (tf *TestFixture) initializeAttenuator(ctx context.Context) error {
	if tf.options.AttenuatorTarget == "" {
		testing.ContextLog(ctx, "Skipping opening of attenuator: No attenuatorTarget specified for fixture")
		return nil
	}
	if len(tf.routers) == 0 {
		testing.ContextLog(ctx, "Skipping opening of attenuator: No routers are configured")
		return nil
	}
	testing.ContextLog(ctx, "Opening Attenuator: ", tf.options.AttenuatorTarget)
	var err error
	// routers[DefaultRouter] should always be present, thus we use it as a proxy.
	tf.attenuator, err = attenuator.Open(ctx, tf.options.AttenuatorTarget, tf.routers[DefaultRouter].host)
	if err != nil {
		return errors.Wrap(err, "failed to open attenuator")
	}
	return nil
}

// resolveCompanionHostname builds a companion device hostname based on the
// DefaultDUT's hostname and the provided suffix and attempts to resolve the IP
// address to validate that the built hostname is resolvable.
func (tf *TestFixture) resolveCompanionHostname(companionHostnameSuffix string) (string, error) {
	dutHostname := tf.duts[DefaultDUT].dut.HostName()
	dutHostnameWithoutPort := strings.Split(tf.duts[DefaultDUT].dut.HostName(), ":")[0]
	if dutHostnameWithoutPort == "localhost" || net.ParseIP(dutHostnameWithoutPort) != nil {
		return "", errors.Errorf("cannot resolve companion hostname from localhost or IP dut hostname %q", dutHostname)
	}
	companionHostname, err := utils.CompanionDeviceHostname(dutHostname, companionHostnameSuffix)
	if err != nil {
		return "", errors.Wrap(err, "failed to build companion device hostname")
	}
	if _, err := net.LookupIP(companionHostname); err != nil {
		return "", errors.Wrapf(err, "could not resolve IP for companion device hostname %q", companionHostname)
	}
	return companionHostname, nil
}

// connectCompanion dials SSH connection to companion device with the auth key of DUT.
func (tf *TestFixture) connectCompanion(ctx context.Context, hostname string, retryDNSNotFound bool) (*ssh.Conn, error) {
	var sopt ssh.Options
	if err := ssh.ParseTarget(hostname, &sopt); err != nil {
		return nil, errors.Wrap(err, "failed to parse ssh target")
	}
	// Assumption is, that the key will be shared between DUTs.
	sopt.KeyDir = tf.duts[DefaultDUT].dut.KeyDir()
	sopt.KeyFile = tf.duts[DefaultDUT].dut.KeyFile()
	var conn *ssh.Conn

	if tf.options.HostUserOverrides != nil {
		if username, ok := tf.options.HostUserOverrides[hostname]; ok {
			testing.ContextLogf(ctx, "Using ssh username override %q for host %q", username, hostname)
			sopt.User = username
		}
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var err error
		var dnsErr *net.DNSError

		conn, err = ssh.New(ctx, &sopt)
		if !retryDNSNotFound && errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			// Don't retry DNS not found case.
			return testing.PollBreak(err)
		}
		return err
	}, &testing.PollOptions{
		Timeout: 5 * time.Minute,
	}); err != nil {
		return nil, err
	}
	return conn, nil
}

// newRouter connects to and initializes the router via SSH then returns the router object.
// This method takes two context: ctx and daemonCtx, the first is the context for the NewRouter
// method and daemonCtx is for the spawned background daemons.
// After getting a Server instance, d, the caller should call r.Close() at the end, and use the
// shortened ctx (provided by d.ReserveForClose()) before r.Close() to reserve time for it to run.
func (tf *TestFixture) newRouter(ctx, daemonCtx context.Context, host *ssh.Conn, name string) (router.Base, error) {
	ctx, st := timing.Start(ctx, "newRouter")
	defer st.End()

	name = strings.ReplaceAll(name, ":", "_")
	testing.ContextLogf(ctx, "[WIFICELL_FIXTURE] newRouter :: %q :: START", name)
	defer testing.ContextLogf(ctx, "[WIFICELL_FIXTURE] newRouter :: %q :: END", name)

	rtype, err := tf.resolveRouterTypeFromHost(ctx, host)
	if err != nil {
		return nil, errors.Wrap(err, "failed to resolve router type from host")
	}
	testing.ContextLogf(ctx, "Resolved host router type to be %q", rtype)

	switch rtype {
	case support.LegacyT:
		return legacy.NewRouter(ctx, daemonCtx, host, name)
	case support.AxT:
		return ax.NewRouter(ctx, daemonCtx, host, name)
	case support.OpenWrtT:
		return openwrt.NewRouter(ctx, daemonCtx, host, name)
	case support.UbuntuT:
		return ubuntu.NewRouter(ctx, daemonCtx, host, name)
	case support.UnknownT:
		return nil, errors.New("unable to resolve specific router type from host")
	default:
		return nil, errors.Errorf("unexpected routerType, got %v", rtype)
	}
}

func (tf *TestFixture) resolveRouterTypeFromHost(ctx context.Context, host *ssh.Conn) (support.RouterType, error) {
	if isLegacy, err := legacy.HostIsLegacyRouter(ctx, host); err != nil {
		return -1, err
	} else if isLegacy {
		return support.LegacyT, nil
	}
	if isOpenWrt, err := openwrt.HostIsOpenWrtRouter(ctx, host); err != nil {
		return -1, err
	} else if isOpenWrt {
		return support.OpenWrtT, nil
	}
	if isAx, err := ax.HostIsAXRouter(ctx, host); err != nil {
		return -1, err
	} else if isAx {
		return support.AxT, nil
	}
	if isUbuntu, err := ubuntu.HostIsUbuntuRouter(ctx, host); err != nil {
		return -1, err
	} else if isUbuntu {
		return support.UbuntuT, nil
	}
	return support.UnknownT, nil
}

// Reinit re-initializes the TestFixture by calling both ReinitDUT and
// ReinitRouters. This can be used in precondition or between testcases to
// guarantee a cleaner state.
func (tf *TestFixture) Reinit(ctx context.Context) error {
	ctx, t := timing.Start(ctx, "Reinit")
	defer t.End()
	testing.ContextLog(ctx, "[WIFICELL_FIXTURE] Reinit :: START")
	defer testing.ContextLog(ctx, "[WIFICELL_FIXTURE] Reinit :: END")
	if err := tf.ReinitDUT(ctx); err != nil {
		return errors.Wrap(err, "failed to reinit DUT")
	}
	if err := tf.ReinitRouters(ctx, false); err != nil {
		return errors.Wrap(err, "failed to reinit routers")
	}
	return nil
}

// ReinitDUT re-initializes the DUT.
func (tf *TestFixture) ReinitDUT(ctx context.Context) error {
	ctx, t := timing.Start(ctx, "ReinitDUT")
	defer t.End()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	testing.ContextLog(ctx, "[WIFICELL_FIXTURE] ReinitDUT :: START")
	defer testing.ContextLog(ctx, "[WIFICELL_FIXTURE] ReinitDUT :: END")

	if _, err := tf.WifiClient().HealthCheck(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to pass wifi client health check")
	}
	if _, err := tf.WifiClient().ReinitTestState(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to reinit wifi client test state")
	}
	return nil
}

// ReinitRouters re-initializes the routers. The APs are all deconfigured and
// any OpenWrt routers are rebooted (for stability).
//
// The pcap is only rebooted if it is an OpenWrt router and is also being used
// as a router or doPcapReboot is set to true. In most cases (such as between
// tests), the pcap does not need to be rebooted for stability like the routers
// need, as less is done to them to make them unstable. However, it can be
// useful to reboot them during SetUp to ensure they are not in a bad state from
// previous test runs.
func (tf *TestFixture) ReinitRouters(ctx context.Context, doPcapReboot bool) error {
	ctx, t := timing.Start(ctx, "ReinitRouters")
	defer t.End()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	testing.ContextLog(ctx, "[WIFICELL_FIXTURE] ReinitRouters :: START")
	defer testing.ContextLog(ctx, "[WIFICELL_FIXTURE] ReinitRouters :: END")

	// Deconfigure all routers.
	if err := tf.DeconfigAllAPs(ctx); err != nil {
		return errors.Wrap(err, "failed to deconfig all APs")
	}

	// Reboot routers.
	var routersToReboot []*routerData
	if doPcapReboot && !tf.pcapIsRouter {
		routersToReboot = append(routersToReboot, tf.pcap)
	}
	for _, rd := range tf.routers {
		routersToReboot = append(routersToReboot, rd)
	}
	if len(routersToReboot) > 0 {
		testing.ContextLogf(ctx, "Rebooting %d routers", len(routersToReboot))
		for _, rd := range routersToReboot {
			if err := tf.rebootRouter(ctx, rd); err != nil {
				return err
			}
		}
		testing.ContextLogf(ctx, "Rebooted and re-initialized %d routers", len(routersToReboot))
	} else {
		testing.ContextLog(ctx, "Skipping router reboot step: No routers")
	}

	if tf.options.EnableBridgeAndVeth {
		for i, rd := range tf.routers {
			// Configure bridges and veth on routers except pcap.
			if rd != tf.pcap {
				if err := tf.initializeBridgeAndVethOnRouter(ctx, rd); err != nil {
					return errors.Wrapf(err, "failed to initialize bridges and veths on router %d", i)
				}
				testing.ContextLogf(ctx, "Re-initialized bridges and veths at router %d", i)
			}
		}
	}

	return nil
}

func (tf *TestFixture) rebootRouter(ctx context.Context, rd *routerData) error {
	ctx, t := timing.Start(ctx, "rebootRouter_"+rd.object.RouterType().String())
	defer t.End()
	routerName := rd.object.RouterName()
	routerType := rd.object.RouterType()
	routerMsgName := fmt.Sprintf("%s router %q", routerType.String(), routerName)
	testing.ContextLogf(ctx, "[WIFICELL_FIXTURE] rebootRouter :: %s :: START", routerMsgName)
	defer testing.ContextLogf(ctx, "[WIFICELL_FIXTURE] rebootRouter :: %s :: END", routerMsgName)

	if !tf.options.EnableRouterReboot {
		testing.ContextLogf(ctx, "Skipping reboot of %s: fixture option EnableRouterReboot is false", routerName)
		return nil
	}

	if rd.object.RouterType() != support.OpenWrtT {
		testing.ContextLogf(ctx, "Skipping reboot of %s: Router is not an OpenWrt router", routerName)
		return nil
	}

	// Close and reboot router.
	testing.ContextLogf(ctx, "Preparing %s for reboot", routerMsgName)
	if err := rd.object.Close(ctx); err != nil {
		testing.ContextLogf(ctx, "Failed to close %s before reboot, err: %v", routerMsgName, err)
	}
	testing.ContextLogf(ctx, "Rebooting %s", routerMsgName)
	if err := rd.object.StartReboot(ctx); err != nil {
		return errors.Wrapf(err, "failed to reboot %s", routerMsgName)
	}
	_ = rd.host.Close(ctx)
	rd.host = nil
	rd.object = nil
	rd.brveth = nil

	// Give the router a moment to shut down before trying to reconnect.
	testing.ContextLogf(ctx, "Waiting %s before trying to reconnect to %s", routerPostRebootWaitTime, routerMsgName)
	//GoBigSleepLint. Sleep is a minimum time, enhanced by a later poll.
	if err := testing.Sleep(ctx, routerPostRebootWaitTime); err != nil {
		return errors.Wrapf(err, "failed to wait for %s after rebooting %s", routerPostRebootWaitTime, routerMsgName)
	}

	// Reconnect to router and create a new router controller.
	testing.ContextLogf(ctx, "Reconnecting to %s", routerMsgName)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		routerHost, err := tf.connectCompanion(ctx, rd.target, true)
		if err != nil {
			return errors.Wrapf(err, "failed to reconnect to %s after reboot", routerMsgName)
		}
		rd.host = routerHost
		return nil
	}, &testing.PollOptions{
		Interval: 1 * time.Second,
		Timeout:  90 * time.Second,
	}); err != nil {
		return err
	}
	testing.ContextLogf(ctx, "Reconnected to %s", routerMsgName)

	testing.ContextLogf(ctx, "Reinitializing %s", routerMsgName)
	routerObject, err := tf.newRouter(ctx, ctx, rd.host, routerName)
	if err != nil {
		return errors.Wrapf(err, "failed to reinitialize %s", routerMsgName)
	}
	rd.object = routerObject
	testing.ContextLogf(ctx, "Reconnected to %s with new router controller after reboot", routerMsgName)
	return nil
}

// Close closes the connections created by TestFixture.
func (tf *TestFixture) Close(ctx context.Context) (firstErr error) {
	ctx, st := timing.Start(ctx, "tf.Close")
	defer st.End()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	testing.ContextLog(ctx, "[WIFICELL_FIXTURE] Close :: START")
	defer testing.ContextLog(ctx, "[WIFICELL_FIXTURE] Close :: END")

	for i := range tf.duts {
		testing.ContextLogf(ctx, "Resetting NetCertStore on dut[%d]", i)
		if err := tf.resetNetCertStore(ctx, DutIdx(i)); err != nil {
			utils.CollectFirstErr(ctx, &firstErr, errors.Wrap(err, "failed to reset the NetCertStore"))
		}
	}

	// Close attenuator.
	if tf.attenuator != nil {
		testing.ContextLog(ctx, "Closing attenuator")
		tf.attenuator.Close()
		tf.attenuator = nil
		testing.ContextLog(ctx, "Closed attenuator")
	}

	// Close all routers, starting with the pcap.
	if !tf.pcapIsRouter && tf.pcap != nil {
		rd := tf.pcap
		routerDescription := fmt.Sprintf("pcap router target %q", rd.target)
		testing.ContextLogf(ctx, "Closing %s", routerDescription)
		if rd.object != nil {
			if err := rd.object.Close(ctx); err != nil {
				utils.CollectFirstErr(ctx, &firstErr, errors.Wrapf(err, "failed to close router controller for %s", routerDescription))
			}
		}
		if rd.host != nil {
			if err := rd.host.Close(ctx); err != nil {
				utils.CollectFirstErr(ctx, &firstErr, errors.Wrapf(err, "failed to close ssh connection to %s", routerDescription))
			}
		}
		testing.ContextLogf(ctx, "Closed %s", routerDescription)
	}
	for i, rd := range tf.routers {
		routerDescription := fmt.Sprintf("primary router[%d] target %q", i, rd.target)
		testing.ContextLogf(ctx, "Closing %s", routerDescription)

		// De-configure bridges on routers except pcap.
		if rd.br != nil && rd != tf.pcap {
			testing.ContextLogf(ctx, "Closing bridges at router %d", i)
			br := rd.br
			if err := br.r.ReleaseBridge(ctx, br.br); err != nil {
				utils.CollectFirstErr(ctx, &firstErr, errors.Wrapf(err, "failed to release bridge %q: ", br.br))
			}
			testing.ContextLogf(ctx, "Closed bridges at router %d", i)
		}

		// De-configure bridges and veths on routers except pcap.
		if rd.brveth != nil && rd != tf.pcap {
			testing.ContextLogf(ctx, "Closing bridges and veths at router %d", i)
			bv := rd.brveth

			for j := 0; j < 2; j++ {
				if err := bv.r.UnbindVeth(ctx, bv.veth[j]); err != nil {
					utils.CollectFirstErr(ctx, &firstErr, errors.Wrapf(err, "failed to unbind %q", bv.veth[j]))
				}
				if err := bv.r.ReleaseBridge(ctx, bv.br[j]); err != nil {
					utils.CollectFirstErr(ctx, &firstErr, errors.Wrapf(err, "failed to release bridge %q: ", bv.br[j]))
				}
			}
			if err := bv.r.ReleaseVethPair(ctx, bv.veth[0]); err != nil {
				utils.CollectFirstErr(ctx, &firstErr, errors.Wrap(err, "failed to release veth"))
			}
			testing.ContextLogf(ctx, "Closed bridges and veths at router %d", i)
		}

		if rd.object != nil {
			if err := rd.object.Close(ctx); err != nil {
				utils.CollectFirstErr(ctx, &firstErr, errors.Wrapf(err, "failed to close router controller for %s", routerDescription))
			}
		}
		if rd.host != nil {
			if err := rd.host.Close(ctx); err != nil {
				utils.CollectFirstErr(ctx, &firstErr, errors.Wrapf(err, "failed to close ssh connection to %s", routerDescription))
			}
		}
		testing.ContextLogf(ctx, "Closed %s", routerDescription)
	}

	// Close dut connections. Do not close DUT, it'll be closed by the framework.
	for i, d := range tf.duts {
		dutDescription := fmt.Sprintf("dut[%d]", i)
		if d.wifiClient != nil {
			if tf.options.SetDutWifiLogging {
				testing.ContextLogf(ctx, "Setting logging config back to original settings for %s", dutDescription)
				if err := tf.setLoggingConfig(ctx, d.wifiClient, d.originalLogLevel, d.originalLogTags); err != nil {
					utils.CollectFirstErr(ctx, &firstErr, errors.Wrapf(err, "failed to tear down test state for %s", dutDescription))
				}
			}
			testing.ContextLogf(ctx, "Tearing down wifi client for %s", dutDescription)
			if _, err := d.wifiClient.TearDown(ctx, &empty.Empty{}); err != nil {
				utils.CollectFirstErr(ctx, &firstErr, errors.Wrapf(err, "failed to tear down test state for %s", dutDescription))
			}
			d.wifiClient = nil
		}
		testing.ContextLogf(ctx, "Closing RPC connections to %s", dutDescription)
		if d.rpc != nil {
			// Ignore the error of rpc.Close as aborting rpc daemon will always have error.
			_ = d.rpc.Close(ctx)
			d.rpc = nil
		}
	}
	return firstErr
}

// ReserveForClose returns a shorter ctx and cancel function for tf.Close().
func (tf *TestFixture) ReserveForClose(ctx context.Context) (context.Context, context.CancelFunc) {
	return ctxutil.Shorten(ctx, 10*time.Second)
}

// CollectLogs downloads related log files to OutDir.
func (tf *TestFixture) CollectLogs(ctx context.Context) error {
	var firstErr error
	for _, rt := range tf.routers {
		// Assert router can collect logs
		r, ok := rt.object.(support.Logs)
		if !ok {
			return errors.Errorf("router type %q does not support Logs", rt.object.RouterType().String())
		}
		err := r.CollectLogs(ctx)
		if err != nil {
			utils.CollectFirstErr(ctx, &firstErr, errors.Wrap(err, "failed to collect logs"))
		}
	}
	return firstErr
}

// ReserveForCollectLogs returns a shorter ctx and cancel function for tf.CollectRouterFileLogs.
func (tf *TestFixture) ReserveForCollectLogs(ctx context.Context) (context.Context, context.CancelFunc) {
	return ctxutil.Shorten(ctx, time.Second)
}

// UniqueAPName returns a unique ID string for each AP as their name, so that related
// logs/pcap can be identified easily.
func (tf *TestFixture) UniqueAPName() string {
	id := strconv.Itoa(tf.apID)
	tf.apID++
	return id
}

// ConfigureAPOnRouterID is an extended version of ConfigureAP, allowing to choose router
// to establish the AP on.
func (tf *TestFixture) ConfigureAPOnRouterID(ctx context.Context, idx RouterIdx, ops []hostapd.Option, fac security.ConfigFactory, enableDNS, enableHTTP bool) (ret *APIface, retErr error) {
	return tf.ConfigureAPOnRouterIDWithConfs(ctx, idx, []hostapd.ApConfig{{ApOpts: ops, SecConfFac: fac}}, "", true, enableDNS, enableHTTP)
}

// ConfigureAPOnRouterIDWithConfs is an extended version of ConfigureAPOnRouterID, allowing
// to choose router and interfaces to establish the AP on, for example, hostapd controls multiple interfaces.
// apConfigs[i].ApOpts and .SecConfFac are the hostapd options and security configurations of Interface i
// If enableDHCP is true, the DHCP server is installed on dhcpIface
// When dhcpIface is empty, the DHCP server uses the interface of the hostapd server if the hostapd server has
// only one interface; otherwise it is installed on a bridge which connects all hostapd interfaces
func (tf *TestFixture) ConfigureAPOnRouterIDWithConfs(ctx context.Context, idx RouterIdx, apConfigs []hostapd.ApConfig, dhcpIface string, enableDHCP, enableDNS, enableHTTP bool) (ret *APIface, retErr error) {
	ctx, st := timing.Start(ctx, "tf.ConfigureAPOnRouterIDWithConfs")
	defer st.End()

	if len(tf.routers) <= int(idx) {
		return nil, errors.Errorf("router index (%d) out of range [0, %d)", idx, len(tf.routers))
	}

	r := tf.routers[idx].object
	name := tf.UniqueAPName()

	var configs []*hostapd.Config
	capturers := make(map[int]*pcap.Capturer)
	for i := 0; i < len(apConfigs); i++ {
		ops := apConfigs[i].ApOpts
		fac := apConfigs[i].SecConfFac
		if fac != nil {
			// Defer the securityConfig generation from test's init() to here because the step may emit error and that's not allowed in test init().
			securityConfig, err := fac.Gen()
			if err != nil {
				return nil, err
			}
			ops = append([]hostapd.Option{hostapd.SecurityConfig(securityConfig)}, ops...)
		}
		config, err := hostapd.NewConfig(ops...)
		if err != nil {
			return nil, err
		}
		configs = append(configs, config)
		var capturer *pcap.Capturer
		if tf.options.EnablePacketCapture {
			freqOps, err := config.PcapFreqOptions()
			if err != nil {
				return nil, err
			}
			capturer, err = tf.PcapRouter().StartCapture(ctx, tf.UniqueAPName(), config.Channel, config.OpClass, freqOps)
			if err != nil {
				return nil, errors.Wrap(err, "failed to start capturer")
			}
			defer func() {
				if retErr != nil {
					tf.PcapRouter().StopCapture(ctx, capturer)
				}
			}()
			if capturer != nil {
				freq, err := hostapd.ChannelToFrequencyWithOpClass(config.Channel, config.OpClass)
				if err != nil {
					return nil, errors.Wrap(err, "failed to calculate the frequency of the capturer")
				}
				capturers[freq] = capturer
			}
		}
	}
	var ap *APIface
	var err error
	if len(configs) > 1 {
		router, ok := r.(router.StandardWithBridge)
		if !ok {
			return nil, errors.New("router is not a standard router with bridge support")
		}
		bridge, err := router.NewBridge(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get a bridge")
		}
		tf.routers[idx].br = &bridgeData{r: router, br: bridge}
		if dhcpIface == "" {
			dhcpIface = bridge
		}
		ap, err = StartAPIface(ctx, r, name, dhcpIface, enableDHCP, enableDNS, enableHTTP, configs...)
		if err != nil {
			return nil, errors.Wrap(err, "failed to start APIface")
		}
		for _, iface := range ap.Interfaces() {
			router.BindIfaceToBridge(ctx, iface, bridge)
		}
	} else {
		ap, err = StartAPIface(ctx, r, name, dhcpIface, enableDHCP, enableDNS, enableHTTP, configs...)
		if err != nil {
			return nil, errors.Wrap(err, "failed to start APIface")
		}
	}
	tf.capturers[ap] = capturers
	tf.aps[ap] = struct{}{}
	return ap, nil
}

// ConfigureAP configures the router to provide a WiFi service with the options specified.
// Note that after getting an APIface, ap, the caller should defer tf.DeconfigAP(ctx, ap) and
// use tf.ReserveForClose(ctx, ap) to reserve time for the deferred call.
func (tf *TestFixture) ConfigureAP(ctx context.Context, ops []hostapd.Option, fac security.ConfigFactory) (ret *APIface, retErr error) {
	return tf.ConfigureAPOnRouterID(ctx, DefaultRouter, ops, fac, false, false)
}

// ConfigureMultiAP configures the router[idx] to provide multiple WiFi services(AP's)
// with corresponding ApConfigs specified. All AP's are colocated, which means there
// is only one hostapd controlling all the AP's.
// Note that after getting an APIface, ap, the caller should defer tf.DeconfigAP(ctx, ap) and
// use tf.ReserveForClose(ctx, ap) to reserve time for the deferred call.
func (tf *TestFixture) ConfigureMultiAP(ctx context.Context, idx RouterIdx, apConfigs []hostapd.ApConfig) (ret *APIface, retErr error) {
	return tf.ConfigureAPOnRouterIDWithConfs(ctx, idx, apConfigs, "", true, false, false)
}

// ReserveForDeconfigAP returns a shorter ctx and cancel function for tf.DeconfigAP().
func (tf *TestFixture) ReserveForDeconfigAP(ctx context.Context, ap *APIface) (context.Context, context.CancelFunc) {
	if len(tf.routers) == 0 {
		return ctx, func() {}
	}
	ctx, cancel := ap.ReserveForStop(ctx)
	if capturers, ok := tf.capturers[ap]; ok {
		// Also reserve time for stopping the capturer if it exists.
		// Noted that CancelFunc returned here is dropped as we rely on its
		// parent's cancel() being called.
		for _, capturer := range capturers {
			ctx, _ = tf.PcapRouter().ReserveForStopCapture(ctx, capturer)
		}
	}
	return ctx, cancel
}

// DeconfigAP stops the WiFi service on router.
func (tf *TestFixture) DeconfigAP(ctx context.Context, ap *APIface) (firstErr error) {
	ctx, st := timing.Start(ctx, "tf.DeconfigAP")
	defer st.End()
	capturers := tf.capturers[ap]
	delete(tf.capturers, ap)
	ifaces := ap.Interfaces()
	// If ap has multiple interfaces, these interfaces are bound to a bridge,
	// unbind these interfaces before stopping ap
	if len(ifaces) > 1 {
		if ap.Router() != nil {
			if router, ok := ap.Router().(router.StandardWithBridge); !ok {
				utils.CollectFirstErr(ctx, &firstErr, errors.New("router is not a standard router with bridge support"))
			} else {
				for _, iface := range ifaces {
					if err := router.UnbindIface(ctx, iface); err != nil {
						utils.CollectFirstErr(ctx, &firstErr, errors.Wrapf(err, "failed to unbind %q", iface))
					}
				}
			}
		} else {
			utils.CollectFirstErr(ctx, &firstErr, errors.New("router of ap is nil"))
		}
	}

	if err := ap.Stop(ctx); err != nil {
		utils.CollectFirstErr(ctx, &firstErr, errors.Wrap(err, "failed to stop APIface"))
	}
	if capturers != nil {
		for _, capturer := range capturers {
			if err := tf.PcapRouter().StopCapture(ctx, capturer); err != nil {
				utils.CollectFirstErr(ctx, &firstErr, errors.Wrap(err, "failed to stop capturer"))
			}
		}
	}
	return firstErr
}

// DeconfigAllAPs facilitates deconfiguration of all APs established for
// this test fixture.
func (tf *TestFixture) DeconfigAllAPs(ctx context.Context) error {
	var firstErr error
	for ap := range tf.aps {
		if err := tf.DeconfigAP(ctx, ap); err != nil {
			utils.CollectFirstErr(ctx, &firstErr, errors.Wrap(err, "failed to deconfig AP"))
		}
	}
	return firstErr
}

// StartWPAMonitor configures and starts wpa_supplicant events monitor
// newCtx is ctx shortened for the stop function, which should be deferred by the caller.
func (tf *TestFixture) StartWPAMonitor(ctx context.Context, dutIdx DutIdx) (wpaMonitor *wpacli.WPAMonitor, stop func(), newCtx context.Context, retErr error) {
	const wpaMonitorStopTimeout = 10 * time.Second
	wpaRunner := remotewpacli.NewRemoteRunner(tf.duts[dutIdx].dut.Conn())
	wpaMonitor = wpaRunner.NewWPAMonitor()
	stop, newCtx, err := wpaMonitor.StartWPAMonitor(ctx, wpaMonitorStopTimeout)
	if err != nil {
		return nil, nil, ctx, err
	}
	return wpaMonitor, stop, newCtx, nil
}

// ConnectWifi is backwards-compatible version of ConnectWifiFromDUT. Deprecated.
// TODO(b/234845693): remove after stabilizing period.
func (tf *TestFixture) ConnectWifi(ctx context.Context, ssid string, options ...dutcfg.ConnOption) (*wifi.ConnectResponse, error) {
	return tf.ConnectWifiFromDUT(ctx, DefaultDUT, ssid, options...)
}

// ConnectWifiFromDUT asks the DUT #dutIdx to connect to the specified WiFi.
func (tf *TestFixture) ConnectWifiFromDUT(ctx context.Context, dutIdx DutIdx, ssid string, options ...dutcfg.ConnOption) (*wifi.ConnectResponse, error) {
	c := &dutcfg.ConnConfig{
		Ssid:    ssid,
		SecConf: &base.Config{},
	}
	for _, op := range options {
		op(c)
	}
	ctx, st := timing.Start(ctx, "tf.ConnectWifiFromDUT")
	defer st.End()

	// Setup the NetCertStore only for EAP-related tests.
	if c.SecConf.NeedsNetCertStore() {
		if err := tf.setupNetCertStore(ctx, dutIdx); err != nil {
			return nil, errors.Wrap(err, "failed to set up the NetCertStore")
		}

		if err := c.SecConf.InstallClientCredentials(ctx, tf.duts[dutIdx].netCertStore); err != nil {
			return nil, errors.Wrap(err, "failed to install client credentials")
		}
	}

	secProps, err := c.SecConf.ShillServiceProperties()
	if err != nil {
		return nil, err
	}

	props := make(map[string]interface{})
	for k, v := range c.Props {
		props[k] = v
	}
	for k, v := range secProps {
		props[k] = v
	}

	propsEnc, err := protoutil.EncodeToShillValMap(props)
	if err != nil {
		return nil, err
	}
	request := &wifi.ConnectRequest{
		Ssid:          []byte(c.Ssid),
		Hidden:        c.Hidden,
		SecurityClass: c.SecConf.Class(),
		Shillprops:    propsEnc,
	}
	response, err := tf.duts[dutIdx].wifiClient.Connect(ctx, request)
	if err != nil {
		return nil, errors.Wrapf(err, "client failed to connect to WiFi network with SSID %q", c.Ssid)
	}
	return response, nil
}

// setupNetCertStore sets up tf.netCertStore for EAP-related tests.
func (tf *TestFixture) setupNetCertStore(ctx context.Context, dutIdx DutIdx) error {
	if tf.duts[dutIdx].netCertStore != nil {
		// Nothing to do if it was set up.
		return nil
	}

	runner := hwsec.NewCmdRunner(tf.duts[dutIdx].dut)
	var err error
	tf.duts[dutIdx].netCertStore, err = netcertstore.CreateStore(ctx, runner)
	return err
}

// resetNetCertStore nullifies tf.netCertStore.
func (tf *TestFixture) resetNetCertStore(ctx context.Context, dutIdx DutIdx) error {
	if tf.duts[dutIdx].netCertStore == nil {
		// Nothing to do if it was not set up.
		return nil
	}

	err := tf.duts[dutIdx].netCertStore.Cleanup(ctx)
	tf.duts[dutIdx].netCertStore = nil
	return err
}

// ConnectWifiAPFromDUT asks the given DUT to connect to the WiFi provided by the given AP.
func (tf *TestFixture) ConnectWifiAPFromDUT(ctx context.Context, dutIdx DutIdx, ap *APIface, options ...dutcfg.ConnOption) (*wifi.ConnectResponse, error) {
	conf := ap.Config()
	opts := append([]dutcfg.ConnOption{dutcfg.ConnHidden(conf.Hidden), dutcfg.ConnSecurity(conf.SecurityConfig)}, options...)
	return tf.ConnectWifiFromDUT(ctx, dutIdx, conf.SSID, opts...)
}

// ConnectWifiAP is backwards-compatible version of ConnectWifiAPFromDUT. Deprecated.
// TODO(b/234845693): remove after stabilizing period.
func (tf *TestFixture) ConnectWifiAP(ctx context.Context, ap *APIface, options ...dutcfg.ConnOption) (*wifi.ConnectResponse, error) {
	return tf.ConnectWifiAPFromDUT(ctx, DefaultDUT, ap, options...)
}

func (tf *TestFixture) disconnectWifi(ctx context.Context, dutIdx DutIdx, removeProfile bool) error {
	ctx, st := timing.Start(ctx, "tf.disconnectWifi")
	defer st.End()

	resp, err := tf.duts[dutIdx].wifiClient.SelectedService(ctx, &empty.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get selected service")
	}

	// Note: It is possible that selected service changed after SelectService call,
	// but we are usually in a stable state when calling this. If not, the Disconnect
	// call will also fail and caller usually leaves hint for this.
	// In Close and Reinit, we pop + remove related profiles so it should still be
	// safe for next test if this case happened in clean up.
	req := &wifi.DisconnectRequest{
		ServicePath:   resp.ServicePath,
		RemoveProfile: removeProfile,
	}
	if _, err := tf.duts[dutIdx].wifiClient.Disconnect(ctx, req); err != nil {
		return errors.Wrap(err, "failed to disconnect")
	}
	return nil
}

// DisconnectWifi is backwards-compatible version of DisconnectDUTFromWifi. Deprecated.
// TODO(b/234845693): remove after stabilizing period.
func (tf *TestFixture) DisconnectWifi(ctx context.Context) error {
	return tf.disconnectWifi(ctx, DefaultDUT, false)
}

// CleanDisconnectWifi is backwards-compatible version of CleanDisconnectDUTFromWifi. Deprecated.
// TODO(b/234845693): remove after stabilizing period.
func (tf *TestFixture) CleanDisconnectWifi(ctx context.Context) error {
	return tf.disconnectWifi(ctx, DefaultDUT, true)
}

// DisconnectDUTFromWifi asks the given DUT to disconnect from current WiFi service.
func (tf *TestFixture) DisconnectDUTFromWifi(ctx context.Context, dutIdx DutIdx) error {
	return tf.disconnectWifi(ctx, dutIdx, false)
}

// CleanDisconnectDUTFromWifi asks the given DUT to disconnect from current WiFi service and removes the configuration.
func (tf *TestFixture) CleanDisconnectDUTFromWifi(ctx context.Context, dutIdx DutIdx) error {
	return tf.disconnectWifi(ctx, dutIdx, true)
}

// ReserveForDisconnect returns a shorter ctx and cancel function for tf.DisconnectWifi.
func (tf *TestFixture) ReserveForDisconnect(ctx context.Context) (context.Context, context.CancelFunc) {
	return ctxutil.Shorten(ctx, 5*time.Second)
}

// ReserveForReboot returns a shorter ctx and cancel function for DUT().Reboot().
func (tf *TestFixture) ReserveForReboot(ctx context.Context) (context.Context, context.CancelFunc) {
	return ctxutil.Shorten(ctx, 10*time.Second)
}

// PingFromDUT is backwards-compatible version of PingFromSpecificDUT. Deprecated.
// TODO(b/234845693): remove after stabilizing period.
func (tf *TestFixture) PingFromDUT(ctx context.Context, targetIP string, opts ...ping.Option) error {
	res, err := tf.PingFromSpecificDUT(ctx, DefaultDUT, targetIP, opts...)
	if err != nil {
		return err
	}
	return VerifyPingResults(res, pingLossThreshold)
}

// PingFromSpecificDUT tests the connectivity between the given DUT and a target IP.
func (tf *TestFixture) PingFromSpecificDUT(ctx context.Context, dutIdx DutIdx, targetIP string, opts ...ping.Option) (*ping.Result, error) {
	iface, err := tf.DUTClientInterface(ctx, dutIdx)
	if err != nil {
		return nil, errors.Wrap(err, "DUT: failed to get the client WiFi interface")
	}

	// Bind ping used in all WiFi Tests to WiFiInterface. Otherwise if the
	// WiFi interface is not up yet they will be routed through the Ethernet
	// interface. Also see b/225205611 for details.
	opts = append(opts, ping.BindAddress(true), ping.SourceIface(iface))

	ctx, st := timing.Start(ctx, "tf.PingFromSpecificDUT")
	defer st.End()

	pr := remoteping.NewRemoteRunner(tf.duts[dutIdx].dut.Conn())
	res, err := pr.Ping(ctx, targetIP, opts...)
	if err != nil {
		return nil, err
	}
	testing.ContextLogf(ctx, "ping statistics=%+v", res)
	return res, nil
}

// VerifyPingResults checks if ping results are within acceptable range (loss not exceeding the lossThreshold, in %).
func VerifyPingResults(res *ping.Result, lossThreshold float64) error {

	if res.Loss > lossThreshold {
		return errors.Errorf("unexpected packet loss percentage: got %g%%, want <= %g%%", res.Loss, lossThreshold)
	}

	return nil
}

// PingFromRouterID tests the connectivity between DUT and router through currently connected WiFi service.
func (tf *TestFixture) PingFromRouterID(ctx context.Context, idx int, opts ...ping.Option) error {
	ctx, st := timing.Start(ctx, "tf.PingFromRouterID")
	defer st.End()

	addrs, err := tf.ClientIPv4Addrs(ctx)
	if err != nil || len(addrs) == 0 {
		return errors.Wrap(err, "failed to get the IP address")
	}

	pr := remoteping.NewRemoteRunner(tf.routers[idx].host)
	res, err := pr.Ping(ctx, addrs[0].String(), opts...)
	if err != nil {
		return err
	}
	testing.ContextLogf(ctx, "ping statistics=%+v", res)

	return VerifyPingResults(res, pingLossThreshold)
}

// PingFromServer calls PingFromRouterID for router 0.
// Kept for backwards-compatibility.
func (tf *TestFixture) PingFromServer(ctx context.Context, opts ...ping.Option) error {
	return tf.PingFromRouterID(ctx, 0, opts...)
}

// ArpingFromDUT is backwards-compatible version of ArpingFromSpecificDUT. Deprecated.
// TODO(b/234845693): remove after stabilizing period.
func (tf *TestFixture) ArpingFromDUT(ctx context.Context, serverIP string, ops ...arping.Option) error {
	return tf.ArpingFromSpecificDUT(ctx, DefaultDUT, serverIP, ops...)
}

// ArpingFromSpecificDUT tests that the given DUT can send the broadcast packets to server.
func (tf *TestFixture) ArpingFromSpecificDUT(ctx context.Context, dutIdx DutIdx, serverIP string, ops ...arping.Option) error {
	ctx, st := timing.Start(ctx, "tf.ArpingFromSpecificDUT")
	defer st.End()

	iface, err := tf.DUTClientInterface(ctx, dutIdx)
	if err != nil {
		return errors.Wrap(err, "failed to get the client WiFi interface")
	}

	runner := remotearping.NewRemoteRunner(tf.duts[dutIdx].dut.Conn())
	res, err := runner.Arping(ctx, serverIP, iface, ops...)
	if err != nil {
		return errors.Wrap(err, "arping failed")
	}

	if res.Loss > arpingLossThreshold {
		return errors.Errorf("unexpected arping loss percentage: got %g%% want <= %g%%", res.Loss, arpingLossThreshold)
	}

	return nil
}

// ArpingFromRouterID tests that DUT can receive the broadcast packets from server.
func (tf *TestFixture) ArpingFromRouterID(ctx context.Context, idx int, serverIface string, ops ...arping.Option) error {
	ctx, st := timing.Start(ctx, "tf.ArpingFromRouterID")
	defer st.End()

	addrs, err := tf.ClientIPv4Addrs(ctx)
	if err != nil || len(addrs) == 0 {
		return errors.Wrap(err, "failed to get the IP address")
	}

	runner := remotearping.NewRemoteRunner(tf.routers[idx].host)
	res, err := runner.Arping(ctx, addrs[0].String(), serverIface, ops...)
	if err != nil {
		return errors.Wrap(err, "arping failed")
	}

	if res.Loss > arpingLossThreshold {
		return errors.Errorf("unexpected arping loss percentage: got %g%% want <= %g%%", res.Loss, arpingLossThreshold)
	}

	return nil
}

// ArpingFromServer tests that DUT can receive the broadcast packets from server.
// Kept for backwards-compatibility.
func (tf *TestFixture) ArpingFromServer(ctx context.Context, serverIface string, ops ...arping.Option) error {
	return tf.ArpingFromRouterID(ctx, 0, serverIface, ops...)
}

// ClientIPv4Addrs is backwards-compatible version of DUTIPv4Addrs. Deprecated.
// TODO(b/234845693): remove after stabilizing period.
func (tf *TestFixture) ClientIPv4Addrs(ctx context.Context) ([]net.IP, error) {
	return tf.DUTIPv4Addrs(ctx, DefaultDUT)
}

// DUTIPv4Addrs returns the IPv4 addresses for the network interface.
func (tf *TestFixture) DUTIPv4Addrs(ctx context.Context, dutIdx DutIdx) ([]net.IP, error) {
	iface, err := tf.DUTClientInterface(ctx, dutIdx)
	if err != nil {
		return nil, errors.Wrap(err, "DUT: failed to get the client WiFi interface")
	}

	netIface := &wifi.GetIPv4AddrsRequest{
		InterfaceName: iface,
	}
	addrs, err := tf.DUTWifiClient(dutIdx).GetIPv4Addrs(ctx, netIface)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get the IPv4 addresses")
	}

	ret := make([]net.IP, len(addrs.Ipv4))
	for i, a := range addrs.Ipv4 {
		ret[i], _, err = net.ParseCIDR(a)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse IP address %s", a)
		}
	}

	return ret, nil
}

// ClientHardwareAddr is backwards-compatible version of DUTHardwareAddr. Deprecated.
// TODO(b/234845693): remove after stabilizing period.
func (tf *TestFixture) ClientHardwareAddr(ctx context.Context) (string, error) {
	mac, err := tf.DUTHardwareAddr(ctx, DefaultDUT)
	return mac.String(), err
}

// DUTHardwareAddr returns the HardwareAddr for the network interface.
func (tf *TestFixture) DUTHardwareAddr(ctx context.Context, dutIdx DutIdx) (net.HardwareAddr, error) {
	iface, err := tf.DUTClientInterface(ctx, dutIdx)
	if err != nil {
		return nil, errors.Wrap(err, "DUT: failed to get the client WiFi interface")
	}

	netIface := &wifi.GetHardwareAddrRequest{
		InterfaceName: iface,
	}
	resp, err := tf.DUTWifiClient(dutIdx).GetHardwareAddr(ctx, netIface)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get the HardwareAddr")
	}

	return net.ParseMAC(resp.HwAddr)
}

// AssertNoDisconnect runs the given routine and verifies that no disconnection event
// is captured in the same duration.
func (tf *TestFixture) AssertNoDisconnect(ctx context.Context, dutIdx DutIdx, f func(context.Context) error) error {
	ctx, st := timing.Start(ctx, "tf.AssertNoDisconnect")
	defer st.End()

	el, err := iw.NewEventLogger(ctx, tf.duts[dutIdx].dut)
	if err != nil {
		return errors.Wrap(err, "failed to start iw.EventLogger")
	}
	errf := f(ctx)
	if err := el.Stop(); err != nil {
		// Let's also keep errf if available. Wrapf is equivalent to Errorf when errf==nil.
		return errors.Wrapf(errf, "failed to stop iw.EventLogger, err=%s", err.Error())
	}
	if errf != nil {
		return errf
	}
	evs := el.EventsByType(iw.EventTypeDisconnect)
	if len(evs) != 0 {
		return errors.Errorf("disconnect events captured: %v", evs)
	}
	return nil
}

// VerifyConnection is backwards-compatible version of VerifyConnectionFromDUT. Deprecated.
// TODO(b/234845693): remove after stabilizing period.
func (tf *TestFixture) VerifyConnection(ctx context.Context, ap *APIface) error {
	return tf.VerifyConnectionFromDUT(ctx, 0, ap)
}

// VerifyConnectionFromDUT verifies that the AP is reachable from a specific DUT by pinging, and we have the same frequency and subnet as AP's.
func (tf *TestFixture) VerifyConnectionFromDUT(ctx context.Context, dutIdx DutIdx, ap *APIface) error {
	return tf.duts[dutIdx].verifyStaConnection(ctx, tf.routers[DefaultRouter].host, ap)
}

func (dd *dutData) verifyStaConnection(ctx context.Context, routerConn *ssh.Conn, ap *APIface) error {
	// TODO(b/234845693): move to dut.go
	iface, err := dd.IfName(ctx, StaIfaceType)
	if err != nil {
		return errors.Wrap(err, "failed to get interface from the DUT")
	}

	// Check frequency.
	service, err := dd.wifiClient.QueryService(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to query shill service information")
	}
	clientFreq := service.Wifi.Frequency
	// The channel number might change during the test due to CSA event. Use iw command to
	// get the current AP channel.
	iwr := iw.NewRemoteRunner(routerConn)
	chConfig, err := iwr.RadioConfig(ctx, ap.Interface())
	if err != nil {
		return errors.Wrap(err, "failed to get the radio configuration")
	}
	serverFreq, err := hostapd.ChannelToFrequencyWithOpClass(chConfig.Number, ap.Config().OpClass)
	if err != nil {
		return errors.Wrap(err, "failed to get server frequency")
	}
	if clientFreq != uint32(serverFreq) {
		return errors.Errorf("frequency does not match, got %d want %d", clientFreq, serverFreq)
	}

	// Check subnet.
	addrs, err := dd.wifiClient.GetIPv4Addrs(ctx, &wifi.GetIPv4AddrsRequest{InterfaceName: iface})
	if err != nil {
		return errors.Wrap(err, "failed to get client ipv4 addresses")
	}
	serverSubnet := ap.ServerSubnet().String()
	foundSubnet := false
	for _, a := range addrs.Ipv4 {
		_, ipnet, err := net.ParseCIDR(a)
		if err != nil {
			return errors.Wrapf(err, "failed to parse IP address %s", a)
		}
		if ipnet.String() == serverSubnet {
			foundSubnet = true
			break
		}
	}
	if !foundSubnet {
		return errors.Errorf("subnet does not match, got addrs %v want %s", addrs.Ipv4, serverSubnet)
	}

	// Perform ping.
	res, err := dd.Ping(ctx, ap.ServerIP().String(), StaIfaceType)
	if err != nil {
		return errors.Wrap(err, "failed to ping from the DUT")
	}
	if err := VerifyPingResults(res, pingLossThreshold); err != nil {
		return errors.Wrap(err, "ping verification failed")
	}

	return nil
}

// ClearBSSIDIgnoreDUT clears the BSSID_IGNORE list on DUT.
func (tf *TestFixture) ClearBSSIDIgnoreDUT(ctx context.Context, dutIdx DutIdx) error {
	wpar := remotewpacli.NewRemoteRunner(tf.duts[dutIdx].dut.Conn())

	err := wpar.ClearBSSIDIgnore(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to clear WPA BSSID_IGNORE")
	}

	return nil
}

// AddToBSSIDIgnoreDUT adds the passed BSSID into BSSID_IGNORE list on DUT.
func (tf *TestFixture) AddToBSSIDIgnoreDUT(ctx context.Context, dutIdx DutIdx, bssid string) error {
	wpar := remotewpacli.NewRemoteRunner(tf.duts[dutIdx].dut.Conn())
	err := wpar.AddToBSSIDIgnore(ctx, bssid)
	if err != nil {
		return errors.Wrapf(err, "failed to add %s to WPA BSSID_IGNORE", bssid)
	}
	return nil
}

// SendChannelSwitchAnnouncement sends a CSA frame and waits for Client_Disconnection, or Channel_Switch event.
func (tf *TestFixture) SendChannelSwitchAnnouncement(ctx context.Context, dutIdx DutIdx, ap *APIface, maxRetry, alternateChannel int) error {
	ctxForCloseFrameSender := ctx
	r, ok := tf.Router().(support.FrameSender)
	if !ok {
		return errors.Errorf("router type %q does not support FrameSender", tf.Router().RouterType().String())
	}
	ctx, cancel := ctxutil.Shorten(ctx, common.RouterCloseFrameSenderDuration)
	defer cancel()
	sender, err := r.NewFrameSender(ctx, ap.Interface())
	if err != nil {
		return errors.Wrap(err, "failed to create frame sender")
	}
	defer func(ctx context.Context) error {
		if err := r.CloseFrameSender(ctx, sender); err != nil {
			return errors.Wrap(err, "failed to close frame sender")
		}
		return nil
	}(ctxForCloseFrameSender)

	ew, err := iw.NewEventWatcher(ctx, tf.duts[dutIdx].dut)
	if err != nil {
		return errors.Wrap(err, "failed to start iw.EventWatcher")
	}
	defer ew.Stop()

	// Action frame might be lost, give it some retries.
	for i := 0; i < maxRetry; i++ {
		testing.ContextLogf(ctx, "Try sending channel switch frame %d", i)
		if err := sender.Send(ctx, framesender.TypeChannelSwitch, alternateChannel); err != nil {
			return errors.Wrap(err, "failed to send channel switch frame")
		}
		// The frame might need some time to reach DUT, wait for a few seconds.
		wCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		// TODO(b/154879577): Find some way to know if DUT supports
		// channel switch, and only wait for the proper event.
		_, err := ew.WaitByType(wCtx, iw.EventTypeChanSwitch, iw.EventTypeDisconnect)
		if err == context.DeadlineExceeded {
			// Retry if deadline exceeded.
			continue
		}
		if err != nil {
			return errors.Wrap(err, "failed to wait for iw event")
		}
		// Channel switch or client disconnection detected.
		return nil
	}

	return errors.New("failed to disconnect client or switch channel")
}

// DisablePowersaveMode disables power saving mode (if it's enabled) and return a function to restore it's initial mode.
func (tf *TestFixture) DisablePowersaveMode(ctx context.Context, dutIdx DutIdx) (shortenCtx context.Context, restore func() error, err error) {
	iwr := iw.NewRemoteRunner(tf.duts[dutIdx].dut.Conn())
	iface, err := tf.ClientInterface(ctx)
	if err != nil {
		return ctx, nil, errors.Wrap(err, "failed to get the client interface")
	}

	ctxForResetingPowersaveMode := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Second)
	psMode, err := iwr.PowersaveMode(ctx, iface)
	if err != nil {
		return ctx, nil, errors.Wrap(err, "failed to get the powersave mode")
	}
	if psMode {
		restore := func() error {
			cancel()
			testing.ContextLogf(ctxForResetingPowersaveMode, "Restoring power save mode to %t", psMode)
			if err := iwr.SetPowersaveMode(ctxForResetingPowersaveMode, iface, psMode); err != nil {
				return errors.Wrapf(err, "failed to restore powersave mode to %t", psMode)
			}
			return nil
		}
		testing.ContextLog(ctx, "Disabling power save in the test")
		if err := iwr.SetPowersaveMode(ctx, iface, false); err != nil {
			return ctx, nil, errors.Wrap(err, "failed to turn off powersave")
		}
		return ctx, restore, nil
	}

	// Power saving mode already disabled.
	return ctxForResetingPowersaveMode, func() error { return nil }, nil
}

// setLoggingConfig configures the logging setting with the specified values (level and tags).
func (tf *TestFixture) setLoggingConfig(ctx context.Context, wc *WifiClient, level int, tags []string) error {
	testing.ContextLogf(ctx, "Configuring logging level: %d, tags: %v", level, tags)
	_, err := wc.SetLoggingConfig(ctx, &wifi.SetLoggingConfigRequest{DebugLevel: int32(level), DebugTags: tags})
	return err
}

// getLoggingConfig returns the current DUT's logging setting (level and tags).
func (tf *TestFixture) getLoggingConfig(ctx context.Context, wc *WifiClient) (int, []string, error) {
	currentConfig, err := wc.GetLoggingConfig(ctx, &empty.Empty{})
	if err != nil {
		return 0, nil, err
	}
	return int(currentConfig.DebugLevel), currentConfig.DebugTags, err
}

// SetWakeOnWifi sets properties related to wake on WiFi.
// DEPRECATED: Use tf.WifiClient().SetWakeOnWifi instead.
func (tf *TestFixture) SetWakeOnWifi(ctx context.Context, ops ...SetWakeOnWifiOption) (shortenCtx context.Context, cleanupFunc func() error, retErr error) {
	return tf.WifiClient().SetWakeOnWifi(ctx, ops...)
}

// WaitWifiConnected waits until WiFi is connected to the SHILL profile with specific GUID.
func (tf *TestFixture) WaitWifiConnected(ctx context.Context, dutIdx DutIdx, guid string) error {
	testing.ContextLogf(ctx, "Waiting for WiFi to be connected from DUT #%v to profile with GUID: %s", dutIdx, guid)

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		req := &wifi.RequestScansRequest{Count: 1}
		if _, err := tf.DUTWifiClient(dutIdx).RequestScans(ctx, req); err != nil {
			errors.Wrap(err, "failed to request scan")
		}

		serInfo, err := tf.DUTWifiClient(dutIdx).QueryService(ctx)
		if err != nil {
			return errors.Wrapf(err, "failed to get the WiFi service information from DUT #%v", dutIdx)
		}

		if guid == serInfo.Guid && serInfo.IsConnected {
			iface, err := tf.DUTClientInterface(ctx, dutIdx)
			if err != nil {
				return errors.Wrapf(err, "failed to get interface from the DUT #%v", dutIdx)
			}

			addrs, err := tf.DUTWifiClient(dutIdx).GetIPv4Addrs(ctx, &wifi.GetIPv4AddrsRequest{InterfaceName: iface})
			if err != nil {
				return errors.Wrapf(err, "failed to get client #%v IPv4 addresses", dutIdx)
			}
			if len(addrs.Ipv4) > 0 {
				return nil
			}
			return errors.New("IPv4 address not assigned yet")
		} else if guid != serInfo.Guid {
			return errors.New("GUID does not match, current service is: " + serInfo.Guid)
		}
		return errors.New("Service is not connected")
	}, &testing.PollOptions{
		Timeout:  time.Minute,
		Interval: time.Second,
	}); err != nil {
		return errors.Wrap(err, "no matching GUID service selected")
	}

	testing.ContextLog(ctx, "WiFi connected")
	return nil
}

// P2PDevice returns device data object respective to the device ID.
func (tf *TestFixture) P2PDevice(ctx context.Context, device P2PDevice) (P2PWiFiDevice, error) {
	switch device {
	case P2PDeviceDUT:
		return tf.duts[DefaultDUT], nil
	case P2PDeviceCompanionDUT:
		return tf.duts[PeerDUT1], nil
	}
	return nil, errors.Errorf("unexpected P2P device type: %d", device)
}

// P2PConfigureGO configures the DUT as a p2p group owner (GO).
func (tf *TestFixture) P2PConfigureGO(ctx context.Context, device P2PDevice, ops ...p2p.GroupOption) error {
	var err error
	if tf.p2pGO, err = tf.P2PDevice(ctx, device); err != nil {
		return err
	}

	wpar := remotewpacli.NewRemoteRunner(tf.p2pGO.Conn())

	// Make sure seeder BSSID is available in scan before asking to configure P2P.
	// This should set regdomain, which otherwise may cause test flakes.
	// Other tests are usually starting from scan anyway.
	if err := wpar.DiscoverNetwork(ctx, tf.seederSSID); err != nil {
		return err
	}

	err = tf.p2pGO.P2PGroupCreate(ctx, ops...)
	if err != nil {
		return err
	}
	return nil
}

// P2PDeconfigureGO deconfigures the p2p group owner (GO).
func (tf *TestFixture) P2PDeconfigureGO(ctx context.Context) (err error) {
	return tf.p2pGO.P2PGroupDelete(ctx)
}

// P2PConnect connects the p2p client to the p2p group owner (GO) network and waits for the service to be connected.
func (tf *TestFixture) P2PConnect(ctx context.Context, device P2PDevice, ops ...p2p.GroupOption) error {
	var err error
	if tf.p2pClient, err = tf.P2PDevice(ctx, device); err != nil {
		return err
	}

	err = tf.p2pClient.P2PGroupConnect(ctx, tf.p2pGO, ops...)
	if err != nil {
		return errors.Wrap(err, "failed to connect to P2P Group")
	}

	return nil
}

// P2PDisconnect disconnects and deconfigures the p2p client.
func (tf *TestFixture) P2PDisconnect(ctx context.Context) (err error) {
	return tf.p2pClient.P2PGroupDisconnect(ctx)
}

// AssertConnectionBetweenP2PDevices check connectivity (ping) between tow devices using their P2P interfaces.
func (tf *TestFixture) AssertConnectionBetweenP2PDevices(ctx context.Context, src, dest P2PWiFiDevice, srcIfType, dstIfType IfaceType, opts ...ping.Option) error {
	res, err := src.PingDevice(ctx, dest, srcIfType, dstIfType, opts...)
	if err != nil {
		return err
	}
	testing.ContextLogf(ctx, "ping statistics=%+v", res)
	if res.Loss > pingLossThreshold {
		return errors.Errorf("unexpected packet loss percentage: got %g%%, want <= %g%%", res.Loss, pingLossThreshold)
	}

	return nil
}

// P2PAssertPingFromGO pings the p2p client from the group owner (GO) device.
func (tf *TestFixture) P2PAssertPingFromGO(ctx context.Context, opts ...ping.Option) error {
	testing.ContextLog(ctx, "Ping p2p client from p2p group owner (GO)")
	return tf.AssertConnectionBetweenP2PDevices(ctx, tf.p2pGO, tf.p2pClient, P2PIfaceType, P2PIfaceType, opts...)
}

// P2PAssertPingFromClient pings the p2p group owner (GO) from the p2p client device.
func (tf *TestFixture) P2PAssertPingFromClient(ctx context.Context, opts ...ping.Option) error {
	testing.ContextLog(ctx, "Ping p2p group owner (GO) from p2p client")
	return tf.AssertConnectionBetweenP2PDevices(ctx, tf.p2pClient, tf.p2pGO, P2PIfaceType, P2PIfaceType, opts...)
}

// P2PPerf pings the p2p client from the group owner (GO) device.
func (tf *TestFixture) P2PPerf(ctx context.Context) (*iperf.Result, error) {
	// p2pGOFirewallParams is a set of parameters needed for unblocking p2p tcp traffic on the p2p GO.
	var p2pGOFirewallParams = []firewall.RuleOption{
		firewall.OptionWait(5),
		firewall.OptionAppendRule(firewall.InputChain),
		firewall.OptionSource(utils.P2PGOIPAddress),
		firewall.OptionProto(firewall.L4ProtoTCP),
		firewall.OptionMatch(firewall.L4ProtoTCP),
		firewall.OptionJumpTarget(firewall.TargetAccept),
	}

	// p2pClientFirewallParams is a set of parameters needed for unblocking p2p tcp traffic on the p2p client.
	var p2pClientFirewallParams = []firewall.RuleOption{
		firewall.OptionWait(5),
		firewall.OptionAppendRule(firewall.InputChain),
		firewall.OptionSource(utils.P2PClientIPAddress),
		firewall.OptionProto(firewall.L4ProtoTCP),
		firewall.OptionMatch(firewall.L4ProtoTCP),
		firewall.OptionJumpTarget(firewall.TargetAccept),
	}

	firewallRunnerGO := remotefirewall.NewRemoteRunner(tf.p2pGO.Conn())
	firewallRunnerClient := remotefirewall.NewRemoteRunner(tf.p2pClient.Conn())
	if err := firewallRunnerGO.ExecuteCommand(ctx, p2pGOFirewallParams...); err != nil {
		return nil, errors.Wrap(err, "failed to set P2P GO iptable rule")
	}
	if err := firewallRunnerClient.ExecuteCommand(ctx, p2pClientFirewallParams...); err != nil {
		return nil, errors.Wrap(err, "failed to set P2P Client iptable rule")
	}

	// Configuring the p2p GO as an iperf server and the p2p client as an iperf client.
	p2pIperfConfig, err := iperf.NewConfig(iperf.ProtocolTCP, utils.P2PClientIPAddress, utils.P2PGOIPAddress, []iperf.ConfigOption{}...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to configure iperf on the p2p link")
	}

	client, err := iperf.NewRemoteClient(ctx, tf.p2pClient.Conn())
	if err != nil {
		return nil, errors.Wrap(err, "failed ot create Iperf client")
	}
	defer client.Close(ctx)

	server, err := iperf.NewRemoteServer(ctx, tf.p2pGO.Conn())
	if err != nil {
		return nil, errors.Wrap(err, "failed ot create Iperf server")
	}
	defer server.Close(ctx)

	session := iperf.NewSession(client, server)

	finalResult, _, err := session.Run(ctx, p2pIperfConfig)
	if err != nil {
		return nil, errors.Wrap(err, "failed to run Iperf session")
	}

	return finalResult, nil
}

// SAPPerf verifies performance between AP and STA.
func (tf *TestFixture) SAPPerf(ctx context.Context, protocol iperf.Protocol, reverse bool, opts ...iperf.ConfigOption) (*iperf.Result, error) {
	addrsReq := &wifi.GetIPv4AddrsRequest{
		InterfaceName: shillconst.ApInterfaceName,
	}
	addrsRespAP, err := tf.DUTWifiClient(DefaultDUT).GetIPv4Addrs(ctx, addrsReq)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get the IPv4 addresses")
	}
	if len(addrsRespAP.Ipv4) == 0 {
		return nil, errors.New("no AP IP address returned")
	}
	apIP := strings.Split(addrsRespAP.Ipv4[0], "/")[0]
	ifResp, err := tf.DUTWifiClient(PeerDUT1).GetInterface(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get interface name")
	}
	addrsReq.InterfaceName = ifResp.GetName()
	addrsRespAP, err = tf.DUTWifiClient(PeerDUT1).GetIPv4Addrs(ctx, addrsReq)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get the IPv4 addresses")
	}
	if len(addrsRespAP.Ipv4) == 0 {
		return nil, errors.New("no STA IP address returned")
	}
	staIP := strings.Split(addrsRespAP.Ipv4[0], "/")[0]

	var perfConfig *iperf.Config
	var serverConn, clientConn *ssh.Conn
	if reverse {
		// Configuring AP as an iperf client and STA as an iperf server.
		serverConn = tf.DUT(PeerDUT1).Conn()
		clientConn = tf.DUT(DefaultDUT).Conn()
		perfConfig, err = iperf.NewConfig(protocol, apIP, staIP, opts...)
	} else {
		// Configuring AP as an iperf server and STA as an iperf client.
		serverConn = tf.DUT(DefaultDUT).Conn()
		clientConn = tf.DUT(PeerDUT1).Conn()
		perfConfig, err = iperf.NewConfig(protocol, staIP, apIP, opts...)
	}
	if err != nil {
		return nil, errors.Wrap(err, "failed to configure iperf")
	}

	client, err := iperf.NewRemoteClient(ctx, clientConn)
	if err != nil {
		return nil, errors.Wrap(err, "failed ot create Iperf client")
	}
	defer client.Close(ctx)
	ctx, cancel := ctxutil.Shorten(ctx, time.Second)
	defer cancel()

	server, err := iperf.NewRemoteServer(ctx, serverConn)
	if err != nil {
		return nil, errors.Wrap(err, "failed ot create Iperf server")
	}
	defer server.Close(ctx)
	ctx, cancel = ctxutil.Shorten(ctx, time.Second)
	defer cancel()

	session := iperf.NewSession(client, server)

	finalResult, _, err := session.Run(ctx, perfConfig)
	if err != nil {
		return nil, errors.Wrap(err, "failed to run Iperf session")
	}

	return finalResult, nil
}

// P2PGOIface returns the p2p GO interface name.
func (tf *TestFixture) P2PGOIface(ctx context.Context) (string, error) {
	return tf.p2pGO.IfName(ctx, P2PIfaceType)
}

// P2PGOConn returns connection object to the p2pGO.
func (tf *TestFixture) P2PGOConn() *ssh.Conn {
	return tf.p2pGO.Conn()
}

// ReserveForDeconfigP2P returns a shorter ctx and cancel function for tf.P2PDeconfigureGO() or tf.P2PDeconfigureClient().
func (tf *TestFixture) ReserveForDeconfigP2P(ctx context.Context) (context.Context, context.CancelFunc) {
	return ctxutil.Shorten(ctx, 12*time.Second)
}

// UseWpaCliAPI sets up test fixture to use wpa_cli API for extra configuration.
func (tf *TestFixture) UseWpaCliAPI(enable bool) {
	tf.useWpaCliAPI = enable
}

// SeedRegdomain sets up AP which broadcasts country information, so that all
// self-managed devices in the wificell get their regdomain seeded.
// It sets up 2 APs on routers[0] and 1 AP on pcap, routers[0] cannot be reused as
// the pcap, otherwise the AP on pcap reuses the one of the other 2 interfaces and fail.
func (tf *TestFixture) SeedRegdomain(ctx context.Context) error {
	startSeedingAP := func(ctx context.Context, r *routerData, channel int) error {
		// AP instance name for logging.
		name := tf.UniqueAPName()
		// Store the most recent seeder SSID.
		tf.seederSSID = hostapd.RandomSSID("SUPPORT_SSID_")
		ops := []hostapd.Option{
			hostapd.Mode(hostapd.Mode80211nPure), ap.HTCaps(ap.HTCapHT20),
			hostapd.Channel(channel), hostapd.SSID(tf.seederSSID),
			hostapd.SpectrumManagement()}
		config, err := hostapd.NewConfig(ops...)
		if err != nil {
			return err
		}
		ap, err := StartAPIface(ctx, r.object, name, "", false, false, false, config)
		if err != nil {
			return errors.Wrap(err, "failed to start APIface")
		}
		tf.aps[ap] = struct{}{}
		tf.seederIfaces = append(tf.seederIfaces, ap)
		return nil
	}
	if tf.duts[DefaultDUT].chipset == wlan.Intel7265 ||
		len(tf.duts) == 2 && tf.duts[PeerDUT1].chipset == wlan.Intel7265 {
		// AC7265 requires at least 3 APs to seed the regulatory domain, see b/313936485 for context.
		if err := startSeedingAP(ctx, tf.routers[0], 1); err != nil {
			return err
		}
		if err := startSeedingAP(ctx, tf.routers[0], 1); err != nil {
			return err
		}
	}
	if err := startSeedingAP(ctx, tf.pcap, 1); err != nil {
		return err
	}
	return nil
}

// DeconfigSeedingAP deconfigures AP broadcasting the regulatory domain.
func (tf *TestFixture) DeconfigSeedingAP(ctx context.Context) error {
	if tf.seederIfaces == nil {
		return nil
	}
	for _, seederIface := range tf.seederIfaces {
		if err := tf.DeconfigAP(ctx, seederIface); err != nil {
			return errors.Wrap(err, "failed to deconfig AP")
		}
	}
	tf.seederIfaces = nil
	tf.seederSSID = ""
	return nil
}

// StartTethering configures the specific DUT to provide a tethering session with the options specified.
func (tf *TestFixture) StartTethering(ctx context.Context, dutIdx DutIdx, ops []tethering.Option, fac security.ConfigFactory) (retCfg *tethering.Config, retResp *wifi.TetheringResponse, retErr error) {
	ctx, st := timing.Start(ctx, "tf.StartTethering")
	defer st.End()

	iwr := iw.NewRemoteRunner(tf.duts[dutIdx].dut.Conn())
	selfManaged, err := iwr.IsRegulatorySelfManaged(ctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to read regulatory status")
	}

	if !selfManaged {
		if err := iwr.SetAndVerifyRegulatoryDomain(ctx, "US"); err != nil {
			return nil, nil, errors.Wrap(err, "failed to set regulatory domain to WiFi")
		}
	}

	// Disable station mode to get rid of station interface scan
	ctxForEnablingWiFi := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Second)
	defer cancel()
	if err := tf.DUTWifiClient(dutIdx).SetWifiEnabled(ctx, false); err != nil {
		return nil, nil, errors.Wrap(err, "DUT: failed to disable the wifi")
	}
	defer func(ctx context.Context) {
		if retErr != nil {
			tf.DUTWifiClient(dutIdx).SetWifiEnabled(ctx, true)
		}
	}(ctxForEnablingWiFi)

	c, err := tethering.NewConfig(ops...)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create tethering config")
	}

	request := &wifi.TetheringRequest{
		NoUplink:          c.NoUL,
		AutoDisableMinute: c.AutoDisableMin,
		Ssid:              []byte(c.SSID),
		Band:              c.Band.String(),
		UseWpaCliApi:      tf.useWpaCliAPI,
		PriIface:          c.PriIface,
	}

	if fac != nil {
		// Defer the securityConfig generation from test's init() to here because the step may emit error and that's not allowed in test init().
		c.SecConf, err = fac.Gen()
		if err != nil {
			return nil, nil, err
		}
	}

	if c.SecConf.Class() == shillconst.SecurityClassPSK {
		request.Psk = c.PSK
		if c.SecMode == wpa.ModePureWPA2 {
			request.Security = shillconst.SecurityWPA2
		} else if c.SecMode == wpa.ModePureWPA3 {
			request.Security = shillconst.SecurityWPA3
		} else if c.SecMode == wpa.ModeMixedWPA3 {
			request.Security = shillconst.SecurityWPA2WPA3
		}
		wpaCfg := c.SecConf.(*wpa.Config)
		request.Psk = wpaCfg.PSK()
		request.Cipher = wpaCfg.Ciphers2()
	} else if c.SecConf.Class() == shillconst.SecurityClassNone {
		request.Security = shillconst.SecurityNone
	}

	resp, err := tf.duts[dutIdx].wifiClient.StartTethering(ctx, request)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "client failed to start tethering session with SSID %q", c.SSID)
	}
	defer func(ctx context.Context) {
		if retErr != nil {
			tf.StopTethering(ctx, dutIdx, c)
		}
	}(ctx)
	// The assumption is, that we don't need to return the new context as that would
	// be relevant only in case of error further in this function only
	// (precisely: in StartCapture()), so it won't be used anyway.
	ctx, cancel = ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if (c.Band == tethering.Band5g && resp.Channel <= 14) ||
		(c.Band == tethering.Band2p4g && resp.Channel > 14) {
		return nil, nil, errors.Errorf("AP Chanel/band mismatch, got %v, wanted band %s", resp.Channel, c.Band.String())
	}

	testing.ContextLogf(ctx, "Tethering started on channel %v, width: %v", resp.Channel, resp.ChannelWidth)

	var capturer *pcap.Capturer
	if tf.options.EnablePacketCapture {
		apOptions := []ap.Option{ap.Channel(int(resp.Channel))}
		// Pick the maximum available standard per band (assuming Gale capabilities).
		if resp.Channel <= 14 {
			apOptions = append(apOptions, ap.Mode(ap.Mode80211nMixed))
		} else {
			apOptions = append(apOptions, ap.Mode(ap.Mode80211acMixed))
		}
		switch resp.ChannelWidth {
		case 20:
			apOptions = append(apOptions, ap.HTCaps(ap.HTCapHT20))
		case 40:
			apOptions = append(apOptions, ap.HTCaps(ap.HTCapHT40))
		case 80:
			apOptions = append(apOptions, ap.HTCaps(ap.HTCapHT40), ap.VHTCaps(ap.VHTCapSGI80))
		case 160:
			apOptions = append(apOptions, ap.HTCaps(ap.HTCapHT40), ap.VHTCaps(ap.VHTCapVHT160))
		}
		config, err := hostapd.NewConfig(apOptions...)
		if err != nil {
			return nil, nil, err
		}
		freqOps, err := config.PcapFreqOptions()
		if err != nil {
			return nil, nil, err
		}
		capturer, err = tf.PcapRouter().StartCapture(ctx, tf.UniqueAPName(), config.Channel, config.OpClass, freqOps)
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to start capturer")
		}
		tf.tetheringCapturer = capturer
		defer func() {
			if retErr != nil {
				tf.PcapRouter().StopCapture(ctx, capturer)
			}
		}()
	}

	return c, resp, nil
}

// StopTethering attempts to stop the tethering session for the specified DUT.
func (tf *TestFixture) StopTethering(ctx context.Context, dutIdx DutIdx, c *tethering.Config) (*wifi.TetheringResponse, error) {
	ctx, st := timing.Start(ctx, "tf.StopTethering")
	defer st.End()
	resp, err := tf.duts[dutIdx].wifiClient.StopTethering(ctx, &wifi.StopTetheringRequest{UseWpaCliApi: tf.useWpaCliAPI, PriIface: c.PriIface})
	if tf.tetheringCapturer != nil {
		tf.PcapRouter().StopCapture(ctx, tf.tetheringCapturer)
	}
	if err != nil {
		return nil, errors.Wrap(err, "client failed to stop tethering session")
	}
	// Re-enable station mode
	if err := tf.DUTWifiClient(dutIdx).SetWifiEnabled(ctx, true); err != nil {
		return nil, errors.Wrap(err, "DUT: failed to enable the wifi")
	}
	return resp, nil
}

// ReserveForStopTethering returns a shorter ctx and cancel function for tf.StopTethering().
func (tf *TestFixture) ReserveForStopTethering(ctx context.Context) (context.Context, context.CancelFunc) {
	return ctxutil.Shorten(ctx, 15*time.Second)
}

// RebootDUT reboots DUT and re-establishes wifiClient for the given DUT.
// re-esablish wifiClient is required after rebooting due to the RPC client will be closed.
func (tf *TestFixture) RebootDUT(ctx context.Context, dutIdx DutIdx) (retErr error) {
	if err := tf.duts[dutIdx].dut.Reboot(ctx); err != nil {
		return errors.Wrap(err, "failed to reboot DUT")
	}

	var err error
	if tf.duts[dutIdx].rpc, err = rpc.Dial(ctx, tf.duts[dutIdx].dut, tf.duts[dutIdx].rpcHint); err != nil {
		return errors.Wrap(err, "failed to connect to the RPC service on the DUT")
	}
	tf.duts[dutIdx].wifiClient = &WifiClient{ShillServiceClient: wifi.NewShillServiceClient(tf.duts[dutIdx].rpc.Conn)}

	if tf.options.SetDutWifiLogging {
		if err := tf.setLoggingConfig(ctx, tf.duts[dutIdx].wifiClient, tf.options.DutWifiLogLevel, tf.options.DutWifiLogTags); err != nil {
			return err
		}
	}

	// Network configurations will be written in test profile when user isn't logged in, but it will not be reloaded if DUT restarts.
	// Call this function to make sure test profile is reloaded.
	_, err = tf.duts[dutIdx].wifiClient.EnsureTestProfileAvailable(ctx, &empty.Empty{})
	return err
}

// RemoveWiFiInterfaces removes all WiFi interfaces on the DUT using iw commands.
// It returns a context and function that can be used to restore the interfaces to
// their previous state.
func (tf *TestFixture) RemoveWiFiInterfaces(ctx context.Context, dutIdx DutIdx) (shortenCtx context.Context, restore func() error, err error) {
	iwr := iw.NewRemoteRunner(tf.DUTConn(dutIdx))
	ifaces, err := iwr.ListInterfaces(ctx)
	if err != nil {
		return nil, nil, err
	}
	restore = func() error {
		for _, iface := range ifaces {
			phy, err := iwr.PhyByID(ctx, iface.PhyNum)
			if err != nil {
				return err
			}
			if err := iwr.AddInterface(ctx, phy.Name, iface.IfName, iface.IfType, nil); err != nil {
				return err
			}
		}
		return nil
	}
	for _, iface := range ifaces {
		iwr.RemoveInterface(ctx, iface.IfName)
	}
	return ctx, restore, nil
}

// SetupAndConnectCellular setup the cellular client on dutIdx DUT and connect to the active cellular network.
func (tf *TestFixture) SetupAndConnectCellular(ctx context.Context, dutIdx DutIdx) (cleanup func(ctx context.Context), err error) {
	if _, err := tf.DUTCellularClient(dutIdx).SetUp(ctx, &emptypb.Empty{}); err != nil {
		return nil, errors.Wrap(err, "failed to setup cellular client")
	}
	testing.ContextLog(ctx, "Successfully initialized cellular device on DUT")
	cleanup = func(ctx context.Context) {
		tf.DUTCellularClient(dutIdx).TearDown(ctx, &emptypb.Empty{})
	}

	if _, err := tf.DUTCellularClient(dutIdx).Connect(ctx, &emptypb.Empty{}); err != nil {
		return cleanup, errors.Wrap(err, "failed to setup cellular client")
	}
	testing.ContextLog(ctx, "Successfully connected to the cellular network")
	return cleanup, nil
}

// ConnectCompanionDUTToHotspot retrieves the tethering config on hotspot host DUT,
// and uses a companion DUT to connect to the hotspot.
func (tf *TestFixture) ConnectCompanionDUTToHotspot(ctx context.Context, cdDutIdx, hsDutIdx DutIdx) (cleanup func(ctx context.Context), err error) {
	wifiClient := tf.DUTWifiClient(hsDutIdx)
	tetheringConfig, err := wifiClient.GetTetheringConfig(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get hotspot config")
	}
	testing.ContextLogf(ctx, "Using hotspot ssid: %s", tetheringConfig.Ssid)

	if tetheringConfig.Security != shillconst.SecurityWPA2 {
		return nil, errors.New("invalid tethering security type")
	}

	fac := wpa.NewConfigFactory(
		tetheringConfig.Passphrase, wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP),
	)
	secConf, err := fac.Gen()
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate security config")
	}

	_, err = tf.ConnectWifiFromDUT(ctx, cdDutIdx, tetheringConfig.Ssid, dutcfg.ConnSecurity(secConf))
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to the hotspot")
	}
	testing.ContextLog(ctx, "Downsteam DUT connected to hotspot successfully")
	return func(ctx context.Context) {
		if err := tf.DisconnectDUTFromWifi(ctx, cdDutIdx); err != nil {
			testing.ContextLogf(ctx, "Failed to disconnect from hotspot, err: %s", err)
		}
	}, nil
}

// errIsInvalidHost checks if the error is a wrapped "no such host" error.
func (tf *TestFixture) errIsInvalidHost(err error) bool {
	if err == utils.ErrCompanionHostname {
		return true
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return true
	}
	return false
}

// NumberOfDUTs returns number of DUTs handled by this fixture.
func (tf *TestFixture) NumberOfDUTs() int {
	return len(tf.duts)
}

// DUT returns particular DUT.
func (tf *TestFixture) DUT(dutIdx DutIdx) *dut.DUT {
	return tf.duts[dutIdx].dut
}

// DUTConn returns connection object to particular DUT.
func (tf *TestFixture) DUTConn(dutIdx DutIdx) *ssh.Conn {
	return tf.duts[dutIdx].dut.Conn()
}

// APConn returns connection object to the first AP.
// Currently, the test fixture only requires to control the first (0th) AP.
func (tf *TestFixture) APConn() *ssh.Conn {
	if len(tf.routers) == 0 {
		return nil
	}
	return tf.routers[DefaultRouter].host
}

// APConnByID returns connection object to the AP.
func (tf *TestFixture) APConnByID(idx RouterIdx) *ssh.Conn {
	if len(tf.routers) <= int(idx) {
		return nil
	}
	return tf.routers[idx].host
}

// RouterByID returns the respective router object in the fixture.
func (tf *TestFixture) RouterByID(idx RouterIdx) router.Base {
	return tf.routers[idx].object
}

// Router returns the router with id 0 in the fixture as the generic router.Base.
func (tf *TestFixture) Router() router.Base {
	return tf.RouterByID(DefaultRouter)
}

// StandardRouter returns the Router as a router.Standard.
func (tf *TestFixture) StandardRouter() (router.Standard, error) {
	r, ok := tf.Router().(router.Standard)
	if !ok {
		return nil, errors.New("router is not a standard router")
	}
	return r, nil
}

// StandardRouterWithFrameSenderSupport returns the Router as a router.StandardWithFrameSender.
func (tf *TestFixture) StandardRouterWithFrameSenderSupport() (router.StandardWithFrameSender, error) {
	r, ok := tf.Router().(router.StandardWithFrameSender)
	if !ok {
		return nil, errors.New("router is not a standard router with frame sender support")
	}
	return r, nil
}

// StandardRouterWithBridgeAndVethSupport returns the Router as a router.StandardWithBridgeAndVeth.
func (tf *TestFixture) StandardRouterWithBridgeAndVethSupport() (router.StandardWithBridgeAndVeth, error) {
	r, ok := tf.Router().(router.StandardWithBridgeAndVeth)
	if !ok {
		return nil, errors.New("router is not a standard router with bridge and veth support")
	}
	return r, nil
}

// PcapRouter returns the pcap router in the fixture as a support.Capture.
func (tf *TestFixture) PcapRouter() support.Capture {
	return tf.pcap.object.(support.Capture)
}

// StandardPcapRouter returns the PcapRouter as a router.Standard.
func (tf *TestFixture) StandardPcapRouter() (router.Standard, error) {
	r, ok := tf.PcapRouter().(router.Standard)
	if !ok {
		return nil, errors.New("pcap is not a standard router")
	}
	return r, nil
}

// Capturer returns the auto-spawned Capturer for the APIface instance only if APIface has only one Capturer.
func (tf *TestFixture) Capturer(ap *APIface) (*pcap.Capturer, bool) {
	capturers, ok := tf.capturers[ap]
	if !ok || len(capturers) > 1 {
		return nil, false
	}
	for _, capturer := range capturers {
		return capturer, true
	}
	return nil, true
}

// Capturers returns the auto-spawned Capturers for the APIface instance.
func (tf *TestFixture) Capturers(ap *APIface) (map[int]*pcap.Capturer, bool) {
	capturers, ok := tf.capturers[ap]
	return capturers, ok
}

// Attenuator returns the Attenuator object in the fixture.
func (tf *TestFixture) Attenuator() *attenuator.Attenuator {
	return tf.attenuator
}

// WifiClient is a backwards-compatible version of DUTWifiClient. Deprecated.
// TODO(b/234845693): remove after stabilizing period.
func (tf *TestFixture) WifiClient() *WifiClient {
	return tf.DUTWifiClient(DefaultDUT)
}

// DUTWifiClient returns the gRPC ShillServiceClient of the given DUT.
func (tf *TestFixture) DUTWifiClient(dutIdx DutIdx) *WifiClient {
	return tf.duts[dutIdx].wifiClient
}

// DUTCellularClient returns the gRPC ShillServiceClient of the given DUT.
func (tf *TestFixture) DUTCellularClient(dutIdx DutIdx) cellular.RemoteCellularServiceClient {
	return tf.duts[dutIdx].cellularClient
}

// RPC returns the gRPC connection of the default DUT. Deprecated.
// TODO(b/234845693): remove after stabilizing period.
func (tf *TestFixture) RPC() *rpc.Client {
	return tf.DUTRPC(DefaultDUT)
}

// DUTRPC returns the gRPC connection of the given DUT.
func (tf *TestFixture) DUTRPC(dutIdx DutIdx) *rpc.Client {
	return tf.duts[dutIdx].rpc
}

// DefaultOpenNetworkAPOptions returns the Options for an common 802.11n open wifi.
// The function is useful to allow common logic shared between the default setting
// and customized setting.
func DefaultOpenNetworkAPOptions() []hostapd.Option {
	return []hostapd.Option{
		hostapd.Mode(hostapd.Mode80211nPure),
		hostapd.Channel(48),
		hostapd.HTCaps(hostapd.HTCapHT20),
	}
}

// DefaultWPA3NetworkAPOptions returns the Options for an common 802.11n WPA3 wifi.
// The function is useful to allow common logic shared between the default setting
// and customized setting.
func DefaultWPA3NetworkAPOptions() []hostapd.Option {
	return []hostapd.Option{
		hostapd.Mode(hostapd.Mode80211nPure),
		hostapd.Channel(48),
		hostapd.HTCaps(hostapd.HTCapHT20),
		hostapd.PMF(hostapd.PMFOptional),
	}
}

// DefaultOpenNetworkAP configures the router to provide an 802.11n open wifi.
func (tf *TestFixture) DefaultOpenNetworkAP(ctx context.Context) (*APIface, error) {
	return tf.ConfigureAP(ctx, DefaultOpenNetworkAPOptions(), nil)
}

// DefaultOpenNetworkAPwithDNSHTTP configures the router to provide an 802.11n open wifi and
// enables DNS server and HTTP server on router.
func (tf *TestFixture) DefaultOpenNetworkAPwithDNSHTTP(ctx context.Context) (*APIface, error) {
	return tf.ConfigureAPOnRouterID(ctx, 0, DefaultOpenNetworkAPOptions(), nil, true, true)
}

// ClientInterface is a backwards-compatible version of DUTClientInterface. Deprecated.
func (tf *TestFixture) ClientInterface(ctx context.Context) (string, error) {
	return tf.DUTWifiClient(DefaultDUT).Interface(ctx)
}

// DUTClientInterface returns the client interface name of the given DUT.
func (tf *TestFixture) DUTClientInterface(ctx context.Context, dutIdx DutIdx) (string, error) {
	return tf.DUTWifiClient(dutIdx).Interface(ctx)
}

// InitializeRegdomainUS initializes the regulatory domain of the phy on the DUT to US.
func (tf *TestFixture) InitializeRegdomainUS(ctx context.Context) (string, error) {
	iwr := iw.NewRemoteRunner(tf.DUT(0).Conn())
	initialRegDomain, err := iwr.SinglePhyRegulatoryDomain(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to read the initial regulatory domain")
	}
	if initialRegDomain == "US" {
		testing.ContextLog(ctx, "The initial regulatory domain is already US, skip initializing the regulatory domain to US")
		return "US", nil
	}
	testing.ContextLog(ctx, "The initial regulatory domain is ", initialRegDomain)
	selfManaged, err := iwr.IsRegulatorySelfManaged(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to read regulatory status")
	}
	if !selfManaged {
		if err := iwr.SetAndVerifyRegulatoryDomain(ctx, "US"); err != nil {
			return "", errors.Wrap(err, "unable to set the non-self-managed regulatory domain to US")
		}
	} else {
		if err := tf.SeedRegdomain(ctx); err != nil {
			return "", errors.Wrap(err, "failed to set up regdomain seeding AP")
		}
		defer func(ctx context.Context) error {
			if err := tf.DeconfigSeedingAP(ctx); err != nil {
				return errors.Wrap(err, "failed to deconfig regdomain seeding AP")
			}
			return nil
		}(ctx)

		clientIface, err := tf.ClientInterface(ctx)
		if err != nil {
			return "", errors.Wrap(err, "failed to get the client interface")
		}
		if _, err := tf.WifiClient().ShillServiceClient.ScanAndFetchRegion(ctx, &wifi.ScanAndFetchRegionRequest{IfName: clientIface}); err != nil {
			return "", errors.Wrap(err, "unable to set the self-managed the regulatory domain to US")
		}
	}

	regdomain, err := iwr.SinglePhyRegulatoryDomain(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to read the regulatory regulatory domain")
	}
	testing.ContextLog(ctx, "The regulatory domain is initialized to ", regdomain)
	return initialRegDomain, nil
}

// ResetRegdomain resets the regulatory domain of the phy on the DUT to regDomain.
func (tf *TestFixture) ResetRegdomain(ctx context.Context, regDomain string) error {
	iwr := iw.NewRemoteRunner(tf.DUT(0).Conn())
	selfManaged, err := iwr.IsRegulatorySelfManaged(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to read the regulatory status")
	}
	if selfManaged {
		testing.ContextLog(ctx, "The regulatory domain is self-managed, no need to reset")
		return nil
	}
	if !(len(regDomain) == 2 && regDomain[0] >= 'A' && regDomain[0] <= 'Z' && regDomain[1] >= 'A' && regDomain[1] <= 'Z') {
		testing.ContextLogf(ctx, "The regulatory domain %s is not a valid alpha2 code, resetting the regulatory domain to 00", regDomain)
		regDomain = "00"
	}
	if err := iwr.SetAndVerifyRegulatoryDomain(ctx, regDomain); err != nil {
		return errors.Wrapf(err, "failed to reset the non-self-managed regulatory domain to %s", regDomain)
	}
	regdomain, err := iwr.SinglePhyRegulatoryDomain(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to read regulatory status")
	}
	testing.ContextLog(ctx, "The regulatory domain is reset to ", regdomain)
	return nil
}

// CheckFullAuthFlow check what authentication algorithm is used in pcap.
// An authentication flow is considered full when the total number of auth frames
// with valid sequences matches the expectation of successful authentication.
// Since a successful auth flow only uses one of authentication algorithm, it is
// an invalid flow if multiple auth algos found in the same pcap file.
func (tf *TestFixture) CheckFullAuthFlow(ctx context.Context, capturer *pcap.Capturer) (wpa.AuthAlgo, error) {
	pcapPath, err := capturer.PacketPath(ctx)
	if err != nil {
		return wpa.AuthAlgoInvalid, errors.Wrap(err, "failed to get path of packet file")
	}

	// Filtering authentication frame.
	filters := []pcap.Filter{
		pcap.RejectLowSignal(),
		pcap.RejectRetransmission(),
		pcap.Dot11FCSValid(),
		pcap.TypeFilter(layers.LayerTypeDot11MgmtAuthentication,
			func(layer gopacket.Layer) bool {
				// Check fixed parameter:
				//   contents[0:1]: Authentication Algorithm Number – 0 for Open System, 2 for FT, 3 for SAE
				//   contents[2:3]: Authentication Transaction Sequence Number
				//   contents[4:5]: Status Code - 0 for Successful or Reserved
				contents := layer.LayerContents()
				if len(contents) < 6 || (contents[0] != 0 && contents[0] != 2 && contents[0] != 3) || contents[1] != 0 || contents[4] != 0 {
					return false
				}
				return true
			},
		),
	}
	packets, err := pcap.ReadPackets(pcapPath, filters...)
	if err != nil {
		return wpa.AuthAlgoInvalid, errors.Wrap(err, "failed to read packets")
	}
	testing.ContextLog(ctx, "Total packets found: ", len(packets))

	var openAuthCount uint16 = 0
	var ftAuthCount uint16 = 0
	var saeAuthCount uint16 = 0

	for _, p := range packets {
		for _, l := range p.Layers() {
			auth, ok := l.(*layers.Dot11MgmtAuthentication)
			if !ok {
				continue
			}

			if auth.Sequence == 1 || auth.Sequence == 2 {
				// Open System:
				// Sequence == 1 for Open Authentication Request.
				// Sequence == 2 for Open Authentication Response.
				// Fast BSS Transition:
				// Sequence == 1 for FT Authentication Request.
				// Sequence == 2 for FT Authentication Response.
				// SAE:
				// Sequence == 1 for SAE Commit Request and Response.
				// Sequence == 2 for SAE Confirm Request and Response.
				switch auth.Algorithm {
				case 0:
					openAuthCount++
				case 2:
					ftAuthCount++
				case 3:
					saeAuthCount++
				}
			}
		}
	}

	// Only one of the auth algorithms.
	if openAuthCount > 0 && ftAuthCount == 0 && saeAuthCount == 0 {
		return wpa.AuthAlgoOpen, nil
	}
	if openAuthCount == 0 && ftAuthCount > 0 && saeAuthCount == 0 {
		return wpa.AuthAlgoFT, nil
	}
	if openAuthCount == 0 && ftAuthCount == 0 && saeAuthCount > 0 {
		return wpa.AuthAlgoSAE, nil
	}

	return wpa.AuthAlgoInvalid, nil
}

/*
	Some tests like RoamFT require a networking setup in which there
	is a special key exchange protocol that needs to occur between the
	APs prior to a successful roam. In order for this communication to
	work, we need to construct a specific interface architecture as
	shown below:
	             _________                       _________
	            |         |                     |         |
	            |   br0   |                     |   br1   |
	            |_________|                     |_________|
	           ____|   |____                   ____|   |____
	     _____|____     ____|____         ____|____     ____|_____
	    |          |   |         |       |         |   |          |
	    | managed0 |   |  veth0  | <---> |  veth1  |   | managed1 |
	    |__________|   |_________|       |_________|   |__________|

	The managed0 and managed1 interfaces cannot communicate with each
	other without a bridge. However, the same bridge cannot be used
	to bridge the two interfaces either (as soon as managed0 is bound
	to a bridge, hostapd would notice and would configure the same MAC
	address as managed0 onto the bridge, and send/recv the L2 packet
	with the bridge). Thus, we create a virtual ethernet interface with
	one peer on either bridge to allow the bridges to forward traffic
	between managed0 and managed1.
*/

// initializeBridgeAndVethOnRouter sets up bridges and veths on router.
func (tf *TestFixture) initializeBridgeAndVethOnRouter(ctx context.Context, rd *routerData) error {
	var err error
	router, ok := rd.object.(router.StandardWithBridgeAndVeth)
	if !ok {
		return errors.New("router is not a standard router with bridge and veth support")
	}

	bv := &bridgeVethData{br: make([]string, 2), veth: make([]string, 2)}
	bv.r = router

	bv.veth[0], bv.veth[1], err = bv.r.NewVethPair(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get a veth pair")
	}

	// Bind the two ends of the veth to the two bridges.
	for i := 0; i < 2; i++ {
		bv.br[i], err = bv.r.NewBridge(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get a bridge")
		}
		if err := bv.r.BindVethToBridge(ctx, bv.veth[i], bv.br[i]); err != nil {
			return errors.Wrapf(err, "failed to bind the veth %q to bridge %q", bv.veth[i], bv.br[i])
		}
	}

	rd.brveth = bv
	testing.ContextLogf(ctx, "Network environment setup is done: %s <= %s----%s => %s", bv.br[0], bv.veth[0], bv.veth[1], bv.br[1])
	return nil
}

// GetBridgesOnRouterID gets all of bridge names on router at idx.
func (tf *TestFixture) GetBridgesOnRouterID(idx RouterIdx) ([]string, error) {
	if len(tf.routers) <= int(idx) {
		return nil, errors.Errorf("router index (%d) out of range [0, %d)", idx, len(tf.routers))
	}

	if tf.routers[idx].brveth == nil {
		return nil, errors.New("no bridge set up on router")
	}
	return tf.routers[idx].brveth.br, nil
}
