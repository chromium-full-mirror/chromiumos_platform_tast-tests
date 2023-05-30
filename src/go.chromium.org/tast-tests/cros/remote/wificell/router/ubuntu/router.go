// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ubuntu

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/network/ip"
	"go.chromium.org/tast-tests/cros/common/network/iw"
	"go.chromium.org/tast-tests/cros/common/utils"
	"go.chromium.org/tast-tests/cros/remote/log"
	remote_ip "go.chromium.org/tast-tests/cros/remote/network/ip"
	remote_iw "go.chromium.org/tast-tests/cros/remote/network/iw"
	"go.chromium.org/tast-tests/cros/remote/wificell/dhcp"
	"go.chromium.org/tast-tests/cros/remote/wificell/framesender"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/remote/wificell/http"
	"go.chromium.org/tast-tests/cros/remote/wificell/pcap"
	"go.chromium.org/tast-tests/cros/remote/wificell/router/common"
	"go.chromium.org/tast-tests/cros/remote/wificell/router/common/support"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/timing"
)

// Used hostapd environment variable keys.
const (
	envKeyOpenSslConf                            = "OPENSSL_CONF"
	envKeyOpenSslChromiumSkipTrustedPurposeCheck = "OPENSSL_CHROMIUM_SKIP_TRUSTED_PURPOSE_CHECK"
)

const lsbReleasePath = "/etc/lsb-release"

// logsToCollect is the list of files on router to collect.
var logsToCollect = []string{
	"/var/log/messages",
}

// Router is used to control the Ubuntu wireless router and stores state of the router.
type Router struct {
	host          *ssh.Conn
	name          string
	routerType    support.RouterType
	board         string
	phys          map[int]*iw.Phy // map from phy idx to iw.Phy.
	im            *common.IfaceManager
	nextBridgeID  int
	nextVethID    int
	iwr           *iw.Runner
	ipr           *ip.Runner
	logCollectors map[string]*log.TailCollector // map from log path to its collector.
}

// NewRouter prepares initial test AP state (e.g., initializing wiphy/wdev).
// ctx is the deadline for the step and daemonCtx is the lifetime for background
// daemons.
func NewRouter(ctx, daemonCtx context.Context, host *ssh.Conn, name string) (*Router, error) {
	r := &Router{
		host:          host,
		name:          name,
		routerType:    support.UbuntuT,
		phys:          make(map[int]*iw.Phy),
		iwr:           remote_iw.NewRemoteRunner(host),
		ipr:           remote_ip.NewRemoteRunner(host),
		logCollectors: make(map[string]*log.TailCollector),
	}
	r.im = common.NewRouterIfaceManager(r, r.iwr)

	shortCtx, cancel := ctxutil.Shorten(ctx, common.RouterCloseContextDuration)
	defer cancel()

	ctx, st := timing.Start(shortCtx, "initialize")
	defer st.End()

	// Clean up Autotest working dir, in case we're out of space.
	// NB: we need 'sh' to handle the glob.
	if err := r.host.CommandContext(shortCtx, "sh", "-c", strings.Join([]string{"rm", "-rf", common.AutotestWorkdirGlob}, " ")).Run(); err != nil {
		r.Close(shortCtx)
		return nil, errors.Wrapf(err, "failed to remove workdir %q", common.AutotestWorkdirGlob)
	}

	// Set up working dir.
	if err := r.host.CommandContext(shortCtx, "rm", "-rf", r.workDir()).Run(); err != nil {
		r.Close(shortCtx)
		return nil, errors.Wrapf(err, "failed to remove workdir %q", r.workDir())
	}
	if err := r.host.CommandContext(shortCtx, "mkdir", "-p", r.workDir()).Run(); err != nil {
		r.Close(shortCtx)
		return nil, errors.Wrapf(err, "failed to create workdir %q", r.workDir())
	}

	var err error
	// Start log collectors with daemonCtx as it should live longer than current
	// stage when we are in precondition.
	if r.logCollectors, err = common.StartTailLogCollectors(daemonCtx, r.host, logsToCollect, true); err != nil {
		r.Close(shortCtx)
		return nil, errors.Wrap(err, "failed to start loggers")
	}

	if err := r.setupWifiPhys(shortCtx); err != nil {
		r.Close(shortCtx)
		return nil, err
	}
	if err := common.RemoveAllBridgeIfaces(shortCtx, r.ipr); err != nil {
		r.Close(shortCtx)
		return nil, err
	}
	if err := common.RemoveAllVethIfaces(shortCtx, r.ipr); err != nil {
		r.Close(shortCtx)
		return nil, err
	}

	killHostapdDhcp := func() {
		shortCtx, st := timing.Start(shortCtx, "killHostapdDhcp")
		defer st.End()

		// Kill remaining hostapd/dnsmasq.
		hostapd.KillAll(shortCtx, r.host)
		dhcp.KillAll(shortCtx, r.host)
	}
	killHostapdDhcp()

	if err := r.iwr.SetRegulatoryDomain(shortCtx, "US"); err != nil {
		r.Close(shortCtx)
		return nil, errors.Wrap(err, "failed to set regulatory domain to US")
	}

	testing.ContextLogf(ctx, "Created new UbuntuT router controller for router %q", r.name)
	return r, nil
}

