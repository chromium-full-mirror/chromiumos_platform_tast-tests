// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package peripherals

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	dictationcommon "go.chromium.org/tast-tests/cros/common/dictation"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/peripherals/dictation"
	s "go.chromium.org/tast-tests/cros/local/bundles/cros/peripherals/smartcard"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/common"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	vdiApps "go.chromium.org/tast-tests/cros/local/vdi/apps"
	"go.chromium.org/tast-tests/cros/local/vdi/apps/citrix"
	"go.chromium.org/tast-tests/cros/services/cros/peripherals"
	"go.chromium.org/tast/core/ctxutil"
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
var smartCardAdminUsername = testing.RegisterVarString(
	"peripherals.smart_card_admin_username",
	"",
	"Smart card administrator's username",
)
var smartCardAdminPassword = testing.RegisterVarString(
	"peripherals.smart_card_admin_password",
	"",
	"Smart card administrator's password",
)
var signinProfileTestExtensionManifestKey = testing.RegisterVarString(
	"peripherals.signinProfileTestExtensionManifestKey",
	"",
	"The manifest key of signin profile test extension",
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
	s                *testing.ServiceState
	sharedObject     *common.SharedObjectsForService
	tconn            *chrome.TestConn
	tLoginConn       *chrome.TestConn
	kb               *input.KeyboardEventWriter
	ud               *uidetection.Context
	dictationSupport *dictation.Support
	sc               *s.SmartCard
	vdiConnector     vdiApps.VDIInt
	dataPath         func(string) string
	signaturePad     citrix.SignaturePad
	bounds           coords.Rect
	login            bool
}

