// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package peripherals

import (
	"context"
	"path/filepath"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/common"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	vdiApps "go.chromium.org/tast-tests/cros/local/vdi/apps"
	"go.chromium.org/tast-tests/cros/local/vdi/apps/citrix"
	"go.chromium.org/tast-tests/cros/services/cros/peripherals"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/grpc"
)

var citrixUsername = testing.RegisterVarString(
	"peripherals.citrix_username",
	"",
	"The username of Citrix app",
)
var citrixPassword = testing.RegisterVarString(
	"peripherals.citrix_password",
	"",
	"The password of Citrix app",
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			svc := PeriphService{s: s, sharedObject: common.SharedObjectsForServiceSingleton}
			peripherals.RegisterPeriphServiceServer(srv, &svc)
		},
	})
}

// PeriphService implements tast.cros.peripherals.PeriphService.
type PeriphService struct {
	s            *testing.ServiceState
	sharedObject *common.SharedObjectsForService
	tconn        *chrome.TestConn
	kb           *input.KeyboardEventWriter
	ud           *uidetection.Context
	vdiConnector vdiApps.VDIInt
	dataPath     func(string) string
	signaturePad citrix.SignaturePad
	bounds       coords.Rect
	login        bool
}

// NewCitrix creates a new instance of Citrix and launches the Citrix app.
func (p *PeriphService) NewCitrix(ctx context.Context, req *peripherals.NewCitrixRequest) (*empty.Empty, error) {
	if p.sharedObject.Chrome == nil {
		var err error
		creds := chrome.Creds{User: req.OtaUsername, Pass: req.OtaPassword}
		p.sharedObject.Chrome, err = chrome.New(ctx,
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

	p.dataPath = func(s string) string {
		return filepath.Join(req.DataPath, s)
	}

	var err error
	p.tconn, err = p.sharedObject.Chrome.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Test API connection")
	}

	p.kb, err = input.Keyboard(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open keyboard")
	}
	p.ud = uidetection.NewDefault(p.tconn).WithScreenshotStrategy(uidetection.ImmediateScreenshot)

	p.vdiConnector = &citrix.Connector{}
	p.vdiConnector.Init(p.dataPath, p.tconn, p.ud, p.kb)

	if _, err := p.OpenCitrix(ctx, &empty.Empty{}); err != nil {
		return nil, err
	}

	return &empty.Empty{}, nil
}

// LoginCitrix logins the Citrix app and connects to the desktop.
func (p *PeriphService) LoginCitrix(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	username := citrixUsername.Value()
	if username == "" {
		return nil, errors.Errorf("required variable %q not supplied via -var or -varsfile", citrixUsername.Name())
	}
	password := citrixPassword.Value()
	if password == "" {
		return nil, errors.Errorf("required variable %q not supplied via -var or -varsfile", citrixPassword.Name())
	}

	if err := p.vdiConnector.Login(
		ctx,
		&vdiApps.VDILoginConfig{
			Username: username,
			Password: password,
		}); err != nil {
		return nil, errors.Wrap(err, "failed to login to the Citrix application")
	}
	p.login = true
	if err := citrix.EnterDesktop(p.tconn, p.ud, p.dataPath)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to enter desktop")
	}

	return &empty.Empty{}, nil
}

// OpenCitrix launches the Citrix app.
func (p *PeriphService) OpenCitrix(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	testing.ContextLog(ctx, "Waiting for apps to be installed before launching")
	if err := ash.WaitForChromeAppInstalled(ctx, p.tconn, apps.Citrix.ID, 2*time.Minute); err != nil {
		return nil, errors.Wrap(err, "failed to wait for apps.Citrix to install")
	}

	testing.ContextLog(ctx, "Starting Citrix app")
	if err := apps.Launch(ctx, p.tconn, apps.Citrix.ID); err != nil {
		return nil, errors.Wrap(err, "failed to launch Citrix app")
	}
	if err := ash.WaitForApp(ctx, p.tconn, apps.Citrix.ID, time.Minute); err != nil {
		return nil, errors.Wrap(err, "the Citrix app did not appear in shelf after launch")
	}

	return &empty.Empty{}, nil
}