// RouterType returns the router's type
func (r *Router) RouterType() support.RouterType {
	return r.routerType
}

// RouterName returns the name of the managed router device.
func (r *Router) RouterName() string {
	return r.name
}

// StartReboot initiates a reboot of the router host.
func (r *Router) StartReboot(ctx context.Context) error {
	r.host.CommandContext(ctx, "sudo", "reboot", "now").Run()
	return nil
}

// setupWifiPhys fills r.phys and enables their antennas.
func (r *Router) setupWifiPhys(ctx context.Context) error {
	ctx, st := timing.Start(ctx, "setupWifiPhys")
	defer st.End()

	if err := r.im.RemoveAll(ctx); err != nil {
		return err
	}
	phys, _, err := r.iwr.ListPhys(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to list phys")
	}
	if len(phys) == 0 {
		return errors.New("Expect at least one wireless phy; found nothing")
	}
	for _, p := range phys {
		phyIDBytes, err := r.host.CommandContext(ctx, "cat", fmt.Sprintf("/sys/class/ieee80211/%s/index", p.Name)).Output()
		if err != nil {
			return errors.Wrapf(err, "failed to get phy idx for %s", p.Name)
		}
		phyID, err := strconv.Atoi(strings.TrimSpace(string(phyIDBytes)))
		if err != nil {
			return errors.Wrapf(err, "invalid phy idx %s", string(phyIDBytes))
		}
		r.phys[phyID] = p
	}
	return nil
}

// Close cleans the resource used by Router.
func (r *Router) Close(ctx context.Context) error {
	ctx, st := timing.Start(ctx, "router.Close")
	defer st.End()

	var firstErr error

	// Remove the interfaces that we created.
	for _, nd := range r.im.Available {
		if err := r.im.Remove(ctx, nd.IfName); err != nil {
			utils.CollectFirstErr(ctx, &firstErr, errors.Wrap(err, "failed to remove interfaces"))
		}
	}
	for _, nd := range r.im.Busy {
		testing.ContextLogf(ctx, "iface %s not yet freed", nd.IfName)
		if err := r.im.Remove(ctx, nd.IfName); err != nil {
			utils.CollectFirstErr(ctx, &firstErr, errors.Wrap(err, "failed to remove interfaces"))
		}
	}

	if err := common.RemoveAllBridgeIfaces(ctx, r.ipr); err != nil {
		utils.CollectFirstErr(ctx, &firstErr, err)
	}
	if err := common.RemoveAllVethIfaces(ctx, r.ipr); err != nil {
		utils.CollectFirstErr(ctx, &firstErr, err)
	}

	// Collect closing log to facilitate debugging for error occurs in
	// r.initialize() or after r.CollectRouterFileLogs().
	if err := common.CollectRouterFileLogs(ctx, r, r.logCollectors, logsToCollect, ".close"); err != nil {
		utils.CollectFirstErr(ctx, &firstErr, errors.Wrap(err, "failed to collect logs"))
	}
	if err := common.StopTailLogCollectors(ctx, r.logCollectors); err != nil {
		utils.CollectFirstErr(ctx, &firstErr, errors.Wrap(err, "failed to stop loggers"))
	}
	if err := r.host.CommandContext(ctx, "rm", "-rf", r.workDir()).Run(); err != nil {
		utils.CollectFirstErr(ctx, &firstErr, errors.Wrap(err, "failed to remove working dir"))
	}
	return firstErr
}

