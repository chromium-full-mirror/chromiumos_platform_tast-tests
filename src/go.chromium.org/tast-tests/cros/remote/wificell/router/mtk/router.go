// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mtk

import (
	"context"
	"net"

	"go.chromium.org/tast-tests/cros/common/utils"
	"go.chromium.org/tast-tests/cros/common/wifi/iw"
	"go.chromium.org/tast-tests/cros/remote/fileutils"
	"go.chromium.org/tast-tests/cros/remote/log"
	remoteIp "go.chromium.org/tast-tests/cros/remote/network/ip"
	remoteIw "go.chromium.org/tast-tests/cros/remote/wifi/iw"
	"go.chromium.org/tast-tests/cros/remote/wificell/pcap"
	"go.chromium.org/tast-tests/cros/remote/wificell/router/common"
	"go.chromium.org/tast-tests/cros/remote/wificell/router/common/support"
	"go.chromium.org/tast-tests/cros/remote/wificell/router/mtk/wireless"
	"go.chromium.org/tast-tests/cros/remote/wificell/router/openwrt/uci"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/timing"
)

const (
	readyStatusFile = "/tmp/cros/status/ready"
	buildInfoFile   = "/etc/board.json"
)

var (
	buildInfoDeviceName string
)

// Router controls an MTK router and stores the router state.
type Router struct {
	host             *ssh.Conn
	name             string
	routerType       support.RouterType
	routerModel      string
	syslogdCollector *log.LogreadCollector
	iwr              *remoteIw.Runner
	ipr              *remoteIp.Runner
	uci              *uci.Runner
	activeServices   activeServices
	workDirPath      string
}

// activeServices keeps a record of what services have been started and not yet
// stopped manually so that they can be stopped during Router.Close.
type activeServices struct {
	capture    []*pcap.Capturer
	rawCapture []*pcap.Capturer
}

// NewRouter prepares initial test AP state (e.g., initializing wiphy/wdev).
// ctx is the deadline for the step and daemonCtx is the lifetime for background
// daemons.
func NewRouter(ctx, daemonCtx context.Context, host *ssh.Conn, name string) (router *Router, err error) {
	ctx, st := timing.Start(ctx, "initialize")
	defer st.End()

	testing.ContextLogf(ctx, "Creating new MTK router controller for router %q", name)
	r := &Router{
		host:           host,
		name:           name,
		routerType:     support.MtkOpenWrtT,
		routerModel:    createUniqueRouterModel(),
		iwr:            remoteIw.NewRemoteRunner(host),
		ipr:            remoteIp.NewRemoteRunner(host),
		uci:            uci.NewRemoteRunner(host),
		activeServices: activeServices{},
		workDirPath:    common.BuildWorkingDirPath(),
	}

	// Always call Close on init failure and ensure there is enough time to do so.
	fullCtx := ctx
	defer (func() {
		if err == nil {
			return
		}
		testing.ContextLog(fullCtx, "Router initialization failed, closing before returning error")
		if closeErr := r.Close(fullCtx); closeErr != nil {
			testing.ContextLogf(fullCtx, "Failed to close after initialization error %v due to %v", err, closeErr)
		}
	})()
	ctx, cancel := ctxutil.Shorten(ctx, common.RouterCloseContextDuration)
	defer cancel()

	// Start collecting system logs and save logs already in the buffer to a file.
	// The daemonCtx is used for the log collector as it should live longer than
	// the current stage when we are in precondition.
	testing.ContextLog(ctx, "Starting router log collection")
	if r.syslogdCollector, err = log.StartLogreadCollector(daemonCtx, host); err != nil {
		return nil, errors.Wrap(err, "failed to start syslogd log collector")
	}
	if err := common.CollectRouterLogs(daemonCtx, r, r.syslogdCollector, "pre_setup"); err != nil {
		return nil, errors.Wrap(err, "failed to collect syslogd logs before setup actions")
	}

	// Set up working dir.
	testing.ContextLog(ctx, "Preparing working dir on router")
	if err := r.host.CommandContext(ctx, "rm", "-rf", r.workDir()).Run(); err != nil {
		return nil, errors.Wrapf(err, "failed to remove workdir %q", r.workDir())
	}
	if err := r.host.CommandContext(ctx, "mkdir", "-p", r.workDir()).Run(); err != nil {
		return nil, errors.Wrapf(err, "failed to create workdir %q", r.workDir())
	}
	if err := uci.ResetWifi(ctx, r.uci); err != nil {
		return nil, errors.Wrap(err, "failed to reset WiFi config to default config")
	}
	testing.ContextLog(ctx, "Stopping WiFi and DHCP services")
	if err := r.stopWiFiDHCP(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to stop WiFi and DHCP")
	}

	testing.ContextLog(ctx, "Setting regulatory domain to US")
	if err := r.iwr.SetRegulatoryDomain(ctx, "US"); err != nil {
		return nil, errors.Wrap(err, "failed to set regulatory domain to US")
	}

	// Save logs collected from setup actions.
	testing.ContextLog(ctx, "Collecting logs from setup")
	if err := common.CollectRouterLogs(daemonCtx, r, r.syslogdCollector, "post_setup"); err != nil {
		return nil, errors.Wrap(err, "failed to collect syslogd logs after setup actions")
	}

	testing.ContextLogf(ctx, "Created new MTK router controller for router %q", r.name)
	return r, nil
}

