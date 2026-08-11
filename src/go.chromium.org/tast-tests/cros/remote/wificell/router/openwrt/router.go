// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package openwrt

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/chromiumos/config/go/test/lab/api"
	"go.chromium.org/tast-tests/cros/common/utils"
	"go.chromium.org/tast-tests/cros/common/wifi/iw"
	"go.chromium.org/tast-tests/cros/remote/fileutils"
	"go.chromium.org/tast-tests/cros/remote/log"
	remoteIp "go.chromium.org/tast-tests/cros/remote/network/ip"
	remoteIw "go.chromium.org/tast-tests/cros/remote/wifi/iw"
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
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	readyStatusFile = "/tmp/cros/status/ready"
	buildInfoFile   = "/etc/cros/cros_openwrt_image_build_info.json"
)

var (
	buildInfoDeviceName string
)

// Router controls an OpenWrt router and stores the router state.
type Router struct {
	host             *ssh.Conn
	name             string
	routerType       support.RouterType
	routerModel      string
	syslogdCollector *log.LogreadCollector
	iwr              *remoteIw.Runner
	ipr              *remoteIp.Runner
	phys             map[int]*iw.Phy // map from phy idx to iw.Phy.
	im               *common.IfaceManager
	activeServices   activeServices
	nextBridgeID     int
	nextVethID       int
	closed           bool
	workDirPath      string
}

// activeServices keeps a record of what services have been started and not yet
// stopped manually so that they can be stopped during Router.Close.
type activeServices struct {
	hostapd    []*hostapd.Server
	dhcp       []*dhcp.Server
	capture    []*pcap.Capturer
	rawCapture []*pcap.Capturer
}