// phy finds an suitable phy for the given channel and target interface type t.
// The selected phy index is returned.
func (r *Router) phy(ctx context.Context, channel int, t iw.IfType) (int, error) {
	freq, err := hostapd.ChannelToFrequency(channel)
	if err != nil {
		return 0, errors.Errorf("channel %d not available", channel)
	}
	// Try to find an idle phy which is suitable
	for id, phy := range r.phys {
		if r.im.IsPhyBusy(id, t) {
			continue
		}
		if phySupportsFrequency(phy, freq) {
			return id, nil
		}
	}
	// Try to find any phy which is suitable, even a busy one
	for id, phy := range r.phys {
		if phySupportsFrequency(phy, freq) {
			return id, nil
		}
	}
	return 0, errors.Errorf("cannot find supported phy for channel=%d", channel)
}

// phySupportsFrequency returns true if any band of the given phy supports
// the desired frequency.
func phySupportsFrequency(phy *iw.Phy, freq int) bool {
	for _, b := range phy.Bands {
		if _, ok := b.FrequencyFlags[freq]; ok {
			return true
		}
	}
	return false
}

// netDev finds an available interface suitable for the given channel and type.
func (r *Router) netDev(ctx context.Context, channel int, t iw.IfType) (*iw.NetDev, error) {
	ctx, st := timing.Start(ctx, "netDev")
	defer st.End()

	phyID, err := r.phy(ctx, channel, t)
	if err != nil {
		return nil, err
	}
	return r.netDevWithPhyID(ctx, phyID, t)
}

// netDevWithPhyID finds an available interface on phy#phyID and with given type.
func (r *Router) netDevWithPhyID(ctx context.Context, phyID int, t iw.IfType) (*iw.NetDev, error) {
	// First check if there's an available interface on target phy.
	for _, nd := range r.im.Available {
		if nd.PhyNum == phyID && nd.IfType == t {
			return nd, nil
		}
	}
	// No available interface on phy, create one.
	return r.createWifiIface(ctx, phyID, t)
}

// monitorOnInterface finds an available monitor type interface on the same phy as a
// busy interface with name=iface.
func (r *Router) monitorOnInterface(ctx context.Context, iface string) (*iw.NetDev, error) {
	var ndev *iw.NetDev
	// Find phy ID of iface.
	for name, nd := range r.im.Busy {
		if name == iface {
			ndev = nd
			break
		}
	}
	if ndev == nil {
		return nil, errors.Errorf("cannot find busy interface %s", iface)
	}
	phyID := ndev.PhyNum
	return r.netDevWithPhyID(ctx, phyID, iw.IfTypeMonitor)
}