// Close cleans the resource used by Router.
func (r *Router) Close(ctx context.Context) error {
	ctx, st := timing.Start(ctx, "router.Close")
	defer st.End()

	testing.ContextLogf(ctx, "Closing MTK router controller for router %q", r.name)

	var firstErr error

	// Collect closing log to facilitate debugging.
	if r.syslogdCollector != nil {
		if err := common.CollectRouterLogs(ctx, r, r.syslogdCollector, "pre_close"); err != nil {
			utils.CollectFirstErr(ctx, &firstErr, errors.Wrap(err, "failed to collect syslogd logs before close actions"))
		}
	}

	// Clean working dir.
	if err := r.host.CommandContext(ctx, "rm", "-rf", r.workDir()).Run(); err != nil {
		utils.CollectFirstErr(ctx, &firstErr, errors.Wrap(err, "failed to remove working dir"))
	}

	// Collect closing log to facilitate debugging.
	if r.syslogdCollector != nil {
		if err := common.CollectRouterLogs(ctx, r, r.syslogdCollector, "post_close"); err != nil {
			utils.CollectFirstErr(ctx, &firstErr, errors.Wrap(err, "failed to collect syslogd logs after close actions"))
		}
		if err := r.syslogdCollector.Close(); err != nil {
			utils.CollectFirstErr(ctx, &firstErr, errors.Wrap(err, "failed to stop syslogd log collector"))
		}
	}

	if err := r.stopWiFiDHCP(ctx); err != nil {
		utils.CollectFirstErr(ctx, &firstErr, errors.Wrap(err, "failed to stop WiFi and DHCP"))
	}

	testing.ContextLogf(ctx, "Closed MTK router controller for router %q", r.name)
	return firstErr
}

// RouterName returns the name of the managed router device.
func (r *Router) RouterName() string {
	return r.name
}

// RouterType returns the router type.
func (r *Router) RouterType() support.RouterType {
	return r.routerType
}

// RouterModel returns the router's model.
func (r *Router) RouterModel() string {
	return r.routerModel
}

// StartReboot initiates a reboot of the router host.
//
// Close must be called prior to StartReboot, not after.
//
// This Router instance will be unable to interact with the host after calling
// this, as the connection to the host will be severed. To use this host
// again, create a new Router instance with a new connection after the host is
// fully rebooted.
func (r *Router) StartReboot(ctx context.Context) error {
	_ = r.host.CommandContext(ctx, "reboot").Run()
	return nil
}

// createUniqueRouterModel creates the router model name by combining the router Type and the device name from the build info.
func createUniqueRouterModel() string {
	rModel := "mt7988"
	return rModel
}

// workDir returns the directory to place temporary files on router.
func (r *Router) workDir() string {
	return r.workDirPath
}

// CollectLogs dumps collected syslogd logs to a file.
func (r *Router) CollectLogs(ctx context.Context) error {
	return common.CollectRouterLogs(ctx, r, r.syslogdCollector, "")
}

