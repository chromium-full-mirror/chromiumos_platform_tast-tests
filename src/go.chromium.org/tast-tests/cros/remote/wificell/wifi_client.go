// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wificell

import (
	"context"
	"encoding/hex"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/network/protoutil"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/timing"
)

// WifiClient is a wrapper of ShillServiceClient to simplify gRPC calls
// e.g. handle complex streaming gRPCs, hide gRPC request/response, etc.
// Users can still access the raw gRPC with WifiClient.ShillServiceClient.
type WifiClient struct {
	wifi.ShillServiceClient
}

// DiscoverBSSID discovers a service with the given properties.
func (cli *WifiClient) DiscoverBSSID(ctx context.Context, bssid, iface string, ssid []byte) error {
	ctx, st := timing.Start(ctx, "DiscoverBSSID")
	defer st.End()
	request := &wifi.DiscoverBSSIDRequest{
		Bssid:     bssid,
		Interface: iface,
		Ssid:      ssid,
	}
	if _, err := cli.ShillServiceClient.DiscoverBSSID(ctx, request); err != nil {
		return err
	}

	return nil
}

// RequestRoam requests DUT to roam to the specified BSSID and waits until the DUT has roamed.
func (cli *WifiClient) RequestRoam(ctx context.Context, iface, bssid string, timeout time.Duration) error {
	request := &wifi.RequestRoamRequest{
		InterfaceName: iface,
		Bssid:         bssid,
		Timeout:       timeout.Nanoseconds(),
	}
	if _, err := cli.ShillServiceClient.RequestRoam(ctx, request); err != nil {
		return err
	}

	return nil
}

// Reassociate triggers reassociation with the current AP and waits until it has reconnected or the timeout expires.
func (cli *WifiClient) Reassociate(ctx context.Context, iface string, timeout time.Duration) error {
	_, err := cli.ShillServiceClient.Reassociate(ctx, &wifi.ReassociateRequest{
		InterfaceName: iface,
		Timeout:       timeout.Nanoseconds(),
	})
	return err
}

// FlushBSS flushes BSS entries over the specified age from wpa_supplicant's cache.
func (cli *WifiClient) FlushBSS(ctx context.Context, iface string, age time.Duration) error {
	req := &wifi.FlushBSSRequest{
		InterfaceName: iface,
		Age:           age.Nanoseconds(),
	}
	_, err := cli.ShillServiceClient.FlushBSS(ctx, req)
	return err
}

// AssureDisconnect assures that the WiFi service has disconnected within timeout.
func (cli *WifiClient) AssureDisconnect(ctx context.Context, servicePath string, timeout time.Duration) error {
	req := &wifi.AssureDisconnectRequest{
		ServicePath: servicePath,
		Timeout:     timeout.Nanoseconds(),
	}
	if _, err := cli.ShillServiceClient.AssureDisconnect(ctx, req); err != nil {
		return err
	}
	return nil
}

// GetServicePath returns the object path of the service matching the properties in request.
func (cli *WifiClient) GetServicePath(ctx context.Context, props map[string]interface{}) (string, error) {
	propsEnc, err := protoutil.EncodeToShillValMap(props)
	if err != nil {
		return "", errors.Wrap(err, "failed to encode shill properties")
	}
	req := &wifi.ServicePathRequest{Props: propsEnc}
	rsp, err := cli.ShillServiceClient.GetServicePath(ctx, req)
	if err != nil {
		return "", err
	}
	return rsp.ServicePath, nil
}

// QueryServiceWithPath queries shill information about service pointed by the servicePath.
func (cli *WifiClient) QueryServiceWithPath(ctx context.Context, servicePath string) (*wifi.QueryServiceResponse, error) {
	req := &wifi.QueryServiceRequest{
		Path: servicePath,
	}
	resp, err := cli.ShillServiceClient.QueryService(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get the service information")
	}

	return resp, nil
}