// StartHostapd starts the hostapd server.
func (r *Router) StartHostapd(ctx context.Context, name string, conf *hostapd.Config) (_ *hostapd.Server, retErr error) {
	ctx, st := timing.Start(ctx, "router.StartHostapd")
	defer st.End()

	if err := conf.SecurityConfig.InstallRouterCredentials(ctx, r.host, r.workDir()); err != nil {
		return nil, errors.Wrap(err, "failed to install router credentials")
	}

	nd, err := r.netDev(ctx, conf.Channel, iw.IfTypeManaged)
	if err != nil {
		return nil, err
	}
	iface := nd.IfName
	r.im.SetBusy(iface)
	defer func() {
		if retErr != nil {
			r.im.SetAvailable(iface)
		}
	}()
	return r.startHostapdOnIface(ctx, iface, name, conf)
}

func (r *Router) startHostapdOnIface(ctx context.Context, iface, name string, conf *hostapd.Config) (_ *hostapd.Server, retErr error) {
	ctx, st := timing.Start(ctx, "router.startHostapdOnIface")
	defer st.End()

	// TODO(crbug.com/1047146): Remove this env addition part after we drop the old crypto like MD5.
	if _, isSet := conf.EnvironmentVars[envKeyOpenSslConf]; !isSet {
		conf.EnvironmentVars[envKeyOpenSslConf] = "/etc/ssl/openssl.cnf.compat"
	}
	if _, isSet := conf.EnvironmentVars[envKeyOpenSslChromiumSkipTrustedPurposeCheck]; !isSet {
		conf.EnvironmentVars[envKeyOpenSslChromiumSkipTrustedPurposeCheck] = "1"
	}

	hs, err := hostapd.StartServer(ctx, r.host, name, iface, r.workDir(), conf)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start hostapd server")
	}
	defer func(ctx context.Context) {
		if retErr != nil {
			if err := hs.Close(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to stop hostapd server while StartHostapd has failed: ", err)
			}
		}
	}(ctx)
	ctx, cancel := hs.ReserveForClose(ctx)
	defer cancel()

	if err := r.iwr.SetTxPowerAuto(ctx, iface); err != nil {
		return nil, errors.Wrap(err, "failed to set txpower to auto")
	}
	return hs, nil
}

// StopHostapd stops the hostapd server.
func (r *Router) StopHostapd(ctx context.Context, hs *hostapd.Server) error {
	var firstErr error
	iface := hs.Interface()
	if err := hs.Close(ctx); err != nil {
		utils.CollectFirstErr(ctx, &firstErr, errors.Wrap(err, "failed to stop hostapd"))
	}
	utils.CollectFirstErr(ctx, &firstErr, r.ipr.SetLinkDown(ctx, iface))
	r.im.SetAvailable(iface)
	return firstErr
}

// ReconfigureHostapd restarts the hostapd server with the new config. It preserves the interface and the name of the old hostapd server.
func (r *Router) ReconfigureHostapd(ctx context.Context, hs *hostapd.Server, conf *hostapd.Config) (_ *hostapd.Server, retErr error) {
	iface := hs.Interface()
	name := hs.Name()
	if err := r.StopHostapd(ctx, hs); err != nil {
		return nil, errors.Wrap(err, "failed to stop hostapd server")
	}
	r.im.SetBusy(iface)
	defer func() {
		if retErr != nil {
			r.im.SetAvailable(iface)
		}
	}()
	return r.startHostapdOnIface(ctx, iface, name, conf)
}

// StartDHCP starts the DHCP server and configures the server IP. If DNS functionality is
// not required, set dnsOpt to nil.
func (r *Router) StartDHCP(ctx context.Context, name, iface string, ipStart, ipEnd, serverIP, broadcastIP net.IP, mask net.IPMask, dnsOpt *dhcp.DNSOption) (_ *dhcp.Server, retErr error) {
	ctx, st := timing.Start(ctx, "router.StartDHCP")
	defer st.End()

	if err := r.ipr.FlushIP(ctx, iface); err != nil {
		return nil, err
	}
	maskLen, _ := mask.Size()
	if err := r.ipr.AddIP(ctx, iface, serverIP, maskLen, ip.AddIPBroadcast(broadcastIP)); err != nil {
		return nil, err
	}
	defer func(ctx context.Context) {
		if retErr != nil {
			if err := r.ipr.FlushIP(ctx, iface); err != nil {
				testing.ContextLogf(ctx, "Failed to flush the interface %s while StartDHCP has failed: %v", iface, err)
			}
		}
	}(ctx)
	ctx, cancel := ctxutil.Shorten(ctx, time.Second)
	defer cancel()
	ds, err := dhcp.StartServer(ctx, r.host, name, iface, r.workDir(), ipStart, ipEnd, dnsOpt)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start DHCP server")
	}
	return ds, nil
}

