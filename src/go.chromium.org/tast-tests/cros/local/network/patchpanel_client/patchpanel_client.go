// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package patchpanel interacts with the patchpanel system daemon.
package patchpanel

import (
	"context"
	"os"

	"github.com/godbus/dbus/v5"
	"github.com/golang/protobuf/proto"

	pp "chromiumos/system_api/patchpanel_proto"

	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	jobName                             = "patchpanel"
	dbusName                            = "org.chromium.PatchPanel"
	dbusPath                            = "/org/chromium/PatchPanel"
	connectNamespaceMethod              = "org.chromium.PatchPanel.ConnectNamespace"
	getDevicesMethod                    = "org.chromium.PatchPanel.GetDevices"
	getDownstreamNetworkInfoMethod      = "org.chromium.PatchPanel.GetDownstreamNetworkInfo"
	getTrafficCountersMethod            = "org.chromium.PatchPanel.GetTrafficCounters"
	terminaVMStartupMethod              = "org.chromium.PatchPanel.TerminaVmStartup"
	terminaVMShutdownMethod             = "org.chromium.PatchPanel.TerminaVmShutdown"
	notifyAndroidInteractiveStateMethod = "org.chromium.PatchPanel.NotifyAndroidInteractiveState"
	setFeatureFlagMethod                = "org.chromium.PatchPanel.SetFeatureFlag"
	tagSocketMethod                     = "org.chromium.PatchPanel.TagSocket"
)

// Client is a wrapper around patchpanel DBus API.
type Client struct {
	conn *dbus.Conn
	obj  dbus.BusObject
}

// New connects to the patchpanel daemon via D-Bus and returns a patchpanel Client object.
// TODO(crbug.com/1135106): Implement missing patchpanel D-Bus API methods.
func New(ctx context.Context) (*Client, error) {
	if err := upstart.EnsureJobRunning(ctx, jobName); err != nil {
		return nil, err
	}

	conn, obj, err := dbusutil.Connect(ctx, dbusName, dbusPath)
	if err != nil {
		return nil, err
	}
	return &Client{conn, obj}, nil
}

// ConnectNamespace sends a ConnectNamespaceRequest for the given namespace pid. Returns a pair with
// a open file descriptor and the ConnectNamespaceResponse proto message received if the request succeeded.
// Closing the file descriptor will teardown the veth and routing setup and free the allocated IPv4 subnet.
func (c *Client) ConnectNamespace(ctx context.Context, pid int32, outboundPhysicalDevice string,
	forwardUserTraffic bool) (local *os.File, response *pp.ConnectNamespaceResponse, retErr error) {
	request := &pp.ConnectNamespaceRequest{
		Pid:                    pid,
		OutboundPhysicalDevice: outboundPhysicalDevice,
		AllowUserTraffic:       forwardUserTraffic,
	}
	buf, err := proto.Marshal(request)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed marshaling %s request", connectNamespaceMethod)
	}

	local, remote, err := os.Pipe()
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to open pipe for creating %s request arg", connectNamespaceMethod)

	}
	remoteFd := dbus.UnixFD(remote.Fd())
	defer remote.Close()

	defer func() {
		if retErr != nil {
			local.Close()
		}
	}()
	if retErr = c.obj.CallWithContext(ctx, connectNamespaceMethod, 0, buf, remoteFd).Store(&buf); retErr != nil {
		return nil, nil, errors.Wrapf(retErr, "failed reading %s response", connectNamespaceMethod)
	}

	response = &pp.ConnectNamespaceResponse{}
	if retErr = proto.Unmarshal(buf, response); retErr != nil {
		return nil, nil, errors.Wrapf(retErr, "failed unmarshaling %s response", connectNamespaceMethod)
	}

	return local, response, nil
}

// NotifyTerminaVMStartup sends a TerminaVmStartupRequest for the given container id. The ID must be unique in the system.
func (c *Client) NotifyTerminaVMStartup(ctx context.Context, cid uint32) (response *pp.TerminaVmStartupResponse, retErr error) {
	request := &pp.TerminaVmStartupRequest{
		Cid: cid,
	}
	buf, err := proto.Marshal(request)
	if err != nil {
		return nil, errors.Wrapf(err, "failed marshaling %s request", terminaVMStartupMethod)
	}

	var state []uint8
	if retErr = c.obj.CallWithContext(ctx, terminaVMStartupMethod, 0, buf).Store(&state); retErr != nil {
		// Send a shutdown request as we cannot tell if it failed before or after patchpanel allocates a FD.
		c.NotifyTerminaVMShutdown(ctx, cid)
		return nil, errors.Wrapf(retErr, "failed reading %s response", terminaVMStartupMethod)
	}

	response = &pp.TerminaVmStartupResponse{}
	if retErr = proto.Unmarshal(state, response); retErr != nil {
		return nil, errors.Wrapf(retErr, "failed unmarshaling %s response", terminaVMStartupMethod)
	}

	return response, nil
}