// GetTetheringConfig returns the tethering configuration.
func (cli *WifiClient) GetTetheringConfig(ctx context.Context) (*wifi.GetTetheringConfigResponse, error) {
	resp, err := cli.ShillServiceClient.GetTetheringConfig(ctx, &empty.Empty{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get tethering config")
	}

	return resp, nil
}

// QueryService queries shill information of selected service.
func (cli *WifiClient) QueryService(ctx context.Context) (*wifi.QueryServiceResponse, error) {
	selectedSvcResp, err := cli.ShillServiceClient.SelectedService(ctx, &empty.Empty{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get selected service")
	}

	return cli.QueryServiceWithPath(ctx, selectedSvcResp.ServicePath)
}

// Interface returns the WiFi interface name of the DUT.
func (cli *WifiClient) Interface(ctx context.Context) (string, error) {
	netIf, err := cli.ShillServiceClient.GetInterface(ctx, &empty.Empty{})
	if err != nil {
		return "", errors.Wrap(err, "failed to get the WiFi interface name")
	}
	return netIf.Name, nil
}

// CurrentTime returns the current time on DUT.
func (cli *WifiClient) CurrentTime(ctx context.Context) (time.Time, error) {
	res, err := cli.ShillServiceClient.GetCurrentTime(ctx, &empty.Empty{})
	if err != nil {
		return time.Time{}, errors.Wrap(err, "failed to get the current DUT time")
	}
	currentTime := time.Unix(res.NowSecond, res.NowNanosecond)
	return currentTime, nil
}

// ShillProperty holds a shill service property with it's expected and unexpected values.
type ShillProperty struct {
	Property         string
	ExpectedValues   []interface{}
	UnexpectedValues []interface{}
	Method           wifi.ExpectShillPropertyRequest_CheckMethod
}

// ExpectShillProperty is a wrapper for the streaming gRPC call ExpectShillProperty.
// It takes an array of ShillProperty, an array of shill properties to monitor, and
// a shill service path. It returns a function that waites for the expected property
// changes and returns the monitor results.
func (cli *WifiClient) ExpectShillProperty(ctx context.Context, objectPath string, props []*ShillProperty, monitorProps []string) (func() ([]protoutil.ShillPropertyHolder, error), error) {
	var expectedProps []*wifi.ExpectShillPropertyRequest_Criterion
	for _, prop := range props {
		var anyOfVals []*wifi.ShillVal
		for _, shillState := range prop.ExpectedValues {
			state, err := protoutil.ToShillVal(shillState)
			if err != nil {
				return nil, errors.Wrap(err, "failed to convert property name to ShillVal")
			}
			anyOfVals = append(anyOfVals, state)
		}

		var noneOfVals []*wifi.ShillVal
		for _, shillState := range prop.UnexpectedValues {
			state, err := protoutil.ToShillVal(shillState)
			if err != nil {
				return nil, errors.Wrap(err, "failed to convert property name to ShillVal")
			}
			noneOfVals = append(noneOfVals, state)
		}

		shillPropReqCriterion := &wifi.ExpectShillPropertyRequest_Criterion{
			Key:    prop.Property,
			AnyOf:  anyOfVals,
			NoneOf: noneOfVals,
			Method: prop.Method,
		}
		expectedProps = append(expectedProps, shillPropReqCriterion)
	}

	req := &wifi.ExpectShillPropertyRequest{
		ObjectPath:   objectPath,
		Props:        expectedProps,
		MonitorProps: monitorProps,
	}

	stream, err := cli.ShillServiceClient.ExpectShillProperty(ctx, req)
	if err != nil {
		return nil, err
	}

	ready, err := stream.Recv()
	if err != nil || ready.Key != "" {
		// Error due to expecting an empty response as ready signal.
		return nil, errors.New("failed to get the ready signal")
	}

	// Get the expected properties and values.
	waitForProperties := func() ([]protoutil.ShillPropertyHolder, error) {
		for {
			resp, err := stream.Recv()
			if err != nil {
				return nil, errors.Wrap(err, "failed to get the expected properties")
			}

			if resp.MonitorDone {
				return protoutil.DecodeFromShillPropertyChangedSignalList(resp.Props)
			}

			// Now we get the matched state change in resp.
			stateVal, err := protoutil.FromShillVal(resp.Val)
			if err != nil {
				return nil, errors.Wrap(err, "failed to convert property name to ShillVal")
			}
			testing.ContextLogf(ctx, "The current WiFi service %s: %v", resp.Key, stateVal)
		}
	}

	return waitForProperties, nil
}

// GenerateRoamPropertyWatcher returns a property watcher for roam state changes
// and whether WiFi.BSSID updates to roamBSSID.
func (cli *WifiClient) GenerateRoamPropertyWatcher(waitCtx context.Context, roamBSSID, servicePath string) (func() ([]protoutil.ShillPropertyHolder, error), error) {
	props := []*ShillProperty{{
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
		ExpectedValues: []interface{}{roamBSSID},
		Method:         wifi.ExpectShillPropertyRequest_CHECK_ONLY,
	}}

	monitorProps := []string{shillconst.ServicePropertyIsConnected}
	waitForProps, err := cli.ExpectShillProperty(waitCtx, servicePath, props, monitorProps)
	return waitForProps, err
}

// WaitForConnected queries a WiFi service with specified |ssid|, and waits for
// its shill property: "ServicePropertyIsConnected" to be the same as |expectedValue|.
// Note that the network needs to be added in advance, this function will not attempt to add any network.
func (cli *WifiClient) WaitForConnected(ctx context.Context, ssid string, expectedValue bool) error {
	props := map[string]interface{}{
		shillconst.ServicePropertyType:        shillconst.TypeWifi,
		shillconst.ServicePropertyWiFiHexSSID: strings.ToUpper(hex.EncodeToString([]byte(ssid))),
	}

	var servicePath string
	// The service path may not be found immediately after the network is added.
	if err := testing.Poll(ctx, func(ctx context.Context) (err error) {
		servicePath, err = cli.GetServicePath(ctx, props)
		return err
	}, &testing.PollOptions{Timeout: shillconst.DefaultTimeout, Interval: time.Second}); err != nil {
		return err
	}

	req := []*ShillProperty{{
		Property:       shillconst.ServicePropertyIsConnected,
		ExpectedValues: []interface{}{expectedValue},
		Method:         wifi.ExpectShillPropertyRequest_CHECK_WAIT,
	}}

	waitCtx, cancel := context.WithTimeout(ctx, shillconst.DefaultTimeout)
	defer cancel()

	waitServiceConnected, err := cli.ExpectShillProperty(waitCtx, servicePath, req, nil)
	if err != nil {
		return errors.Wrap(err, "failed to create a property watcher")
	}

	if _, err := waitServiceConnected(); err != nil {
		return errors.Wrap(err, "failed to wait for service connected")
	}
	return nil
}

// WaitForWiFiServiceStates queries a WiFi service with specified |ssid|, and
// waits for all the provided service states to be matched within shill property
// "ServicePropertyState".
func (cli *WifiClient) WaitForWiFiServiceStates(ctx context.Context, ssid string, states []string) error {
	props := map[string]interface{}{
		shillconst.ServicePropertyType:        shillconst.TypeWifi,
		shillconst.ServicePropertyWiFiHexSSID: strings.ToUpper(hex.EncodeToString([]byte(ssid))),
	}

	var servicePath string
	// The service path may not be found immediately after the network is added.
	if err := testing.Poll(ctx, func(ctx context.Context) (err error) {
		servicePath, err = cli.GetServicePath(ctx, props)
		return err
	}, &testing.PollOptions{Timeout: shillconst.DefaultTimeout, Interval: time.Second}); err != nil {
		return err
	}

	var req []*ShillProperty
	for _, state := range states {
		req = append(req, &ShillProperty{
			Property:       shillconst.ServicePropertyState,
			ExpectedValues: []interface{}{state},
			Method:         wifi.ExpectShillPropertyRequest_CHECK_WAIT,
		})
	}

	waitCtx, cancel := context.WithTimeout(ctx, shillconst.DefaultTimeout)
	defer cancel()

	waitServiceStates, err := cli.ExpectShillProperty(waitCtx, servicePath, req, nil)
	if err != nil {
		return errors.Wrap(err, "failed to create a property watcher")
	}

	if _, err := waitServiceStates(); err != nil {
		return errors.Wrap(err, "failed to wait for service states")
	}
	return nil
}

// EAPAuthSkipped is a wrapper for the streaming gRPC call EAPAuthSkipped.
// It returns a function that waits and verifies the EAP authentication is skipped or not in the next connection.
func (cli *WifiClient) EAPAuthSkipped(ctx context.Context) (func() (bool, error), error) {
	recv, err := cli.ShillServiceClient.EAPAuthSkipped(ctx, &empty.Empty{})
	if err != nil {
		return nil, err
	}
	s, err := recv.Recv()
	if err != nil {
		return nil, errors.Wrap(err, "failed to receive ready signal from EAPAuthSkipped")
	}
	if s.Skipped {
		return nil, errors.New("unexpected ready signal: got true, want false")
	}
	return func() (bool, error) {
		resp, err := recv.Recv()
		if err != nil {
			return false, errors.Wrap(err, "failed to receive from EAPAuthSkipped")
		}
		return resp.Skipped, nil
	}, nil
}

// DisconnectReason is a wrapper for the streaming gRPC call DisconnectReason.
// It returns a function that waits for the wpa_supplicant DisconnectReason
// property change, and returns the disconnection reason code.
func (cli *WifiClient) DisconnectReason(ctx context.Context) (func() (int32, error), error) {
	recv, err := cli.ShillServiceClient.DisconnectReason(ctx, &empty.Empty{})
	if err != nil {
		return nil, err
	}
	ready, err := recv.Recv()
	if err != nil || ready.Reason != 0 {
		// Error due to expecting an empty response as ready signal.
		return nil, errors.New("failed to get the ready signal")
	}
	return func() (int32, error) {
		resp, err := recv.Recv()
		if err != nil {
			return 0, errors.Wrap(err, "failed to receive from DisconnectReason")
		}
		return resp.Reason, nil
	}, nil
}

// SuspendAssertConnect suspends the DUT for wakeUpTimeout seconds through gRPC and returns the duration from resume to connect.
func (cli *WifiClient) SuspendAssertConnect(ctx context.Context, wakeUpTimeout time.Duration) (time.Duration, error) {
	service, err := cli.ShillServiceClient.SelectedService(ctx, &empty.Empty{})
	if err != nil {
		return 0, errors.Wrap(err, "failed to get selected service")
	}
	resp, err := cli.ShillServiceClient.SuspendAssertConnect(ctx, &wifi.SuspendAssertConnectRequest{
		WakeUpTimeout: wakeUpTimeout.Nanoseconds(),
		ServicePath:   service.ServicePath,
	})
	if err != nil {
		return 0, errors.Wrap(err, "failed to suspend and assert connection")
	}
	return time.Duration(resp.ReconnectTime), nil
}

// Suspend suspends the DUT for wakeUpTimeout seconds through gRPC.
// This call will fail when the DUT wake up early. If the caller expects the DUT to
// wake up early, please use the Suspend gRPC to specify the detailed options.
func (cli *WifiClient) Suspend(ctx context.Context, wakeUpTimeout time.Duration) error {
	req := &wifi.SuspendRequest{
		WakeUpTimeout:  wakeUpTimeout.Nanoseconds(),
		CheckEarlyWake: true,
	}
	_, err := cli.ShillServiceClient.Suspend(ctx, req)
	if err != nil {
		return errors.Wrap(err, "failed to suspend")
	}
	return nil
}

// DisableMACRandomize disables MAC randomization on DUT if supported, this
// is useful for tests verifying probe requests from DUT.
// On success, a shortened context and cleanup function is returned.
func (cli *WifiClient) DisableMACRandomize(ctx context.Context) (shortenCtx context.Context, cleanupFunc func() error, retErr error) {
	// If MAC randomization setting is supported, disable MAC randomization
	// as we're filtering the packets with MAC address.
	if supResp, err := cli.ShillServiceClient.MACRandomizeSupport(ctx, &empty.Empty{}); err != nil {
		return ctx, nil, errors.Wrap(err, "failed to get if MAC randomization is supported")
	} else if supResp.Supported {
		resp, err := cli.ShillServiceClient.GetMACRandomize(ctx, &empty.Empty{})
		if err != nil {
			return ctx, nil, errors.Wrap(err, "failed to get MAC randomization setting")
		}
		if resp.Enabled {
			ctxRestore := ctx
			ctx, cancel := ctxutil.Shorten(ctx, time.Second)
			_, err := cli.ShillServiceClient.SetMACRandomize(ctx, &wifi.SetMACRandomizeRequest{Enable: false})
			if err != nil {
				return ctx, nil, errors.Wrap(err, "failed to disable MAC randomization")
			}
			// Restore the setting when exiting.
			restore := func() error {
				cancel()
				if _, err := cli.ShillServiceClient.SetMACRandomize(ctxRestore, &wifi.SetMACRandomizeRequest{Enable: true}); err != nil {
					return errors.Wrap(err, "failed to re-enable MAC randomization")
				}
				return nil
			}
			return ctx, restore, nil
		}
	}
	// Not supported or not enabled. No-op for these cases.
	return ctx, func() error { return nil }, nil
}

// SetWifiEnabled persistently enables/disables Wifi via shill.
func (cli *WifiClient) SetWifiEnabled(ctx context.Context, enabled bool) error {
	req := &wifi.SetWifiEnabledRequest{Enabled: enabled}
	_, err := cli.ShillServiceClient.SetWifiEnabled(ctx, req)
	return err
}

// GetWifiEnabled checks if Wifi is an enabled technology on shill.
// This call will wait for WiFi to appear in available technologies so we
// can get correct enabled setting.
func (cli *WifiClient) GetWifiEnabled(ctx context.Context) (bool, error) {
	res, err := cli.ShillServiceClient.GetWifiEnabled(ctx, &empty.Empty{})
	return res.Enabled, err
}

// SetPortalDetectionEnabled persistently enables/disables PortalDection via shill.
func (cli *WifiClient) SetPortalDetectionEnabled(ctx context.Context, enabled bool) error {
	req := &wifi.SetPortalDetectionEnabledRequest{Enabled: enabled}
	_, err := cli.ShillServiceClient.SetPortalDetectionEnabled(ctx, req)
	return err
}

// SetCaptivePortalList sets the CheckPortalList property value.
func (cli *WifiClient) SetCaptivePortalList(ctx context.Context, captivePortalList string) error {
	req := &wifi.SetCaptivePortalListRequest{CaptivePortalList: captivePortalList}
	_, err := cli.ShillServiceClient.SetCaptivePortalList(ctx, req)
	return err
}

// GetCaptivePortalList returns the CheckPortalList manager property value.
func (cli *WifiClient) GetCaptivePortalList(ctx context.Context) (string, error) {
	res, err := cli.ShillServiceClient.GetCaptivePortalList(ctx, &empty.Empty{})
	return res.CaptivePortalList, err
}

// TurnOffBgAndFgscan turns off the DUT's background and foreground scan, and
// returns a shortened ctx and a restoring function.
func (cli *WifiClient) TurnOffBgAndFgscan(ctx context.Context) (context.Context, func() error, error) {
	ctxForRestoreBgConfig := ctx
	ctx, cancel := ctxutil.Shorten(ctxForRestoreBgConfig, 2*time.Second)

	testing.ContextLog(ctx, "Disable the DUT's background and foreground scan")
	bgscanResp, err := cli.ShillServiceClient.GetBgscanConfig(ctx, &empty.Empty{})
	if err != nil {
		return ctxForRestoreBgConfig, nil, err
	}
	oldBgConfig := bgscanResp.Config

	// Setting long interval affects foreground scans as well as background
	// scans and setting it to 0 disables periodic scanning.
	turnOffBgAndFgConfig := wifi.BgscanConfig{
		Method:        shillconst.DeviceBgscanMethodNone,
		LongInterval:  0,
		ShortInterval: oldBgConfig.ShortInterval,
	}
	if _, err := cli.ShillServiceClient.SetBgscanConfig(ctx, &wifi.SetBgscanConfigRequest{Config: &turnOffBgAndFgConfig}); err != nil {
		return ctxForRestoreBgConfig, nil, err
	}

	return ctx, func() error {
		cancel()
		testing.ContextLogf(ctxForRestoreBgConfig, "Restore the DUT's background scan config: %s and foreground scan interval: %d",
			oldBgConfig, oldBgConfig.LongInterval)
		_, err := cli.ShillServiceClient.SetBgscanConfig(ctxForRestoreBgConfig, &wifi.SetBgscanConfigRequest{Config: oldBgConfig})
		return err
	}, nil
}

// TransitionFromEthernetAndRecover tests if the network is transitioned from ethernet to WiFi when ethernet is not available.
// The function will also recover the ethernet connection to ensure remote connection is back.
func (cli *WifiClient) TransitionFromEthernetAndRecover(ctx context.Context, ssid string) error {
	_, err := cli.ShillServiceClient.EthernetFailoverToWifiTest(ctx, &wifi.EthernetFailoverToWifiTestRequest{Ssid: ssid})
	return err
}

// GetNetworksForGeolocation returns geolocation cache
// Deprecated: use GetWiFiNetworksForGeolocation instead.
func (cli *WifiClient) GetNetworksForGeolocation(ctx context.Context) (*wifi.NetworksForGeolocation, error) {
	return nil, errors.New("GetNetworksForGeolocation is deprecated, use GetWiFiNetworksForGeolocation instead")
}

// GetWiFiNetworksForGeolocation returns WiFi geolocation cache
func (cli *WifiClient) GetWiFiNetworksForGeolocation(ctx context.Context) (*wifi.NetworksForGeolocation, error) {
	res, err := cli.ShillServiceClient.GetWiFiNetworksForGeolocation(ctx, &empty.Empty{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get WiFi geolocation information")
	}
	wifiNetworks, ok := res.Networks[shillconst.GeoWifiAccessPointsProperty]
	if !ok {
		return nil, errors.Errorf("GetWiFiNetworksForGeolocation has no %s", shillconst.GeoWifiAccessPointsProperty)
	}
	return wifiNetworks, err
}

// RequestScan requests shill to trigger a wpa_supplicant scan on WiFi device
func (cli *WifiClient) RequestScan(ctx context.Context) error {
	_, err := cli.ShillServiceClient.RequestScan(ctx, &empty.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to request wpa_supplicant scan")
	}
	return nil
}

// TriggerIntelFirmwareDump triggers firmware dump
func (cli *WifiClient) TriggerIntelFirmwareDump(ctx context.Context) error {
	_, err := cli.ShillServiceClient.TriggerIntelFirmwareDump(ctx, &empty.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to trigger firmware dump")
	}
	return nil
}

// SetBSSIDRequested sets the BSSIDRequested service property in shill
func (cli *WifiClient) SetBSSIDRequested(ctx context.Context, bssid string) error {
	service, err := cli.ShillServiceClient.SelectedService(ctx, &empty.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get selected service")
	}
	if _, err := cli.ShillServiceClient.SetBSSIDRequested(ctx, &wifi.SetBSSIDRequestedRequest{
		ServicePath: service.ServicePath,
		Bssid:       bssid,
	}); err != nil {
		return errors.Wrap(err, "failed to set BSSIDRequested")
	}
	return nil
}

// GetRequestScanTypeProperty returns the WiFi.RequestScanType manager property value
func (cli *WifiClient) GetRequestScanTypeProperty(ctx context.Context) (string, error) {
	res, err := cli.ShillServiceClient.GetRequestScanTypeProperty(ctx, &empty.Empty{})
	if err != nil {
		return "", errors.Wrap(err, "failed to get WiFi.RequestScanType manager property")
	}
	return res.ScanType, nil
}

// SetRequestScanTypeProperty sets the WiFi.RequestScanType manager property value
func (cli *WifiClient) SetRequestScanTypeProperty(ctx context.Context, scanType string) error {
	_, err := cli.ShillServiceClient.SetRequestScanTypeProperty(ctx, &wifi.SetRequestScanTypePropertyRequest{ScanType: scanType})
	if err != nil {
		return errors.Wrapf(err, "failed to set WiFi.RequestScanType manager property with value %s", scanType)
	}
	return nil
}
