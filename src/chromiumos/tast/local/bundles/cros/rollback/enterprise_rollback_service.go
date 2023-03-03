// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rollback

import (
	"context"

	"google.golang.org/grpc"

	"chromiumos/tast/common/network/netconfigtypes"
	"chromiumos/tast/common/rollback"
	"chromiumos/tast/local/chrome"
	nc "chromiumos/tast/local/network/netconfig"
	rpb "chromiumos/tast/services/cros/rollback"

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
	s *testing.ServiceState
}

// SetUpNetworks sets up a series of network configuration on the device that
// are supported by rollback.
// The device needs to be in a state so that chrome://network may be opened.
func (e *EnterpriseRollbackService) SetUpNetworks(ctx context.Context, request *rpb.SetUpNetworksRequest) (*rpb.SetUpNetworksResponse, error) {
	testing.ContextLog(ctx, "setting up networks supported by rollback")
	// Open chrome and create a connection to the network configuration api.
	// This is needed to set up each network without having to create a connection
	// each time.
	cr, err := chrome.New(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start Chrome")
	}
	defer cr.Close(ctx)

	api, err := nc.CreateLoggedInCrosNetworkConfig(ctx, cr)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get cros network config api")
	}
	defer api.Close(ctx)

	// Set up the supported networks.
	var networks []*rpb.NetworkInformation
	for _, nw := range rollback.SupportedNetworks {
		nwInfo, err := setUpNetwork(ctx, api, nw.Config)
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

// verifyNetworks checks the networks set are the expected ones.
func verifyNetworks(ctx context.Context, networks []*rpb.NetworkInformation, api *nc.CrosNetworkConfig) (*rpb.VerifyRollbackResponse, error) {
	response := &rpb.VerifyRollbackResponse{
		Successful:          true,
		VerificationDetails: "",
	}

	for idx, networkInfo := range networks {
		// Obtain properties of network set.
		guid := networkInfo.Guid
		managedProperties, err := api.GetManagedProperties(ctx, guid)
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

// VerifyRollback verifies the previously set-up networks exists during OOBE,
// logs in as a normal user and verifies the networks again.
// VerifyRollbackRequest needs to contain the unchanged NetworkInformation from
// SetUpNetworksResponse.
func (e *EnterpriseRollbackService) VerifyRollback(ctx context.Context, request *rpb.VerifyRollbackRequest) (*rpb.VerifyRollbackResponse, error) {
	// Chrome would send an auto re-enrollment request to the real DMServer
	// which will fail because the device wasn't enrolled at all.
	// Try to prevent that by setting DMServer URL to nonsense.
	cr, err := chrome.New(ctx, chrome.DMSPolicy("do-not-call-any-dmserver"), chrome.DeferLogin())
	if err != nil {
		return nil, errors.Wrap(err, "failed to restart Chrome for testing after rollback")
	}
	defer cr.Close(ctx)

	// Verify network configuration during OOBE.
	apiOOBE, err := nc.CreateOobeCrosNetworkConfig(ctx, cr)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get cros network config api during OOBE")
	}
	testing.ContextLogf(ctx, "Verify preservation of networks during OOBE: %s ", request.Networks)
	if response, err := verifyNetworks(ctx, request.Networks, apiOOBE); err != nil {
		return nil, errors.Wrap(err, "failed to verify networks during OOBE")
	} else if !response.Successful {
		// The verification is unsuccessful. Finish here and return the response.
		return response, nil
	}
	// Close JS API connection in the OOBE before login.
	if err := apiOOBE.Close(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to close cros network config api")
	}

	if err := cr.ContinueLogin(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to login as normal user after rollback")
	}

	apiLoggedIn, err := nc.CreateLoggedInCrosNetworkConfig(ctx, cr)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get cros network config api after logged in")
	}
	defer apiLoggedIn.Close(ctx)

	// Verify network configuration after login.
	testing.ContextLogf(ctx, "Verify preservation of networks after login: %s ", request.Networks)
	response, err := verifyNetworks(ctx, request.Networks, apiLoggedIn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to verify networks")
	}

	return response, nil
}