// StopDHCP stops the DHCP server and flushes the interface.
func (r *Router) StopDHCP(ctx context.Context, ds *dhcp.Server) error {
	var firstErr error
	iface := ds.Interface()
	if err := ds.Close(ctx); err != nil {
		utils.CollectFirstErr(ctx, &firstErr, errors.Wrap(err, "failed to stop dhcpd"))
	}
	utils.CollectFirstErr(ctx, &firstErr, r.ipr.FlushIP(ctx, iface))
	return firstErr
}

// StartHTTP starts the HTTP server.
func (r *Router) StartHTTP(ctx context.Context, name, iface, redirectAddr string, port, statusCode int) (_ *http.Server, retErr error) {
	httpServer, err := http.StartServer(ctx, r.host, name, iface, r.workDir(), redirectAddr, port, statusCode)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start HTTP server")
	}
	return httpServer, nil
}

// StopHTTP stops the HTTP server.
func (r *Router) StopHTTP(ctx context.Context, httpServer *http.Server) error {
	var firstErr error
	if err := httpServer.Close(ctx); err != nil {
		utils.CollectFirstErr(ctx, &firstErr, errors.Wrap(err, "failed to stop HTTP server"))
	}
	return firstErr
}

// StartCapture starts a packet capturer.
func (r *Router) StartCapture(ctx context.Context, name string, ch int, freqOps []iw.SetFreqOption, pcapOps ...pcap.Option) (ret *pcap.Capturer, retErr error) {
	return nil, nil
}

// ReserveForStopCapture returns a shortened ctx with cancel function.
// The shortened ctx is used for running things before r.StopCapture() to reserve time for it to run.
func (r *Router) ReserveForStopCapture(ctx context.Context, capturer *pcap.Capturer) (context.Context, context.CancelFunc) {
	return ctx, nil
}

// StopCapture stops the packet capturer and releases related resources.
func (r *Router) StopCapture(ctx context.Context, capturer *pcap.Capturer) error {
	return nil
}

// StartRawCapturer starts a capturer on an existing interface on the router instead of a
// monitor type interface.
// This function is useful for the tests that don't care the 802.11 frames but the behavior
// of upper layer traffic and tests can capture packets directly on AP's interface.
func (r *Router) StartRawCapturer(ctx context.Context, name, iface string, ops ...pcap.Option) (*pcap.Capturer, error) {
	return nil, nil
}

// ReserveForStopRawCapturer returns a shortened ctx with cancel function.
// The shortened ctx is used for running things before r.StopRawCapture to reserve time for it.
func (r *Router) ReserveForStopRawCapturer(ctx context.Context, capturer *pcap.Capturer) (context.Context, context.CancelFunc) {
	return ctx, nil
}

// StopRawCapturer stops the packet capturer (no extra resources to release).
func (r *Router) StopRawCapturer(ctx context.Context, capturer *pcap.Capturer) error {
	return nil
}

// NewFrameSender creates a new framesender.Sender object.
func (r *Router) NewFrameSender(ctx context.Context, iface string) (ret *framesender.Sender, retErr error) {
	nd, err := r.monitorOnInterface(ctx, iface)
	if err != nil {
		return nil, err
	}
	r.im.SetBusy(nd.IfName)
	defer func() {
		if retErr != nil {
			r.im.SetAvailable(nd.IfName)
		}
	}()

	if err := r.cloneMAC(ctx, nd.IfName, iface); err != nil {
		return nil, errors.Wrap(err, "failed to clone MAC")
	}
	if err := r.ipr.SetLinkUp(ctx, nd.IfName); err != nil {
		return nil, err
	}
	return framesender.New(r.host, nd.IfName, r.workDir()), nil
}