// stopWiFiDHCP forcibly stops any WiFi and dhcp processes.
func (r *Router) stopWiFiDHCP(ctx context.Context) error {
	shortCtx, st := timing.Start(ctx, "stopWiFiDHCP")
	defer st.End()
	var firstErr error
	if err := r.host.CommandContext(shortCtx, "/etc/init.d/dnsmasq", "stop").Run(); err != nil {
		utils.CollectFirstErr(ctx, &firstErr, errors.Wrap(err, "failed to stop dnsmasq service which manages core OpenWrt DHCP servers"))
	}
	if err := uci.StopWifi(shortCtx, r.uci); err != nil {
		utils.CollectFirstErr(ctx, &firstErr, errors.Wrap(err, "failed to stop WiFi which manages core OpenWrt WiFi processes"))
	}
	return nil
}

// StartWireless Start server
func (r *Router) StartWireless(ctx context.Context, name string, configs *wireless.Config) (_ *wireless.Server, retErr error) {
	ws, err := wireless.StartServer(ctx, r.host, r.uci, name, configs)
	if err != nil {
		return nil, err
	}
	if err := uci.StartDnsmasq(ctx, r.uci); err != nil {
		return nil, err
	}
	return ws, nil
}

// StopWireless stops the wireless server.
func (r *Router) StopWireless(ctx context.Context, ws *wireless.Server) error {
	return uci.StopWifi(ctx, r.uci)
}

// StartCapture starts a packet capturer.
func (r *Router) StartCapture(ctx context.Context, name string, ch int, is6GHz bool, freqOps []iw.SetFreqOption, pcapOps ...pcap.Option) (_ *pcap.Capturer, retErr error) {
	return nil, nil
}

// StopCapture stops the packet capturer and releases related resources.
func (r *Router) StopCapture(ctx context.Context, capturer *pcap.Capturer) error {
	return nil
}

// StartRawCapturer starts a capturer on an existing interface on the router
// instead of a monitor type interface.
func (r *Router) StartRawCapturer(ctx context.Context, name, iface string, ops ...pcap.Option) (*pcap.Capturer, error) {
	return nil, nil
}

// StopRawCapturer stops the packet capturer (no extra resources to release).
func (r *Router) StopRawCapturer(ctx context.Context, capturer *pcap.Capturer) (retErr error) {
	return nil
}

// ReserveForStopCapture returns a shortened ctx with cancel function.
func (r *Router) ReserveForStopCapture(ctx context.Context, capturer *pcap.Capturer) (context.Context, context.CancelFunc) {
	return capturer.ReserveForClose(ctx)
}

// ReserveForStopRawCapturer returns a shortened ctx with cancel function.
// The shortened ctx is used for running things before r.StopRawCapture to
// reserve time for it.
func (r *Router) ReserveForStopRawCapturer(ctx context.Context, capturer *pcap.Capturer) (context.Context, context.CancelFunc) {
	return capturer.ReserveForClose(ctx)
}

// MAC returns the MAC address of iface on this router.
func (r *Router) MAC(ctx context.Context, iface string) (net.HardwareAddr, error) {
	return r.ipr.MAC(ctx, iface)
}

// HostIsMtkRouter determines whether the remote host is an MTK router.
func HostIsMtkRouter(ctx context.Context, host *ssh.Conn) (bool, error) {
	deviceInfoPath := "/etc/openwrt_release"
	deviceInfoMatchIfOpenWrt := "(?m)^DISTRIB_TARGET='mediatek/mt7988'$"
	matches, err := fileutils.HostFileContentsMatch(ctx, host, deviceInfoPath, deviceInfoMatchIfOpenWrt)
	if err != nil {
		return false, errors.Wrapf(err, "failed to check if remote file %q contents match %q", deviceInfoPath, deviceInfoMatchIfOpenWrt)
	}
	return matches, nil
}

// iface finds a suitable wifi iface for the given channel, opclass.
// The selected iface name is returned.
func (r *Router) iface(freq int) string {
	if freq < 5000 {
		return wireless.WiFiIface2G
	} else if freq > 5900 {
		return wireless.WiFiIface6G
	} else {
		return wireless.WiFiIface5G
	}
}

// phy finds a suitable wifi phy for the given channel, opclass.
// The selected phy name is returned.
func (r *Router) phy(freq int) string {
	if freq < 5000 {
		return wireless.WiFiPhy2G
	} else if freq > 5900 {
		return wireless.WiFiPhy5G
	} else {
		return wireless.WiFiPhy6G
	}
}