// NewRouter prepares initial test AP state (e.g., initializing wiphy/wdev).
// ctx is the deadline for the step and daemonCtx is the lifetime for background
// daemons.
func NewRouter(ctx, daemonCtx context.Context, host *ssh.Conn, name string) (router *Router, err error) {
	ctx, st := timing.Start(ctx, "initialize")
	defer st.End()

	testing.ContextLogf(ctx, "Creating new OpenWrt router controller for router %q", name)
	r := &Router{
		host:           host,
		name:           name,
		routerType:     support.OpenWrtT,
		routerModel:    createUniqueRouterModel(),
		iwr:            remoteIw.NewRemoteRunner(host),
		ipr:            remoteIp.NewRemoteRunner(host),
		phys:           make(map[int]*iw.Phy),
		activeServices: activeServices{},
		closed:         false,
		workDirPath:    common.BuildWorkingDirPath(),
	}
	r.im = common.NewRouterIfaceManager(r, r.iwr)

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

	testing.ContextLog(ctx, "Collecting router build info")
	if err := r.logBuildInfo(ctx); err != nil {
		// Just log the error and move on, as the router still may be usable.
		testing.ContextLog(ctx, "Failed to log OpenWrt build info file from host: ", err)
	}

	testing.ContextLog(ctx, "Waiting for router to be ready for testing")
	if err := r.waitForReady(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to wait for router to say it is ready: ", err)
		testing.ContextLog(ctx, "Assuming router is ready and continuing with setup")
	}

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

	testing.ContextLog(ctx, "Killing hostapd and dhcp processes")
	if err := r.killHostapdDHCP(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to kill hostapd and DHCP")
	}

	testing.ContextLog(ctx, "Setting up wifi phys")
	if err := r.setupWifiPhys(ctx); err != nil {
		return nil, err
	}

	// Clean up any lingering bridge and veth ifaces.
	testing.ContextLog(ctx, "Removing all bridge and veth interfaces")
	if err := common.RemoveAllBridgeIfaces(ctx, r.ipr); err != nil {
		return nil, err
	}
	if err := common.RemoveAllVethIfaces(ctx, r.ipr); err != nil {
		return nil, err
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

	testing.ContextLogf(ctx, "Created new OpenWrt router controller for router %q", r.name)
	return r, nil
}

// Close cleans the resource used by Router.
func (r *Router) Close(ctx context.Context) error {
	if r.closed {
		return errors.Errorf("router controller for router %s router %q already closed", r.RouterType().String(), r.RouterName())
	}
	ctx, st := timing.Start(ctx, "router.Close")
	defer st.End()

	testing.ContextLogf(ctx, "Closing OpenWrt router controller for router %q", r.name)

	var firstErr error

	// Collect closing log to facilitate debugging.
	if r.syslogdCollector != nil {
		if err := common.CollectRouterLogs(ctx, r, r.syslogdCollector, "pre_close"); err != nil {
			utils.CollectFirstErr(ctx, &firstErr, errors.Wrap(err, "failed to collect syslogd logs before close actions"))
		}
	}

	// Remove the interfaces that we created.
	if r.im != nil {
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
	}

	// Stop any services that were not manually stopped already.
	if err := r.closeActiveServices(ctx); err != nil {
		utils.CollectFirstErr(ctx, &firstErr, errors.Wrap(err, "failed to stop still active services"))
	}

	// Clean up any lingering bridge and veth ifaces.
	if r.ipr != nil {
		if err := common.RemoveAllBridgeIfaces(ctx, r.ipr); err != nil {
			utils.CollectFirstErr(ctx, &firstErr, err)
		}
		if err := common.RemoveAllVethIfaces(ctx, r.ipr); err != nil {
			utils.CollectFirstErr(ctx, &firstErr, err)
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

	testing.ContextLogf(ctx, "Closed OpenWrt router controller for router %q", r.name)
	r.closed = true
	return firstErr
}

func (r *Router) closeActiveServices(ctx context.Context) error {
	var firstError error
	for len(r.activeServices.hostapd) != 0 {
		if err := r.StopHostapd(ctx, r.activeServices.hostapd[0]); err != nil {
			utils.CollectFirstErr(ctx, &firstError, err)
		}
	}
	for len(r.activeServices.dhcp) != 0 {
		if err := r.StopDHCP(ctx, r.activeServices.dhcp[0]); err != nil {
			utils.CollectFirstErr(ctx, &firstError, err)
		}
	}
	for len(r.activeServices.capture) != 0 {
		if err := r.StopCapture(ctx, r.activeServices.capture[0]); err != nil {
			utils.CollectFirstErr(ctx, &firstError, err)
		}
	}
	for len(r.activeServices.rawCapture) != 0 {
		if err := r.StopRawCapturer(ctx, r.activeServices.rawCapture[0]); err != nil {
			utils.CollectFirstErr(ctx, &firstError, err)
		}
	}
	return firstError
}

// RouterType returns the router type.
func (r *Router) RouterType() support.RouterType {
	return r.routerType
}

// RouterModel returns the router's model.
func (r *Router) RouterModel() string {
	return r.routerModel
}

// createUniqueRouterModel creates the router model name by combining the router Type and the device name from the build info.
func createUniqueRouterModel() string {
	rModel := "openwrt"
	devName := strings.ReplaceAll(buildInfoDeviceName, " ", "")
	if devName != "" {
		rModel = rModel + "_" + devName
	}
	return rModel
}

// RouterName returns the name of the managed router device.
func (r *Router) RouterName() string {
	return r.name
}

// RouterIwr returns the iw runner of the router.
func (r *Router) RouterIwr() *iw.Runner {
	return r.iwr
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

// logBuildInfo retrieves the build info file present on all ChromeOS OpenWrt
// test routers, saves a copy of it to the logs, and logs the key info. Also, save
// the device type and name which are used to create the unique model name for the
// router.
func (r *Router) logBuildInfo(ctx context.Context) error {
	// Get the build info from the router.
	if err := r.host.CommandContext(ctx, "test", "-f", buildInfoFile).Run(); err != nil {
		return errors.Wrapf(err, "build info file file %q` not present", buildInfoFile)
	}
	buildInfoJSON, err := r.host.CommandContext(ctx, "cat", buildInfoFile).Output()
	if err != nil {
		return errors.Wrapf(err, "failed to cat build info file %q", buildInfoFile)
	}

	// Save a copy of the build info to the tast logs for reference while debugging.
	if err := common.LogRouterFile(ctx, r, filepath.Base(buildInfoFile), buildInfoJSON); err != nil {
		return errors.Wrapf(err, "failed to log build info file %q from router %q", buildInfoFile, string(buildInfoJSON))
	}

	// Parse and log key build info data to the context log for easy reference.
	buildInfo := &api.CrosOpenWrtImageBuildInfo{}
	if err := protojson.Unmarshal(buildInfoJSON, buildInfo); err != nil {
		return errors.Wrap(err, "failed to unmarshal build info file from router")
	}
	buildInfoDeviceName = buildInfo.GetStandardBuildConfig().GetDeviceName()
	minimalBuildInfo := &api.CrosOpenWrtImageBuildInfo{
		StandardBuildConfig: &api.CrosOpenWrtImageBuildInfo_StandardBuildConfig{
			BuildProfile: buildInfo.GetStandardBuildConfig().GetBuildProfile(),
			DeviceName:   buildInfo.GetStandardBuildConfig().GetDeviceName(),
		},
		ImageUuid:       buildInfo.GetImageUuid(),
		BuildTime:       buildInfo.GetBuildTime(),
		CustomImageName: buildInfo.GetCustomImageName(),
		OsRelease: &api.CrosOpenWrtImageBuildInfo_OSRelease{
			OpenwrtRelease: buildInfo.GetOsRelease().GetOpenwrtRelease(),
		},
		RouterFeatures: buildInfo.GetRouterFeatures(),
	}
	minimalBuildInfoJSON, err := protojson.Marshal(minimalBuildInfo)
	if err != nil {
		return errors.Wrap(err, "failed to marshal minimal build info")
	}
	testing.ContextLogf(ctx, "CrosOpenWrtImageBuildInfo: %s", string(minimalBuildInfoJSON))
	return nil
}

// waitForReady blocks until the router is ready to be configured.
// The router is seen as ready when the ready status file, created by the last
// boot script, is present.
func (r *Router) waitForReady(ctx context.Context) error {
	ctx, t := timing.Start(ctx, "waitForReady")
	defer t.End()
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := r.host.CommandContext(ctx, "test", "-f", readyStatusFile).Run(); err != nil {
			return errors.Wrapf(err, "ready status file %q not present", readyStatusFile)
		}
		return nil
	}, &testing.PollOptions{
		Interval: 1 * time.Second,
		Timeout:  30 * time.Second,
	}); err != nil {
		return errors.Wrap(err, "failed to wait for ready file on router")
	}
	return nil
}

// workDir returns the directory to place temporary files on router.
func (r *Router) workDir() string {
	return r.workDirPath
}

// CollectLogs dumps collected syslogd logs to a file.
func (r *Router) CollectLogs(ctx context.Context) error {
	return common.CollectRouterLogs(ctx, r, r.syslogdCollector, "")
}

// killHostapdDHCP forcibly kills any hostapd and dhcp processes.
func (r *Router) killHostapdDHCP(ctx context.Context) error {
	shortCtx, st := timing.Start(ctx, "killHostapdDHCP")
	defer st.End()
	var firstErr error
	if err := r.host.CommandContext(shortCtx, "/etc/init.d/dnsmasq", "stop").Run(); err != nil {
		utils.CollectFirstErr(ctx, &firstErr, errors.Wrap(err, "failed to stop dnsmasq service which manages core OpenWrt DHCP servers"))
	}
	if err := r.host.CommandContext(shortCtx, "/etc/init.d/dnsmasq", "disable").Run(); err != nil {
		utils.CollectFirstErr(ctx, &firstErr, errors.Wrap(err, "failed to disable dnsmasq service which manages core OpenWrt DHCP servers"))
	}
	if err := r.host.CommandContext(shortCtx, "/etc/init.d/wpad", "stop").Run(); err != nil {
		utils.CollectFirstErr(ctx, &firstErr, errors.Wrap(err, "failed to stop wpad service which manages core OpenWrt hostapd processes"))
	}
	if err := r.host.CommandContext(shortCtx, "/etc/init.d/wpad", "disable").Run(); err != nil {
		utils.CollectFirstErr(ctx, &firstErr, errors.Wrap(err, "failed to disable wpad service which manages core OpenWrt hostapd processes"))
	}
	if err := hostapd.KillAll(shortCtx, r.host); err != nil {
		utils.CollectFirstErr(ctx, &firstErr, errors.Wrap(err, "failed to kill all hostapd processes"))
	}
	if err := dhcp.KillAll(shortCtx, r.host); err != nil {
		utils.CollectFirstErr(ctx, &firstErr, errors.Wrap(err, "failed to kill all dhcp processes"))
	}
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
		// Get phy index using system config and map it.
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

// StartHostapd starts the hostapd server.
func (r *Router) StartHostapd(ctx context.Context, name string, confs ...*hostapd.Config) (_ *hostapd.Server, retErr error) {
	ctx, st := timing.Start(ctx, "router.StartHostapd")
	defer st.End()
	var ifaces []*hostapd.Iface
	for _, conf := range confs {
		nd, err := r.netDev(ctx, conf.Channel, conf.Is6GHz, iw.IfTypeManaged)
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
		ifaces = append(ifaces, hostapd.NewIface(iface, conf))
	}
	hs, retErr := r.startHostapdOnIfaces(ctx, name, ifaces)
	if retErr != nil {
		return nil, retErr
	}
	r.activeServices.hostapd = append(r.activeServices.hostapd, hs)
	return hs, nil
}

// StopHostapd stops the hostapd server.
func (r *Router) StopHostapd(ctx context.Context, hs *hostapd.Server) error {
	var firstErr error
	if err := hs.Close(ctx); err != nil {
		utils.CollectFirstErr(ctx, &firstErr, errors.Wrap(err, "failed to stop hostapd"))
	}
	for _, iface := range hs.Interfaces() {
		utils.CollectFirstErr(ctx, &firstErr, r.ipr.SetLinkDown(ctx, iface))
		r.im.SetAvailable(iface)
	}

	// Remove from active services.
	for i, service := range r.activeServices.hostapd {
		if hs == service {
			active := make([]*hostapd.Server, 0)
			active = append(active, r.activeServices.hostapd[:i]...)
			active = append(active, r.activeServices.hostapd[i+1:]...)
			r.activeServices.hostapd = active
			break
		}
	}
	return firstErr
}

// netDev finds an available interface suitable for the given channel, band and type.
func (r *Router) netDev(ctx context.Context, channel int, is6GHz bool, t iw.IfType) (*iw.NetDev, error) {
	ctx, st := timing.Start(ctx, "netDev")
	defer st.End()

	phyID, err := r.phy(ctx, channel, is6GHz, t)
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

// createWifiIface creates an interface on phy with type=t and returns the name of created interface.
func (r *Router) createWifiIface(ctx context.Context, phyID int, t iw.IfType) (*iw.NetDev, error) {
	ctx, st := timing.Start(ctx, "createWifiIface")
	defer st.End()
	phyName := r.phys[phyID].Name
	return r.im.Create(ctx, phyName, phyID, t)
}

// phy finds a suitable phy for the given channel, band and target interface type t.
// The selected phy index is returned.
func (r *Router) phy(ctx context.Context, channel int, is6GHz bool, t iw.IfType) (int, error) {
	freq, err := hostapd.ChannelToFrequencyWithBand(channel, is6GHz)
	if err != nil {
		return 0, errors.Errorf("channel %d not available", channel)
	}
	// Try to find an idle phy which is suitable.
	for id, phy := range r.phys {
		if r.im.IsPhyBusy(id, t) {
			continue
		}
		if phySupportsFrequency(phy, freq) {
			return id, nil
		}
	}
	// Try to find any phy which is suitable, even a busy one.
	for id, phy := range r.phys {
		if phySupportsFrequency(phy, freq) {
			return id, nil
		}
	}
	return 0, errors.Errorf("cannot find supported phy for channel=%d, is6GHz=%t", channel, is6GHz)
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

func (r *Router) startHostapdOnIfaces(ctx context.Context, name string, ifaces []*hostapd.Iface) (_ *hostapd.Server, retErr error) {
	ctx, st := timing.Start(ctx, "router.startHostapdOnIfaces")
	defer st.End()
	for _, iface := range ifaces {
		if err := iface.Config().SecurityConfig.InstallRouterCredentials(ctx, r.host, r.workDir()); err != nil {
			return nil, errors.Wrapf(err, "failed to install router credentials for interface %s", iface.Name())
		}
		// Log a warning about WEP since OpenWrt support is device and OS version
		// dependent. Allow it to still attempt the configuration though, as it's
		// possible that an OpenWrt device has explicitly enabled it.
		if isWEP, err := common.HostapdSecurityConfigIsWEP(iface.Config().SecurityConfig); err != nil {
			return nil, errors.Wrap(err, "failed to check if hostapd security config uses wep")
		} else if isWEP {
			testing.ContextLog(
				ctx,
				"Warning: Starting hostapd with a security config using WEP. "+
					"OpenWrt routers are not guaranteed to support WEP in future OS"+
					"versions. If this device does not support WEP, hostapd will fail to"+
					"start.",
			)
		}
	}
	hs, err := hostapd.StartServerOnIface(ctx, r.host, name, r.workDir(), ifaces, map[string]string{})
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

	for _, iface := range ifaces {
		if isSupported, err := r.iwr.IsSetTxPowerSupported(ctx, iface.Name()); err == nil && isSupported {
			if err := r.iwr.SetTxPowerAuto(ctx, iface.Name()); err != nil {
				return nil, errors.Wrapf(err, "failed to set txpower on interface %s to auto", iface.Name())
			}
		}
	}
	return hs, nil
}

// ReconfigureHostapd restarts the hostapd server with the new config. It
// preserves the interface and the name of the old hostapd server.
func (r *Router) ReconfigureHostapd(ctx context.Context, hs *hostapd.Server, conf *hostapd.Config) (*hostapd.Server, error) {
	iface := hs.Interface()
	name := hs.Name()
	if err := r.StopHostapd(ctx, hs); err != nil {
		return nil, errors.Wrap(err, "failed to stop hostapd server")
	}
	r.im.SetBusy(iface)
	hs, err := r.startHostapdOnIfaces(ctx, name, []*hostapd.Iface{hostapd.NewIface(iface, conf)})
	if err != nil {
		r.im.SetAvailable(iface)
		return nil, err
	}
	r.activeServices.hostapd = append(r.activeServices.hostapd, hs)
	return hs, nil
}

// StartDHCP starts the DHCP server and configures the server IP. If DNS functionality is
// not required, set dnsOpt to nil.
func (r *Router) StartDHCP(ctx context.Context, name, iface string, ipStart, ipEnd, serverIP, broadcastIP net.IP, mask net.IPMask, dnsOpt *dhcp.DNSOption) (_ *dhcp.Server, retErr error) {
	ds, err := common.StartDHCP(ctx, name, iface, r.workDir(), r.host, r.ipr, ipStart, ipEnd, serverIP, broadcastIP, mask, dnsOpt)
	if err != nil {
		r.activeServices.dhcp = append(r.activeServices.dhcp, ds)
	}
	return ds, err
}

// StopDHCP stops the DHCP server and flushes the interface.
func (r *Router) StopDHCP(ctx context.Context, ds *dhcp.Server) error {
	firstErr := common.StopDHCP(ctx, ds, r.ipr)
	// Remove from active services.
	for i, service := range r.activeServices.dhcp {
		if ds == service {
			active := make([]*dhcp.Server, 0)
			active = append(active, r.activeServices.dhcp[:i]...)
			active = append(active, r.activeServices.dhcp[i+1:]...)
			r.activeServices.dhcp = active
			break
		}
	}
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
	if err := httpServer.Close(ctx); err != nil {
		return errors.Wrap(err, "failed to stop HTTP server")
	}
	return nil
}

// StartCapture starts a packet capturer.
func (r *Router) StartCapture(ctx context.Context, name string, ch int, is6GHz bool, freqOps []iw.SetFreqOption, pcapOps ...pcap.Option) (_ *pcap.Capturer, retErr error) {
	nd, err := r.netDev(ctx, ch, is6GHz, iw.IfTypeMonitor)
	if err != nil {
		return nil, err
	}
	c, err := common.StartCapture(ctx, nd, r.host, r.ipr, r.im, r.iwr, r.workDir(), name, ch, is6GHz, freqOps, pcapOps...)
	if err != nil {
		r.activeServices.capture = append(r.activeServices.capture, c)
	}
	return c, err
}

// StopCapture stops the packet capturer and releases related resources.
func (r *Router) StopCapture(ctx context.Context, capturer *pcap.Capturer) error {
	firstErr := common.StopCapture(ctx, r.ipr, r.im, capturer)
	// Remove from active services.
	for i, service := range r.activeServices.capture {
		if capturer == service {
			active := make([]*pcap.Capturer, 0)
			active = append(active, r.activeServices.capture[:i]...)
			active = append(active, r.activeServices.capture[i+1:]...)
			r.activeServices.capture = active
			break
		}
	}
	return firstErr
}

// StartRawCapturer starts a capturer on an existing interface on the router
// instead of a monitor type interface.
func (r *Router) StartRawCapturer(ctx context.Context, name, iface string, ops ...pcap.Option) (*pcap.Capturer, error) {
	capturer, err := pcap.StartCapturer(ctx, r.host, name, iface, r.workDir(), ops...)
	if err != nil {
		return nil, err
	}
	r.activeServices.rawCapture = append(r.activeServices.rawCapture, capturer)
	return capturer, nil
}

// StopRawCapturer stops the packet capturer (no extra resources to release).
func (r *Router) StopRawCapturer(ctx context.Context, capturer *pcap.Capturer) (retErr error) {
	if err := capturer.Close(ctx); err != nil {
		retErr = errors.Wrap(err, "failed to stop capturer")
	}
	// Remove from active services.
	for i, service := range r.activeServices.rawCapture {
		if capturer == service {
			active := make([]*pcap.Capturer, 0)
			active = append(active, r.activeServices.rawCapture[:i]...)
			active = append(active, r.activeServices.rawCapture[i+1:]...)
			r.activeServices.rawCapture = active
			break
		}
	}
	return retErr
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

// NewBridge returns a bridge name for tests to use. Note that the caller is responsible to call ReleaseBridge.
func (r *Router) NewBridge(ctx context.Context) (string, error) {
	bridgeID := r.nextBridgeID
	r.nextBridgeID++
	return common.NewBridge(ctx, r.ipr, bridgeID)
}

// ReleaseBridge releases the bridge.
func (r *Router) ReleaseBridge(ctx context.Context, br string) error {
	return common.ReleaseBridge(ctx, r.ipr, br)
}

// NewVethPair returns a veth pair for tests to use. Note that the caller is responsible to call ReleaseVethPair.
func (r *Router) NewVethPair(ctx context.Context) (string, string, error) {
	vethID := r.nextVethID
	r.nextVethID++
	return common.NewVethPair(ctx, r.ipr, vethID, true)
}

// ReleaseVethPair releases the veth pair.
// Note that each side of the pair can be passed to this method, but the test should only call the method once for each pair.
func (r *Router) ReleaseVethPair(ctx context.Context, veth string) error {
	return common.ReleaseVethPair(ctx, r.ipr, veth, true)
}

// BindVethToBridge binds the veth to bridge.
func (r *Router) BindVethToBridge(ctx context.Context, veth, br string) error {
	return common.BindVethToBridge(ctx, r.ipr, veth, br)
}

// UnbindVeth unbinds the veth to any other interface.
func (r *Router) UnbindVeth(ctx context.Context, veth string) error {
	return common.UnbindVeth(ctx, r.ipr, veth)
}

// BindIfaceToBridge binds the iface to bridge.
func (r *Router) BindIfaceToBridge(ctx context.Context, iface, br string) error {
	return common.BindIfaceToBridge(ctx, r.ipr, iface, br)
}

// UnbindIface unbinds the iface to any other interface.
func (r *Router) UnbindIface(ctx context.Context, iface string) error {
	return common.UnbindIface(ctx, r.ipr, iface)
}

// HostIsOpenWrtRouter determines whether the remote host is an OpenWrt router.
func HostIsOpenWrtRouter(ctx context.Context, host *ssh.Conn) (bool, error) {
	deviceInfoPath := "/etc/device_info"
	deviceInfoMatchIfOpenWrt := "(?m)^DEVICE_MANUFACTURER='OpenWrt'$"
	matches, err := fileutils.HostFileContentsMatch(ctx, host, deviceInfoPath, deviceInfoMatchIfOpenWrt)
	if err != nil {
		return false, errors.Wrapf(err, "failed to check if remote file %q contents match %q", deviceInfoPath, deviceInfoMatchIfOpenWrt)
	}
	return matches, nil
}

// NewFrameSender creates a new framesender.Sender object.
func (r *Router) NewFrameSender(ctx context.Context, iface string) (ret *framesender.Sender, retErr error) {
	nd, err := r.monitorOnInterface(ctx, iface)
	if err != nil {
		return nil, err
	}
	return common.NewFrameSender(ctx, r.ipr, r.im, nd, r.host, r.workDir(), iface)
}

// CloseFrameSender closes frame sender and releases related resources.
func (r *Router) CloseFrameSender(ctx context.Context, s *framesender.Sender) error {
	return common.CloseFrameSender(ctx, r.ipr, r.im, s)
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