// CloseCitrix closes the Citrix app.
func (p *PeriphService) CloseCitrix(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if p.login {
		testing.ContextLog(ctx, "VDI: Log off from Citrix desktop")
		if err := citrix.LogOff(p.ud, p.dataPath)(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to log off from Citrix desktop")
		}
	}
	p.login = false

	testing.ContextLog(ctx, "VDI: Closing all windows")
	// Ensure that there are no windows open.
	if err := ash.CloseAllWindows(ctx, p.tconn); err != nil {
		return nil, errors.Wrap(err, "failed to close all windows")
	}

	if p.kb != nil {
		if err := p.kb.Close(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to close keyboard")
		}
		p.kb = nil
	}
	return &empty.Empty{}, nil
}

// ConnectUSBDeviceInCitrix connects USB device in Citrix.
func (p *PeriphService) ConnectUSBDeviceInCitrix(ctx context.Context, req *peripherals.ConnectUSBDeviceInCitrixRequest) (*empty.Empty, error) {
	if err := citrix.ConnectUSBDevice(p.kb, p.ud, p.dataPath, req.DeviceName)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to connect USB device")
	}

	return &empty.Empty{}, nil
}

// OpenCitrixApp opens app in Citrix.
func (p *PeriphService) OpenCitrixApp(ctx context.Context, req *peripherals.OpenCitrixAppRequest) (*empty.Empty, error) {
	appName := req.AppName
	appIcon := req.AppIcon
	appTitle := req.AppTitle
	if appIcon != "" {
		if err := citrix.OpenAppByIcon(p.ud, p.dataPath, appIcon, appTitle)(ctx); err != nil {
			return nil, errors.Wrapf(err, "failed to open %s app", appName)
		}

	} else {
		if err := citrix.OpenApp(p.ud, p.dataPath, appName, appTitle)(ctx); err != nil {
			return nil, errors.Wrapf(err, "failed to open %s app", appName)
		}
	}
	return &empty.Empty{}, nil
}

// CloseCitrixApp closes app in Citrix.
func (p *PeriphService) CloseCitrixApp(ctx context.Context, req *peripherals.CloseCitrixAppRequest) (*empty.Empty, error) {
	if err := citrix.CloseApp(ctx, p.tconn, p.kb, p.ud, req.AppTitle); err != nil {
		return nil, err
	}

	return &empty.Empty{}, nil
}

// DeleteFile deletes file in Citrix desktop.
func (p *PeriphService) DeleteFile(ctx context.Context, req *peripherals.DeleteFileRequest) (*empty.Empty, error) {
	if err := uiauto.Combine("delete file",
		citrix.ShowDesktop(p.ud, p.dataPath),
		citrix.DeleteFile(p.ud, p.dataPath, req.FileName),
	)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to delete file")
	}

	return &empty.Empty{}, nil
}

// DeleteFileIfExists deletes file in Citrix if it exists.
func (p *PeriphService) DeleteFileIfExists(ctx context.Context, req *peripherals.DeleteFileRequest) (*empty.Empty, error) {
	if err := citrix.DeleteFileIfExists(p.ud, p.dataPath, req.FileName)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to delete file")
	}

	return &empty.Empty{}, nil
}

// SaveCropScreenshot saves the crop screenshot.
func (p *PeriphService) SaveCropScreenshot(ctx context.Context, req *peripherals.SaveCropScreenshotRequest) (*empty.Empty, error) {
	path := p.dataPath("")
	filePath := p.dataPath(req.FileName)
	if err := citrix.SaveCropScreenshot(p.sharedObject.Chrome, p.bounds, path, req.FileName)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to save crop screenshot")
	}

	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return nil, errors.New("failed to get output dir")
	}
	if err := testexec.CommandContext(ctx, "cp", filePath, outDir).Run(testexec.DumpLogOnError); err != nil {
		return nil, errors.Wrap(err, "failed to copy file to tast out dir")
	}

	return &empty.Empty{}, nil
}