// NewCitrix creates a new instance of Citrix and launches the Citrix app.
func (p *PeriphService) NewCitrix(ctx context.Context, req *peripherals.NewCitrixRequest) (*empty.Empty, error) {
	if p.sharedObject.Chrome == nil {
		creds := chrome.Creds{User: req.OtaUsername, Pass: req.OtaPassword}
		if err := p.newChrome(ctx,
			chrome.GAIALogin(creds),
			chrome.GAIAEnterpriseEnroll(creds),
			chrome.ProdPolicy(),
			chrome.KeepEnrollment(),
			chrome.ExtraArgs("--force-devtools-available"),
		); err != nil {
			return nil, errors.Wrap(err, "failed to start chrome")
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
	if err := citrix.EnterDesktop(p.tconn)(ctx); err != nil {
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
		if err := citrix.LogOff(p.tconn)(ctx); err != nil {
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
	if err := citrix.ConnectUSBDevice(p.tconn)(ctx); err != nil {
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
		citrix.ShowDesktop(p.ud, p.dataPath, p.tconn),
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

// NewDictationSupport creates a new dictation support
func (p *PeriphService) NewDictationSupport(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if p.sharedObject.Chrome == nil {
		return nil, errors.New("chrome is nil")
	}
	var err error
	p.dictationSupport, err = dictation.NewSupport(ctx, p.sharedObject.Chrome)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create dictation support")
	}

	return &empty.Empty{}, nil
}

// CloseDictation closes the dictation support page.
func (p *PeriphService) CloseDictation(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if err := p.dictationSupport.Close(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to close the dictation support")
	}
	return &empty.Empty{}, nil
}

// ConnectToDictationDevice connects to the dictation device with the given device name.
func (p *PeriphService) ConnectToDictationDevice(ctx context.Context, req *peripherals.ConnectToDictationDeviceRequest) (*empty.Empty, error) {
	if err := p.dictationSupport.ConnectToDevice(req.DeviceName)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to connect to device")
	}
	return &empty.Empty{}, nil
}

// GetDictationDevices returns the dictation devices.
func (p *PeriphService) GetDictationDevices(ctx context.Context, req *empty.Empty) (*peripherals.GetDictationDevicesResponse, error) {
	devices, err := p.dictationSupport.Devices(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get devices")
	}
	deviceString, err := json.Marshal(devices)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal devices")
	}

	return &peripherals.GetDictationDevicesResponse{Devices: string(deviceString)}, nil
}

// WaitDictationNewEvent waits for new event and saves the last event message.
func (p *PeriphService) WaitDictationNewEvent(ctx context.Context, req *peripherals.WaitDictationNewEventRequest) (*empty.Empty, error) {
	if err := p.dictationSupport.WaitNewEvent(string(req.Event), time.Duration(req.Timeout)*time.Millisecond)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to wait new event")
	}
	return &empty.Empty{}, nil
}

// SetDictationEventMode sets the event mode to the given state.
func (p *PeriphService) SetDictationEventMode(ctx context.Context, req *peripherals.SetDictationEventModeRequest) (*empty.Empty, error) {
	if err := p.dictationSupport.SetEventMode(dictationcommon.EventMode(req.EventMode))(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to set event mode")
	}
	return &empty.Empty{}, nil
}

// GetDictationEventMode returns the dictation event mode.
func (p *PeriphService) GetDictationEventMode(ctx context.Context, req *empty.Empty) (*peripherals.GetDictationEventModeResponse, error) {
	eventMode, err := p.dictationSupport.EventMode(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get event mode")
	}
	return &peripherals.GetDictationEventModeResponse{EventMode: string(eventMode)}, nil
}

// SetDictationSimpleLEDState sets the simple LED state to the given state.
func (p *PeriphService) SetDictationSimpleLEDState(ctx context.Context, req *peripherals.SetDictationSimpleLEDStateRequest) (*empty.Empty, error) {
	if err := p.dictationSupport.SetSimpleLEDState(dictationcommon.SimpleLEDState(req.SimpleLedState))(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to set simple LED state")
	}
	return &empty.Empty{}, nil
}

// SetDictationLEDState sets the LED state with given index and mode.
func (p *PeriphService) SetDictationLEDState(ctx context.Context, req *peripherals.SetDictationLEDStateRequest) (*empty.Empty, error) {
	if err := p.dictationSupport.SetLEDState(dictationcommon.LEDIndex(req.LedIndex), dictationcommon.LEDMode(req.LedMode))(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to set LED state")
	}
	return &empty.Empty{}, nil
}

// NewSmartCard launches Chrome, creates a new SmartCard instance and
// navigates to the PIN entry page.
func (p *PeriphService) NewSmartCard(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	username := smartCardAdminUsername.Value()
	if username == "" {
		return nil, errors.Errorf("required variable %q not supplied via -var or -varsfile", smartCardAdminUsername.Name())
	}
	password := smartCardAdminPassword.Value()
	if password == "" {
		return nil, errors.Errorf("required variable %q not supplied via -var or -varsfile", smartCardAdminPassword.Name())
	}
	manifestKey := signinProfileTestExtensionManifestKey.Value()
	if manifestKey == "" {
		return nil, errors.Errorf("required variable %q not supplied via -var or -varsfile", signinProfileTestExtensionManifestKey.Name())
	}
	if err := p.newChrome(ctx,
		chrome.GAIAEnterpriseEnroll(chrome.Creds{User: username, Pass: password}),
		chrome.NoLogin(),
		chrome.ProdPolicy(),
		chrome.LoadSigninProfileExtension(manifestKey),
	); err != nil {
		return nil, errors.Wrap(err, "failed to start chrome")
	}

	cr := p.sharedObject.Chrome
	// When in OOBE, use SigninProfileTestAPIConn to create the test connection.
	var err error
	p.tLoginConn, err = cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create test API connection")
	}
	p.kb, err = input.Keyboard(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open keyboard")
	}
	p.sc = s.New(cr, p.tLoginConn, p.kb, username, password)
	if err := p.sc.EnterPINPage()(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to enter PIN page")
	}

	return &empty.Empty{}, nil
}

// CleanupSmartCard cleanups smart card by closing chrome.
func (p *PeriphService) CleanupSmartCard(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if p.sharedObject.Chrome == nil {
		return nil, errors.New("no active Chrome instance")
	}
	p.sharedObject.ChromeMutex.Lock()
	defer func() {
		p.sharedObject.Chrome = nil
		p.sharedObject.ChromeMutex.Unlock()
	}()

	if err := p.sharedObject.Chrome.Close(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to close Chrome")
	}
	return &empty.Empty{}, nil
}

// SetPIN sets PIN with given PIN code.
func (p *PeriphService) SetPIN(ctx context.Context, req *peripherals.SetPINRequest) (*peripherals.SetPINResponse, error) {
	const invalidPIN = "Invalid PIN."
	if err := p.sc.SetPIN(req.PinCode)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to set PIN")
	}
	ui := uiauto.New(p.tLoginConn)
	invalidPINText := nodewith.Name(invalidPIN).Role(role.StaticText)

	errMessage := ""
	if err := ui.WithTimeout(3 * time.Second).WaitUntilExists(invalidPINText)(ctx); err == nil {
		testing.ContextLog(ctx, "The error message 'Invalid PIN.' is displayed")
		errMessage = invalidPIN
	} else if err := p.waitUntilUserLogin(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to wait until user login")
	}

	return &peripherals.SetPINResponse{Err: errMessage}, nil
}

