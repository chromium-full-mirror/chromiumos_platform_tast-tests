// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rollback

import (
	"context"
	"encoding/json"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/common/networkui/netconfigtypes"
	"go.chromium.org/tast-tests/cros/common/rollback"
	"go.chromium.org/tast-tests/cros/local/chrome"
	nc "go.chromium.org/tast-tests/cros/local/networkui/netconfig"
	rpb "go.chromium.org/tast-tests/cros/services/cros/rollback"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			rpb.RegisterEnterpriseRollbackServiceServer(srv, &EnterpriseRollbackService{s: s})
		},
	})
}

// EnterpriseRollbackService implements tast.cros.rollback.EnterpriseRollbackService.
type EnterpriseRollbackService struct {
	s          *testing.ServiceState
	ash        *chrome.Chrome
	networkAPI *nc.CrosNetworkConfig
	ownership  *rpb.Ownership
}

// Connect sets up connection to ash Chrome and network API. Calling this function is a prerequisite to call other functions.
func (e *EnterpriseRollbackService) Connect(ctx context.Context, sessionState *rpb.SessionState) (_ *empty.Empty, returnErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if e.ash != nil || e.networkAPI != nil {
		return nil, errors.New("not connecting because Chrome or network API already set up")
	}

	// Clean up any partial setup if an error occurs.
	defer func(ctx context.Context) {
		if returnErr != nil {
			e.CloseConnections(ctx, &empty.Empty{})
		}
	}(cleanupCtx)

	ash, err := getAsh(ctx, sessionState)
	if err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to connect to ash")
	}

	networkAPI, err := getNetworkConfigAPI(ctx, ash, sessionState.Ownership)
	if err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to connect to network API")
	}

	e.ownership = &sessionState.Ownership
	e.ash = ash
	e.networkAPI = networkAPI
	return &empty.Empty{}, nil
}

// CloseConnections closes ash Chrome and network API connections. This function is resilient to any of the connections not existing.
func (e *EnterpriseRollbackService) CloseConnections(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	var lastError error
	if e.networkAPI != nil {
		lastError = e.networkAPI.Close(ctx)

	}
	e.networkAPI = nil
	if e.ash != nil {

		e.ownership = nil
		lastError = e.ash.Close(ctx)
	}
	e.ash = nil
	return &empty.Empty{}, lastError
}

// Login can be called after connecting in OOBE and continues to login as normal user.
func (e *EnterpriseRollbackService) Login(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if *e.ownership != rpb.Ownership_OOBE && *e.ownership != rpb.Ownership_AUTO_ENROLLING {
		return nil, errors.New("cannot login because we are not in OOBE")
	}

	// Close JS API connection in OOBE before login.
	if e.networkAPI != nil {
		if err := e.networkAPI.Close(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to close cros network config api")
		}
		e.networkAPI = nil
	}

	// When auto-enrolling, we need Wait for the signin OOBE page to be shown.
	// Waiting if enrollment is not expected causes the test to fail.
	if *e.ownership == rpb.Ownership_AUTO_ENROLLING {
		_, err := e.ash.WaitForOOBEConnectionWithPrefix(ctx, "chrome://oobe/gaia-signin")
		if err != nil {
			return nil, errors.Wrap(err, "could not find OOBE connection for gaia sign in")
		}
		testing.ContextLog(ctx, "Login page found after re-enrollment, logging in now")
	}

	if err := e.ash.ContinueLogin(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to login as normal user after rollback")
	}
	if *e.ownership == rpb.Ownership_AUTO_ENROLLING {
		e.ownership = rpb.Ownership_ENROLLED_LOGGED_IN.Enum()
	} else {
		e.ownership = rpb.Ownership_LOGGED_IN.Enum()
	}

	networkAPI, err := nc.CreateLoggedInCrosNetworkConfig(ctx, e.ash)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get cros network config api after logged in")
	}
	e.networkAPI = networkAPI

	return &empty.Empty{}, nil
}

func getNetworkConfigAPI(ctx context.Context, ash *chrome.Chrome, ownership rpb.Ownership) (*nc.CrosNetworkConfig, error) {
	if ownership == rpb.Ownership_LOGGED_IN || ownership == rpb.Ownership_ENROLLED_LOGGED_IN {
		return nc.CreateLoggedInCrosNetworkConfig(ctx, ash)
	}
	return nc.CreateOobeCrosNetworkConfig(ctx, ash)
}

func getAsh(ctx context.Context, sessionState *rpb.SessionState) (*chrome.Chrome, error) {
	return chrome.New(ctx, getAshConfig(sessionState)...)
}

