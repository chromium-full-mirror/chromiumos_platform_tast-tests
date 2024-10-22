// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/firewall"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/services/cros/network"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			network.RegisterAllowlistServiceServer(srv, &AllowlistService{s: s})
		},
	})
}

// AllowlistService implements the tast.cros.network.AllowlistService gRPC service.
type AllowlistService struct {
	s *testing.ServiceState

	cr *chrome.Chrome
}

func (a *AllowlistService) SetupFirewall(ctx context.Context, req *network.SetupFirewallRequest) (*empty.Empty, error) {
	params := firewall.CreateFirewallParams{
		AllowPorts:      []string{fmt.Sprint(req.AllowedPort)},
		AllowInterfaces: []string{"arc_ns+"},
		AllowProtocols:  []string{"tcp"},
		// Drop http and https traffic.
		BlockPorts:     []string{"80", "443"},
		BlockProtocols: []string{"tcp", "udp"},
		Timeout:        3 * time.Second,
	}
	if err := firewall.CreateFirewall(ctx, params); err != nil {
		return nil, err
	}
	return &empty.Empty{}, nil
}

func (a *AllowlistService) VerifyFirewallWorks(ctx context.Context) (*empty.Empty, error) {
	if a.cr == nil {
		return nil, errors.New("Please start a new Chrome instance that uses the firewall by calling GaiaLogin()")
	}
	tconn, err := a.cr.TestAPIConn(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Failed to create test API connection: ", err)
	}

	blockedWebsiteExample := "https://www.example.org/"
	conn, err := a.cr.NewConn(ctx, blockedWebsiteExample)
	if err != nil {
		return nil, errors.Wrapf(err, "error when testing connection to %s", blockedWebsiteExample)
	}
	defer conn.Close()

	ui := uiauto.New(tconn)
	if err := ui.WaitUntilExists(nodewith.NameRegex(regexp.MustCompile("NET::ERR_CERT_AUTH")).First())(ctx); err != nil {
		testing.ContextLog(ctx, "Expected error on webpage due to firewall was not found: ", err)
		return nil, err
	}

	return &empty.Empty{}, nil
}

func (a *AllowlistService) GaiaLogin(ctx context.Context, req *network.GaiaLoginRequest) (*empty.Empty, error) {
	cr, err := chrome.New(
		ctx,
		chrome.GAIAEnterpriseEnroll(chrome.Creds{User: req.Username, Pass: req.Password}),
		chrome.GAIALogin(chrome.Creds{User: req.Username, Pass: req.Password}),
		chrome.ARCSupported(),
		chrome.RemoveNotification(false),
		chrome.ExtraArgs("--proxy-server=http://"+req.ProxyHostAndPort))
	if err != nil {
		return nil, err
	}
	a.cr = cr

	testing.ContextLog(ctx, "Login finished")
	return &empty.Empty{}, nil
}

// CheckArcAppInstalled verifies that ARC provisioning and installing ARC apps by checking that force installed apps are successfully installed.
func (a *AllowlistService) CheckArcAppInstalled(ctx context.Context, req *network.CheckArcAppInstalledRequest) (*empty.Empty, error) {
	if a.cr == nil {
		return nil, errors.New("Please start a new Chrome instance that uses the firewall by calling GaiaLogin()")
	}

	tconn, err := a.cr.TestAPIConn(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Failed to create test API connection: ", err)
		return nil, err
	}

	var isGmailInstalled bool
	if isGmailInstalled, err = ash.ChromeAppInstalled(ctx, tconn, apps.Gmail.ID); err != nil {
		testing.ContextLog(ctx, "Error requesting gmail install status")
		return nil, err
	}
	if isGmailInstalled == true {
		testing.ContextLog(ctx, "Gmail app is already installed, failing test")
		return nil, errors.New("Gmail app is already installed")
	}

	testing.ContextLog(ctx, "Waiting for app store and gmail app")
	if err := ash.WaitForChromeAppInstalled(ctx, tconn, apps.Gmail.ID, 3*time.Minute); err != nil {
		testing.ContextLog(ctx, "Failed to wait for app to install: ", err)
		return nil, err
	}

	testing.ContextLog(ctx, "Gmail app was found")
	return &empty.Empty{}, nil
}

// CheckExtensionInstalled verifies that specified extension is installed by performing a full-text search on the chrome://extensions page.
func (a *AllowlistService) CheckExtensionInstalled(ctx context.Context, req *network.CheckExtensionInstalledRequest) (*empty.Empty, error) {
	// Connect to Test API to use it with the UI library.
	tconn, err := a.cr.TestAPIConn(ctx)
	if err != nil {
		return nil, err
	}

	const extensionURL = "chrome://extensions"

	sconn, err := a.cr.NewConn(ctx, extensionURL)
	if err != nil {
		return nil, err
	}
	defer sconn.Close()

	desc := nodewith.Name(req.ExtensionTitle).Role(role.StaticText)
	ui := uiauto.New(tconn).WithTimeout(3 * time.Minute)

	if err := ui.WaitUntilExists(desc)(ctx); err != nil {
		return nil, err
	}

	return &empty.Empty{}, nil
}

func (a *AllowlistService) Close(ctx context.Context) (*empty.Empty, error) {
	if a.cr != nil {
		a.cr.Close(ctx)
	}
	return &empty.Empty{}, nil
}