// NotifyTerminaVMShutdown sends a TerminaVmShutdownRequest for the given container id.
func (c *Client) NotifyTerminaVMShutdown(ctx context.Context, cid uint32) error {
	request := &pp.TerminaVmShutdownRequest{
		Cid: cid,
	}
	buf, err := proto.Marshal(request)
	if err != nil {
		return errors.Wrapf(err, "failed marshaling %s request", terminaVMShutdownMethod)
	}

	var state []uint8
	if err = c.obj.CallWithContext(ctx, terminaVMShutdownMethod, 0, buf).Store(&state); err != nil {
		return errors.Wrapf(err, "failed reading %s response", terminaVMShutdownMethod)
	}

	response := &pp.TerminaVmShutdownResponse{}
	if err = proto.Unmarshal(state, response); err != nil {
		return errors.Wrapf(err, "failed unmarshaling %s response", terminaVMShutdownMethod)
	}

	return nil
}

// GetDevices gets all patchpanel managed devices information.
func (c *Client) GetDevices(ctx context.Context) (*pp.GetDevicesResponse, error) {
	request := &pp.GetDevicesRequest{}
	buf, err := proto.Marshal(request)
	if err != nil {
		return nil, errors.Wrapf(err, "failed marshaling %s request", getDevicesMethod)
	}

	var result []uint8
	if err = c.obj.CallWithContext(ctx, getDevicesMethod, 0, buf).Store(&result); err != nil {
		return nil, errors.Wrapf(err, "failed reading %s response", getDevicesMethod)
	}

	response := &pp.GetDevicesResponse{}
	if err = proto.Unmarshal(result, response); err != nil {
		return nil, errors.Wrapf(err, "failed unmarshaling %s response", getDevicesMethod)
	}
	return response, nil
}

// GetDownstreamNetworkInfo gets all patchpanel managed devices information.
func (c *Client) GetDownstreamNetworkInfo(ctx context.Context, downstreamIfname string) (*pp.GetDownstreamNetworkInfoResponse, error) {
	request := &pp.GetDownstreamNetworkInfoRequest{
		DownstreamIfname: downstreamIfname,
	}
	buf, err := proto.Marshal(request)
	if err != nil {
		return nil, errors.Wrapf(err, "failed marshaling %s request", getDownstreamNetworkInfoMethod)
	}

	var result []uint8
	if err = c.obj.CallWithContext(ctx, getDownstreamNetworkInfoMethod, 0, buf).Store(&result); err != nil {
		return nil, errors.Wrapf(err, "failed reading %s response", getDownstreamNetworkInfoMethod)
	}

	response := &pp.GetDownstreamNetworkInfoResponse{}
	if err = proto.Unmarshal(result, response); err != nil {
		return nil, errors.Wrapf(err, "failed unmarshaling %s response", getDownstreamNetworkInfoMethod)
	}
	return response, nil
}

// GetTrafficCounters retrieves the current traffic counters for the specified devices.
func (c *Client) GetTrafficCounters(ctx context.Context, devices []string) (*pp.TrafficCountersResponse, error) {
	request := &pp.TrafficCountersRequest{
		Devices: devices,
	}
	buf, err := proto.Marshal(request)
	if err != nil {
		return nil, errors.Wrapf(err, "failed marshaling %s request", getTrafficCountersMethod)
	}

	var result []uint8
	if err = c.obj.CallWithContext(ctx, getTrafficCountersMethod, 0, buf).Store(&result); err != nil {
		return nil, errors.Wrapf(err, "failed reading %s response", getTrafficCountersMethod)
	}

	response := &pp.TrafficCountersResponse{}
	if err = proto.Unmarshal(result, response); err != nil {
		return nil, errors.Wrapf(err, "failed unmarshaling %s response", getTrafficCountersMethod)
	}
	return response, nil
}