func getAshConfig(sessionState *rpb.SessionState) []chrome.Option {
	switch sessionState.Ownership {
	case rpb.Ownership_OOBE:
		return []chrome.Option{
			chrome.DeferLogin(),
			chrome.ExtraArgs("--enterprise-force-manual-enrollment")}
	case rpb.Ownership_ENROLLED_LOGGED_IN:
		return []chrome.Option{
			chrome.DMSPolicy(sessionState.LoginData.DmserverUrl),
			chrome.ExtraArgs("--enterprise-force-manual-enrollment"),
			chrome.GAIAEnterpriseEnroll(chrome.Creds{User: sessionState.LoginData.Username, Pass: sessionState.LoginData.Password}),
			chrome.GAIALogin(chrome.Creds{User: sessionState.LoginData.Username, Pass: sessionState.LoginData.Password})}
	case rpb.Ownership_LOGGED_IN:
		return []chrome.Option{chrome.TryReuseSession()}
	case rpb.Ownership_AUTO_ENROLLING:
		return []chrome.Option{
			chrome.DeferLogin(),
			chrome.DMSPolicy(sessionState.LoginData.DmserverUrl),
			chrome.GAIALogin(chrome.Creds{User: sessionState.LoginData.Username, Pass: sessionState.LoginData.Password})}
	}
	return []chrome.Option{}
}

// SetUpNetworks sets various networks supported by rollback automatically.
func (e *EnterpriseRollbackService) SetUpNetworks(ctx context.Context, req *empty.Empty) (*rpb.SetUpNetworksResponse, error) {
	if e.networkAPI == nil {
		return nil, errors.New("rollback service has no connection to network API")
	}

	testing.ContextLog(ctx, "setting up networks supported by rollback")

	// Set up the supported networks.
	var networks []*rpb.NetworkInformation
	for _, nw := range rollback.SupportedNetworks {
		nwInfo, err := setUpNetwork(ctx, e.networkAPI, nw.Config)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to set up %s network", nw.Type)
		}
		networks = append(networks, nwInfo)
	}

	networksResponse := &rpb.SetUpNetworksResponse{
		Networks: networks,
	}

	testing.ContextLogf(ctx, "Networks set: %s ", string(networksResponse.String()))
	return networksResponse, nil
}

// setUpNetwork sets up a network configuration on the device.
func setUpNetwork(ctx context.Context, api *nc.CrosNetworkConfig, properties netconfigtypes.ConfigProperties) (*rpb.NetworkInformation, error) {
	guid, err := api.ConfigureNetwork(ctx, properties, true)
	if err != nil {
		return nil, errors.Wrap(err, "failed to configure network")
	}
	networkResponse := &rpb.NetworkInformation{
		Guid: guid,
	}
	return networkResponse, nil
}

// VerifyNetworks verifies that networks were preserved during rollback for tests without enrollment. It may be called during OOBE or after login.
func (e *EnterpriseRollbackService) VerifyNetworks(ctx context.Context, request *rpb.VerifyNetworksRequest) (*rpb.VerifyNetworksResponse, error) {
	if e.networkAPI == nil {
		return nil, errors.New("rollback service has no connection to network API")
	}

	testing.ContextLogf(ctx, "Verifying preservation of the following networks in %v: %v", ownershipToString(request.Ownership), request.Networks)

	response := &rpb.VerifyNetworksResponse{
		Successful:          true,
		VerificationDetails: "",
	}

	for idx, networkInfo := range request.Networks {
		guid := networkInfo.Guid
		managedProperties, err := e.networkAPI.GetManagedProperties(ctx, guid)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get managed properties for guid %s", guid)
		}

		// Retrieve corresponding supported network configuration.
		// It is assumed that the guid are received in the same order they were set,
		// so we use the idx to identify which is the corresponding network.
		nwID := rollback.ConfigID(idx)
		preservedNw, err := rollback.VerifyNetwork(ctx, nwID, managedProperties)
		if err != nil {
			return nil, errors.Wrap(err, "failed to verify network")
		}

		if !preservedNw {
			response.Successful = false
			response.VerificationDetails += rollback.SupportedNetworks[nwID].Type + " network was not preserved;"
		}
	}

	return response, nil
}

func ownershipToString(ownership rpb.Ownership) string {
	switch ownership {
	case rpb.Ownership_OOBE:
		return "staying in OOBE"
	case rpb.Ownership_ENROLLED_LOGGED_IN:
		return "enrolled and logged in"
	case rpb.Ownership_LOGGED_IN:
		return "logged in"
	case rpb.Ownership_AUTO_ENROLLING:
		return "going through auto enrollment but staying in OOBE"
	}
	return "unkown"
}

func (e *EnterpriseRollbackService) GetNetworkProperties(ctx context.Context, request *rpb.GetNetworkPropertiesRequest) (*rpb.GetNetworkPropertiesResponse, error) {
	if e.networkAPI == nil {
		return nil, errors.New("Rollback service has no connection to network API")
	}

	testing.ContextLog(ctx, "Requesting network properties from the API")

	properties, err := e.networkAPI.GetManagedProperties(ctx, request.Guid)
	if err == nc.ErrNetworkNotFound {
		testing.ContextLog(ctx, "Requested properties for non-existent network: ", request.Guid)
		return &rpb.GetNetworkPropertiesResponse{}, nil
	}
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get managed properties for GUID %v", request.Guid)
	}

	serializedProperties, err := json.Marshal(properties)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to serialize %v", properties)
	}

	return &rpb.GetNetworkPropertiesResponse{Properties: &rpb.ManagedProperties{SerializedData: serializedProperties}}, nil
}
