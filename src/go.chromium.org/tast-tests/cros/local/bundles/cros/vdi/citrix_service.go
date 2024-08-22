// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vdi

import (
	"context"
	"path/filepath"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/common"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	vdiApps "go.chromium.org/tast-tests/cros/local/vdi/apps"
	"go.chromium.org/tast-tests/cros/local/vdi/apps/citrix"
	"go.chromium.org/tast-tests/cros/services/cros/vdi"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/grpc"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			svc := CitrixService{s: s, sharedObject: common.SharedObjectsForServiceSingleton}
			vdi.RegisterCitrixServiceServer(srv, &svc)
		},
	})
}

// CitrixService implements tast.cros.vdi.CitrixService.
type CitrixService struct {
	s            *testing.ServiceState
	sharedObject *common.SharedObjectsForService
	tconn        *chrome.TestConn
	kb           *input.KeyboardEventWriter
	ud           *uidetection.Context
	vdiConnector vdiApps.VDIInt
	dataPath     func(string) string
}

// NewCitrix creates a new instance of Citrix and launches the Citrix app.
func (c *CitrixService) NewCitrix(ctx context.Context, req *vdi.NewCitrixRequest) (*empty.Empty, error) {
	if c.sharedObject.Chrome == nil {
		var err error
		creds := chrome.Creds{User: req.OtaUsername, Pass: req.OtaPassword}
		c.sharedObject.Chrome, err = chrome.New(ctx,
			chrome.GAIALogin(creds),
			chrome.GAIAEnterpriseEnroll(creds),
			chrome.ProdPolicy(),
			chrome.KeepEnrollment(),
			chrome.ExtraArgs("--force-devtools-available"),
		)
		if err != nil {
			return nil, err
		}
	}

	c.dataPath = func(s string) string {
		return filepath.Join(req.DataPath, s)
	}

	var err error
	c.tconn, err = c.sharedObject.Chrome.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Test API connection")
	}

	c.kb, err = input.Keyboard(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open keyboard")
	}
	c.ud = uidetection.NewDefault(c.tconn).WithScreenshotStrategy(uidetection.ImmediateScreenshot)

	c.vdiConnector = &citrix.Connector{}
	c.vdiConnector.Init(c.dataPath, c.tconn, c.ud, c.kb)

	if _, err := c.OpenCitrix(ctx, &empty.Empty{}); err != nil {
		return nil, err
	}

	return &empty.Empty{}, nil
}

// LoginCitrix logins the Citrix app and connects to the desktop.
func (c *CitrixService) LoginCitrix(ctx context.Context, req *vdi.LoginCitrixRequest) (*empty.Empty, error) {
	if err := c.vdiConnector.Login(
		ctx,
		&vdiApps.VDILoginConfig{
			Username: req.Username,
			Password: req.Password,
		}); err != nil {
		return nil, errors.Wrap(err, "failed to login to the Citrix application")
	}

	if err := citrix.WaitForDesktop(c.ud, c.dataPath)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to wait for desktop")
	}

	// Set the resolution to ensure that uidetection can detect the following UI.
	if err := citrix.SwitchResolution(c.ud, citrix.ResolutionAutoFitScreen, c.dataPath)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to switch resolution")
	}

	return &empty.Empty{}, nil
}

// OpenCitrix launches the Citrix app.
func (c *CitrixService) OpenCitrix(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	testing.ContextLog(ctx, "Waiting for apps to be installed before launching")
	if err := ash.WaitForChromeAppInstalled(ctx, c.tconn, apps.Citrix.ID, 2*time.Minute); err != nil {
		return nil, errors.Wrap(err, "failed to wait for apps.Citrix to install")
	}

	testing.ContextLog(ctx, "Starting Citrix app")
	if err := apps.Launch(ctx, c.tconn, apps.Citrix.ID); err != nil {
		return nil, errors.Wrap(err, "failed to launch Citrix app")
	}
	if err := ash.WaitForApp(ctx, c.tconn, apps.Citrix.ID, time.Minute); err != nil {
		return nil, errors.Wrap(err, "the Citrix app did not appear in shelf after launch")
	}

	return &empty.Empty{}, nil
}

// CloseCitrix closes the Citrix app.
func (c *CitrixService) CloseCitrix(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	testing.ContextLog(ctx, "VDI: Closing all windows")
	// Ensure that there are no windows open.
	if err := ash.CloseAllWindows(ctx, c.tconn); err != nil {
		return nil, errors.Wrap(err, "failed to close all windows")
	}

	if c.kb != nil {
		if err := c.kb.Close(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to close keyboard")
		}
		c.kb = nil
	}
	return &empty.Empty{}, nil
}

// ConnectUSBDevice connects USB device.
func (c *CitrixService) ConnectUSBDevice(ctx context.Context, req *vdi.ConnectUSBDeviceRequest) (*empty.Empty, error) {
	if err := citrix.ConnectUSBDevice(c.kb, c.ud, c.dataPath, req.DeviceName)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to connect USB device")
	}

	return &empty.Empty{}, nil
}

// OpenCitrixApp opens app in Citrix.
func (c *CitrixService) OpenCitrixApp(ctx context.Context, req *vdi.OpenCitrixAppRequest) (*empty.Empty, error) {
	appName := req.AppName
	if err := citrix.OpenApp(c.ud, c.dataPath, appName, req.AppTitle)(ctx); err != nil {
		return nil, errors.Wrapf(err, "failed to open %s app", appName)
	}
	return &empty.Empty{}, nil
}

// CloseCitrixApp closes app in Citrix.
func (c *CitrixService) CloseCitrixApp(ctx context.Context, req *vdi.CloseCitrixAppRequest) (*empty.Empty, error) {
	if err := citrix.CloseApp(ctx, c.tconn, c.kb, c.ud, req.AppTitle); err != nil {
		return nil, err
	}

	return &empty.Empty{}, nil
}

// SetupFootPedalTest sets up the foot pedal test environment.
func (c *CitrixService) SetupFootPedalTest(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if err := citrix.SetupFootPedalTest(c.kb, c.ud, c.dataPath)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to set up foot pedal test")
	}

	return &empty.Empty{}, nil
}

// VerifyFootPedalButtonPressed verifies if the foot pedal button is pressed.
func (c *CitrixService) VerifyFootPedalButtonPressed(ctx context.Context, req *vdi.VerifyFootPedalButtonPressedRequest) (*empty.Empty, error) {
	var button citrix.FootPedalButton
	switch req.Button {
	case vdi.FootPedalButton_CENTER:
		button = citrix.FootPedalButtonCenter
	case vdi.FootPedalButton_LEFT:
		button = citrix.FootPedalButtonLeft
	case vdi.FootPedalButton_RIGHT:
		button = citrix.FootPedalButtonRight
	case vdi.FootPedalButton_TOP:
		button = citrix.FootPedalButtonTop
	}

	if err := citrix.VerifyFootPedalButtonPressed(c.ud, button)(ctx); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to verify foot pedal button pressed")
	}

	return &empty.Empty{}, nil
}