// SmartCardSignOut signs out smart card.
func (p *PeriphService) SmartCardSignOut(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if err := uiauto.Combine("sign out smart card",
		p.sc.SignOut(),
		p.updateChromeAndSmartCard,
	)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to sign out smart card")
	}

	return &empty.Empty{}, nil
}

// SmartCardSignIn signs in smart card.
func (p *PeriphService) SmartCardSignIn(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if err := p.sc.SignIn()(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to sign in smart card")
	}

	return &empty.Empty{}, nil
}

// ReAuthentication signs out and signs in with the smart card.
// If req.Offline is true, it disables ethernet before re-authentication.
func (p *PeriphService) ReAuthentication(ctx context.Context, req *peripherals.ReAuthenticationRequest) (*empty.Empty, error) {
	if _, err := p.SmartCardSignOut(ctx, &empty.Empty{}); err != nil {
		return nil, err
	}

	if req.Offline {
		var cancel context.CancelFunc
		cleanupCtx := ctx
		ctx, cancel = ctxutil.Shorten(ctx, 2*shill.EnableWaitTime+10*time.Second)
		defer cancel()
		manager, err := shill.NewManager(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed creating shill manager proxy")
		}
		testing.ContextLog(ctx, "Disable ethernet")
		ethEnableFunc, err := manager.DisableTechnologyForTesting(ctx, shill.TechnologyEthernet)
		if err != nil {
			return nil, errors.Wrap(err, "failed to disable ethernet")
		}
		defer ethEnableFunc(cleanupCtx)

		networkNotConnectedImage := nodewith.Name("Not connected to network").Role(role.Image)
		if err := uiauto.New(p.tLoginConn).WaitUntilExists(networkNotConnectedImage)(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to verify network disabled")
		}
	}

	if err := uiauto.Combine("sign in smart card",
		p.sc.SignIn(),
		p.sc.SetPIN(req.PinCode),
	)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to sign in smart card")
	}

	return &empty.Empty{}, nil
}