// NotifyAndroidInteractiveState sends a NotifyAndroidInteractiveState to change device power interactive status.
func (c *Client) NotifyAndroidInteractiveState(ctx context.Context, interactive bool) (*pp.NotifyAndroidInteractiveStateResponse, error) {
	request := &pp.NotifyAndroidInteractiveStateRequest{
		Interactive: interactive,
	}
	buf, err := proto.Marshal(request)
	if err != nil {
		return nil, errors.Wrapf(err, "failed marshaling %s request", notifyAndroidInteractiveStateMethod)
	}

	var result []uint8
	if err = c.obj.CallWithContext(ctx, notifyAndroidInteractiveStateMethod, 0, buf).Store(&result); err != nil {
		return nil, errors.Wrapf(err, "failed reading %s response", notifyAndroidInteractiveStateMethod)
	}

	response := &pp.NotifyAndroidInteractiveStateResponse{}
	if err = proto.Unmarshal(result, response); err != nil {
		return nil, errors.Wrapf(err, "failed unmarshaling %s response", notifyAndroidInteractiveStateMethod)
	}
	return response, nil
}

// SetQosEnable enables or disables the QoS feature in Patchpanel.
func (c *Client) SetQosEnable(ctx context.Context, enable bool) (bool, error) {
	request := &pp.SetFeatureFlagRequest{
		Flag:    pp.SetFeatureFlagRequest_WIFI_QOS,
		Enabled: enable,
	}
	buf, err := proto.Marshal(request)
	if err != nil {
		return false, errors.Wrapf(err, "failed marshaling %s request", setFeatureFlagMethod)
	}

	var result []uint8
	if err := c.obj.CallWithContext(ctx, setFeatureFlagMethod, 0, buf).Store(&result); err != nil {
		return false, errors.Wrapf(err, "failed reading %s response", setFeatureFlagMethod)
	}

	response := &pp.SetFeatureFlagResponse{}
	if err := proto.Unmarshal(result, response); err != nil {
		return false, errors.Wrapf(err, "failed unmarshaling %s response", setFeatureFlagMethod)
	}
	return response.GetEnabled(), nil
}

// SetQosEnableWithRestore enables or disables the QoS feature in Patchpanel and
// provide a helper function to restore the flag to its previous value.
func (c *Client) SetQosEnableWithRestore(ctx context.Context, enable bool) (func(context.Context), error) {
	old, err := c.SetQosEnable(ctx, enable)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context) {
		if old == enable {
			// No need to restore anything.
			return
		}
		if _, err := c.SetQosEnable(ctx, old); err != nil {
			testing.ContextLog(ctx, "Failed to restore QoS enable feature flag: ", err)
		}
	}, nil
}

type tagSocketParams struct {
	networkID *int32
	vpnPolicy pp.TagSocketRequest_VpnRoutingPolicy
}

// TagSocketOption is the option to call TagSocket().
type TagSocketOption func(*tagSocketParams)

// WithTagSocketNetworkID configures the network_id field to call TagSocket().
func WithTagSocketNetworkID(networkID int) TagSocketOption {
	n := int32(networkID)
	return func(params *tagSocketParams) {
		params.networkID = &n
	}
}

// WithTagSocketRouteOnVPN sets vpn_policy to ROUTE_ON_VPN.
func WithTagSocketRouteOnVPN() TagSocketOption {
	return func(params *tagSocketParams) {
		params.vpnPolicy = pp.TagSocketRequest_ROUTE_ON_VPN
	}
}

// WithTagSocketBypassVPN sets vpn_policy to BY_PASS_VPN.
func WithTagSocketBypassVPN() TagSocketOption {
	return func(params *tagSocketParams) {
		params.vpnPolicy = pp.TagSocketRequest_BYPASS_VPN
	}
}

// TagSocket calls patchpanel to tag the socket represented by fd with opts. See
// the document for this API in the proto file for more details.
func (c *Client) TagSocket(ctx context.Context, fd int32, opts ...TagSocketOption) error {
	params := tagSocketParams{
		vpnPolicy: pp.TagSocketRequest_DEFAULT_ROUTING,
	}
	for _, opt := range opts {
		opt(&params)
	}

	request := &pp.TagSocketRequest{
		NetworkId: params.networkID,
		VpnPolicy: params.vpnPolicy,
	}

	buf, err := proto.Marshal(request)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal %s request", tagSocketMethod)
	}

	dbusFd := dbus.UnixFD(fd)

	if err := c.obj.CallWithContext(ctx, tagSocketMethod, 0, buf, dbusFd).Store(&buf); err != nil {
		return errors.Wrapf(err, "failed to read %s method", tagSocketMethod)
	}

	response := &pp.TagSocketResponse{}
	if err := proto.Unmarshal(buf, response); err != nil {
		return errors.Wrapf(err, "failed to unmarshal %s response", tagSocketMethod)
	}

	if !response.Success {
		return errors.Wrapf(err, "%s returns failure", tagSocketMethod)
	}

	return nil
}