// CloseFrameSender closes frame sender and releases related resources.
func (r *Router) CloseFrameSender(ctx context.Context, s *framesender.Sender) error {
	err := r.ipr.SetLinkDown(ctx, s.Interface())
	r.im.SetAvailable(s.Interface())
	return err
}

// workDir returns the directory to place temporary files on router.
func (r *Router) workDir() string {
	return common.WorkingDir
}

// NewBridge returns a bridge name for tests to use. Note that the caller is responsible to call ReleaseBridge.
func (r *Router) NewBridge(ctx context.Context) (_ string, retErr error) {
	return "", nil
}

// ReleaseBridge releases the bridge.
func (r *Router) ReleaseBridge(ctx context.Context, br string) error {
	return nil
}

// NewVethPair returns a veth pair for tests to use. Note that the caller is responsible to call ReleaseVethPair.
func (r *Router) NewVethPair(ctx context.Context) (string, string, error) {
	return "", "", nil
}

// ReleaseVethPair release the veth pair.
// Note that each side of the pair can be passed to this method, but the test should only call the method once for each pair.
func (r *Router) ReleaseVethPair(ctx context.Context, veth string) error {
	return nil
}

// BindVethToBridge binds the veth to bridge.
func (r *Router) BindVethToBridge(ctx context.Context, veth, br string) error {
	return nil
}

// UnbindVeth unbinds the veth to any other interface.
func (r *Router) UnbindVeth(ctx context.Context, veth string) error {
	return nil
}

// Utilities for resource control.

// createWifiIface creates an interface on phy with type=t and returns the name of created interface.
func (r *Router) createWifiIface(ctx context.Context, phyID int, t iw.IfType) (*iw.NetDev, error) {
	ctx, st := timing.Start(ctx, "createWifiIface")
	defer st.End()
	phyName := r.phys[phyID].Name
	return r.im.Create(ctx, phyName, phyID, t)
}

// cloneMAC clones the MAC address of src to dst.
func (r *Router) cloneMAC(ctx context.Context, dst, src string) error {
	mac, err := r.ipr.MAC(ctx, src)
	if err != nil {
		return err
	}
	return r.ipr.SetMAC(ctx, dst, mac)
}

// CollectLogs downloads log files from router to OutDir.
func (r *Router) CollectLogs(ctx context.Context) error {
	return common.CollectRouterFileLogs(ctx, r, r.logCollectors, logsToCollect, "")
}

// SetAPIfaceDown brings down the interface that the APIface uses.
func (r *Router) SetAPIfaceDown(ctx context.Context, iface string) error {
	if err := r.ipr.SetLinkDown(ctx, iface); err != nil {
		return errors.Wrapf(err, "failed to set %s down", iface)
	}
	return nil
}

// MAC returns the MAC address of iface on this router.
func (r *Router) MAC(ctx context.Context, iface string) (net.HardwareAddr, error) {
	return r.ipr.MAC(ctx, iface)
}

// HostIsUbuntuRouter determines whether the remote host is a Ubuntu router.
func HostIsUbuntuRouter(ctx context.Context, host *ssh.Conn) (bool, error) {
	lsbReleaseMatchIfUbuntu := "(?m)^DISTRIB_ID=Ubuntu$"
	matches, err := common.HostFileContentsMatch(ctx, host, lsbReleasePath, lsbReleaseMatchIfUbuntu)
	if err != nil {
		return false, errors.Wrapf(err, "failed to check if remote file %q contents match %q", lsbReleasePath, lsbReleaseMatchIfUbuntu)
	}
	return matches, nil
}