// SmartCardRemovalAndInsertionTesting tests whether the UI during smart card
// removal and insertion is as expected.
// If req.Offline is true, it disables ethernet before re-authentication.
func (p *PeriphService) SmartCardRemovalAndInsertionTesting(ctx context.Context, req *peripherals.SmartCardRemovalAndInsertionTestingRequest) (*empty.Empty, error) {
	if req.Offline {
		var cancel context.CancelFunc
		cleanupCtx := ctx
		ctx, cancel = ctxutil.Shorten(ctx, 2*shill.EnableWaitTime+10*time.Second)
		defer cancel()
		manager, err := shill.NewManager(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed creating shill manager proxy")
		}
		testing.ContextLog(ctx, "Disable ethernet")
		ethEnableFunc, err := manager.DisableTechnologyForTesting(ctx, shill.TechnologyEthernet)
		if err != nil {
			return nil, errors.Wrap(err, "failed to disable ethernet")
		}
		defer ethEnableFunc(cleanupCtx)

		networkNotConnectedImage := nodewith.Name("Not connected to network").Role(role.Image)
		if err := uiauto.New(p.tLoginConn).WaitUntilExists(networkNotConnectedImage)(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to verify network disabled")
		}
	}

	unrecognizedText := nodewith.Name("Couldn’t recognize your smart card. Try again.").Role(role.StaticText)
	if err := uiauto.NamedCombine("smart card removal and insertion testing",
		// GoBigSleepLint: Wait for removing smart card manually within 10s.
		uiauto.NamedAction("wait for removing smart card", uiauto.Sleep(10*time.Second)),
		p.sc.SignIn(),
		uiauto.New(p.tLoginConn).WaitUntilExists(unrecognizedText),
		// GoBigSleepLint: Wait for inserting smart card manually within 10s.
		uiauto.NamedAction("wait for inserting smart card", uiauto.Sleep(10*time.Second)),
		p.sc.SignIn(),
		p.sc.SetPIN(req.WrongPinCode),
		p.sc.SetPIN(req.CorrectPinCode),
	)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to perform smart card removal and insertion testing")
	}

	return &empty.Empty{}, nil
}

// AddPerson adds person.
func (p *PeriphService) AddPerson(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if err := p.sc.AddPerson()(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to add person")
	}
	return &empty.Empty{}, nil
}

// LockScreen locks the screen and verifies no password/PIN field is shown.
func (p *PeriphService) LockScreen(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if err := uiauto.NamedCombine("lock the screen",
		p.kb.AccelAction("Search+L"),
		p.updateChromeAndSmartCard,
	)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to lock screen")
	}

	ui := uiauto.New(p.tLoginConn)
	signInButton := nodewith.Name("Sign in with smart card").Role(role.Button)
	simplePinOrPasswordFieldFinder := nodewith.Role(role.TextField).NameContaining("Password for")
	if err := uiauto.NamedCombine("verify no password/PIN field is shown",
		ui.WaitUntilExists(signInButton),
		ui.Gone(simplePinOrPasswordFieldFinder),
	)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to verify no password/PIN field is shown")
	}

	return &empty.Empty{}, nil
}

// SetSecurityTokenRemoval sets security policy and removal notification duration.
func (p *PeriphService) SetSecurityTokenRemoval(ctx context.Context, req *peripherals.SetSecurityTokenRemovalRequest) (*empty.Empty, error) {
	duration := strconv.FormatInt(req.Duration, 10)
	if err := p.sc.SetSecurityTokenRemoval(s.SecurityPolicyOption(req.SecurityPolicyOption), duration)(ctx); err != nil {
		return nil, errors.Wrapf(err, "failed to set security token removal to %q", req.SecurityPolicyOption)
	}
	return &empty.Empty{}, nil
}

// WaitForAutoLockOrLogOut waits for auto lock or log out.
func (p *PeriphService) WaitForAutoLockOrLogOut(ctx context.Context, req *peripherals.SetSecurityTokenRemovalRequest) (*empty.Empty, error) {
	policy := s.SecurityPolicyOption(req.SecurityPolicyOption)
	duration := time.Duration(req.Duration) * time.Second
	if err := p.sc.WaitForAutoLockOrLogOut(policy, duration)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to wait for auto lock or log out")
	}
	var loggedIn, locked bool
	switch policy {
	case s.SecurityPolicyLock:
		locked = true
		loggedIn = true
	case s.SecurityPolicyLogOut:
		loggedIn = false
		locked = false
		if err := p.updateChromeAndSmartCard(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to update chrome")
		}
	}

	if st, err := lockscreen.WaitState(ctx, p.tLoginConn,
		func(st lockscreen.State) bool {
			return st.LoggedIn == loggedIn && st.Locked == locked && st.ReadyForPassword
		}, 30*time.Second); err != nil {
		return nil, errors.Wrapf(err, "failed to wait for screen to be %q (last status %+v)", policy, st)
	}

	return &empty.Empty{}, nil
}