// VerifyTwoImagesSimilarity verifies two images are the same or not.
func (p *PeriphService) VerifyTwoImagesSimilarity(ctx context.Context, req *peripherals.VerifyTwoImagesSimilarityRequest) (*empty.Empty, error) {
	if err := citrix.VerifyTwoImagesSimilarity(p.dataPath(""), req.FileName1, req.FileName2, req.ExpectedSame)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to verify two images are same or not")
	}
	return &empty.Empty{}, nil
}

// WaitUntilIconExists waits for the icon to exist.
func (p *PeriphService) WaitUntilIconExists(ctx context.Context, req *peripherals.WaitUntilIconExistsRequest) (*empty.Empty, error) {
	if err := citrix.WaitUntilIconExists(p.ud, p.dataPath(""), req.IconName)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to wait for icon")
	}
	return &empty.Empty{}, nil
}

// SetupFootPedalTest sets up the foot pedal test environment.
func (p *PeriphService) SetupFootPedalTest(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if err := citrix.SetupFootPedalTest(p.kb, p.ud, p.dataPath)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to set up foot pedal test")
	}

	return &empty.Empty{}, nil
}

// VerifyFootPedalButtonPressed verifies if the foot pedal button is pressed.
func (p *PeriphService) VerifyFootPedalButtonPressed(ctx context.Context, req *peripherals.VerifyFootPedalButtonPressedRequest) (*empty.Empty, error) {
	var button citrix.FootPedalButton
	switch req.Button {
	case peripherals.FootPedalButton_CENTER:
		button = citrix.FootPedalButtonCenter
	case peripherals.FootPedalButton_LEFT:
		button = citrix.FootPedalButtonLeft
	case peripherals.FootPedalButton_RIGHT:
		button = citrix.FootPedalButtonRight
	case peripherals.FootPedalButton_TOP:
		button = citrix.FootPedalButtonTop
	}

	if err := citrix.VerifyFootPedalButtonPressed(p.ud, button)(ctx); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to verify foot pedal button pressed")
	}

	return &empty.Empty{}, nil
}

// StartSignature starts signature.
func (p *PeriphService) StartSignature(ctx context.Context, req *peripherals.StartSignatureRequest) (*empty.Empty, error) {
	var err error
	switch citrix.AppName(req.AppName) {
	case citrix.ScriptelAppName:
		// Create a new ScriptelSignaturePad instance.
		p.signaturePad = citrix.NewScriptelSignaturePad(p.ud, p.kb, p.tconn, p.dataPath)
	case citrix.TopazAppName:
		// Create a new TopazSignaturePad instance.
		p.signaturePad = citrix.NewTopazSignaturePad(p.ud, p.kb, p.tconn, p.dataPath)
	}

	if err := p.signaturePad.StartSignature()(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to start signature")
	}

	p.bounds, err = p.signaturePad.GetCanvasBounds(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get the canvas bounds")
	}

	return &empty.Empty{}, nil
}

// SaveSignature saves the signature.
func (p *PeriphService) SaveSignature(ctx context.Context, req *peripherals.SignatureRequest) (*empty.Empty, error) {
	if err := p.signaturePad.SaveSignature(req.FileName)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to save signature")
	}
	return &empty.Empty{}, nil
}

// LoadSignature loads the signature.
func (p *PeriphService) LoadSignature(ctx context.Context, req *peripherals.SignatureRequest) (*empty.Empty, error) {
	if err := p.signaturePad.LoadSignature(req.FileName)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to load signature")
	}
	return &empty.Empty{}, nil
}

// ClearSignature clears the signature.
func (p *PeriphService) ClearSignature(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if err := p.signaturePad.ClearSignature()(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to clear signature")
	}
	return &empty.Empty{}, nil
}