// WebApplicationAuthentication performs web application authentication with a smart card.
func (p *PeriphService) WebApplicationAuthentication(ctx context.Context, req *peripherals.WebApplicationAuthenticationRequest) (*empty.Empty, error) {
	if err := p.sc.WebApplicationAuthentication(req.Email, req.PinCode)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to perform web application authentication")
	}
	return &empty.Empty{}, nil
}

// DriveLockCSSIAppTesting opens Drivelock CSSI app and recognizes the smart card.
func (p *PeriphService) DriveLockCSSIAppTesting(ctx context.Context, req *peripherals.SetPINRequest) (*empty.Empty, error) {
	tconn, err := p.sharedObject.Chrome.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Test API connection")
	}

	if err := p.sc.DriveLockCSSIAppTesting(ctx, tconn, req.PinCode); err != nil {
		return nil, errors.Wrap(err, "failed to perform drive lock CSSI app testing")
	}
	return &empty.Empty{}, nil
}

// VerifyCertificate verifies the account's certificate.
func (p *PeriphService) VerifyCertificate(ctx context.Context, req *peripherals.VerifyCertificateRequest) (*empty.Empty, error) {
	if err := p.sc.VerifyCertificate(ctx, p.sharedObject.Chrome, req.Username); err != nil {
		return nil, errors.Wrap(err, "failed to verify the account's certificate")
	}
	return &empty.Empty{}, nil
}

func (p *PeriphService) newChrome(ctx context.Context, opts ...chrome.Option) error {
	p.sharedObject.ChromeMutex.Lock()
	defer p.sharedObject.ChromeMutex.Unlock()
	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		return errors.Wrap(err, "failed to start Chrome")
	}
	p.sharedObject.Chrome = cr
	return nil
}

// updateChromeAndSmartCard reinitializes the Chrome instance and SmartCard instance.
// After signing out, the Chrome connection will be closed. It is recommended to
// reconnect Chrome after signing out.
func (p *PeriphService) updateChromeAndSmartCard(ctx context.Context) error {
	if p.sc == nil {
		return errors.New("There is no SmartCard instance")
	}
	manifestKey := signinProfileTestExtensionManifestKey.Value()

	cr, err := chrome.New(ctx,
		chrome.NoLogin(),
		chrome.KeepState(),
		chrome.TryReuseSession(),
		chrome.LoadSigninProfileExtension(manifestKey),
	)
	if err != nil {
		return err
	}
	p.sharedObject.Chrome = cr

	// Wait for the signin OOBE page to appear.
	oobeConn, err := cr.WaitForOOBEConnection(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create OOBE connection")
	}
	defer oobeConn.Close()

	// When in OOBE, use SigninProfileTestAPIConn to create the test connection.
	p.tLoginConn, err = cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create test API connection")
	}
	p.sc.UpdateInstance(cr, p.tLoginConn)
	return nil
}

func (p *PeriphService) waitUntilUserLogin(ctx context.Context) error {
	tconn, err := p.sharedObject.Chrome.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}

	if err := lockscreen.WaitForLoggedIn(ctx, tconn, chrome.LoginTimeout); err != nil {
		return errors.Wrap(err, "failed to login")
	}

	ui := uiauto.New(p.tLoginConn)
	signOutButton := nodewith.Name("Sign out").Role(role.Button).ClassName("MdTextButton")
	if err := ui.WaitUntilExists(signOutButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for sign out button")
	}

	testing.ContextLog(ctx, "User login successful")

	return nil
}
