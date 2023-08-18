// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package cca provides utilities to interact with Chrome Camera App.
package cca

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/camera/chart"
	dutcontrol "go.chromium.org/tast-tests/cros/common/camera/dut"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/assistant"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

const (
	ccaSetUpTimeout        = 25 * time.Second
	ccaTearDownTimeout     = 5 * time.Second
	testBridgeSetUpTimeout = 20 * time.Second
	setUpTimeout           = chrome.LoginTimeout + testBridgeSetUpTimeout
	tearDownTimeout        = chrome.ResetTimeout
)

type feature string

const (
	timeLapse feature = "CameraAppTimeLapse"
)

var (
	recordScreen = testing.RegisterVarString(
		"cca.record_screen",
		"false",
		`Whether to record screen while running tests.
		 The screen recording will be saved if the test fails.
		 Make sure the fixture has sufficient PreTestTimeout / PostTestTimeout to process screen recording`)
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            "ccaLaunchedInCameraBox",
		Desc:            "Launched CCA in a Camera Box",
		Contacts:        []string{"chromeos-camera-eng@google.com", "wtlee@chromium.org"},
		Impl:            &fixture{launchCCAInCameraBox: true, launchCCA: true},
		Parent:          "remoteCameraBox",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    testBridgeSetUpTimeout,
		PreTestTimeout:  ccaSetUpTimeout,
		PostTestTimeout: ccaTearDownTimeout,
		TearDownTimeout: tearDownTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "ccaLaunched",
		Desc:            "Launched CCA",
		Contacts:        []string{"chromeos-camera-eng@google.com", "wtlee@chromium.org"},
		Impl:            &fixture{launchCCA: true},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    testBridgeSetUpTimeout,
		PreTestTimeout:  ccaSetUpTimeout,
		PostTestTimeout: ccaTearDownTimeout,
		TearDownTimeout: tearDownTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "ccaLaunchedGuestWithFakeHALCamera",
		Desc:            "Launched CCA",
		Contacts:        []string{"chromeos-camera-eng@google.com", "wtlee@chromium.org"},
		Impl:            &fixture{useCameraType: testutil.UseFakeHALCamera, guestMode: true, launchCCA: true},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    testBridgeSetUpTimeout,
		PreTestTimeout:  ccaSetUpTimeout,
		PostTestTimeout: ccaTearDownTimeout,
		TearDownTimeout: tearDownTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "ccaLaunchedWithFakeVCDCamera",
		Desc:            "Launched CCA with fake VCD camera input",
		Contacts:        []string{"chromeos-camera-eng@google.com", "wtlee@chromium.org"},
		Impl:            &fixture{useCameraType: testutil.UseFakeVCDCamera, launchCCA: true},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    testBridgeSetUpTimeout,
		PreTestTimeout:  ccaSetUpTimeout,
		PostTestTimeout: ccaTearDownTimeout,
		TearDownTimeout: tearDownTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "ccaLaunchedWithFakeHALCamera",
		Desc:            "Launched CCA with fake camera HAL input",
		Contacts:        []string{"chromeos-camera-eng@google.com", "pihsun@chromium.org"},
		Impl:            &fixture{useCameraType: testutil.UseFakeHALCamera, launchCCA: true},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    testBridgeSetUpTimeout,
		PreTestTimeout:  ccaSetUpTimeout,
		PostTestTimeout: ccaTearDownTimeout,
		TearDownTimeout: tearDownTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "ccaLaunchedWithFakeVCDCameraFieldTrialConfigOn",
		Desc:            "Launched CCA with fake VCD camera input with field trial config enabled",
		Contacts:        []string{"chromeos-camera-eng@google.com", "wtlee@chromium.org"},
		Impl:            &fixture{useCameraType: testutil.UseFakeVCDCamera, launchCCA: true, fieldTrialConfig: chrome.FieldTrialConfigEnable},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    testBridgeSetUpTimeout,
		PreTestTimeout:  ccaSetUpTimeout,
		PostTestTimeout: ccaTearDownTimeout,
		TearDownTimeout: tearDownTimeout,
	})

	// TODO(http://b/275895388): Remove after feature launch.
	testing.AddFixture(&testing.Fixture{
		Name:            "ccaLaunchedWithHoldingSpaceIntegrationEnabled",
		Desc:            "Launched CCA with holding space integration enabled",
		Contacts:        []string{"chromeos-camera-eng@google.com", "cros-system-ui-eng@google.com", "dmblack@google.com"},
		Impl:            &fixture{launchCCA: true, enableFeatures: []feature{"HoldingSpaceCameraAppIntegration"}},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    testBridgeSetUpTimeout,
		PreTestTimeout:  ccaSetUpTimeout,
		PostTestTimeout: ccaTearDownTimeout,
		TearDownTimeout: tearDownTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "ccaLaunchedWithFakeVCDCameraFieldTrialConfigOff",
		Desc:            "Launched CCA with fake VCD camera input with field trial config disabled",
		Contacts:        []string{"chromeos-camera-eng@google.com", "wtlee@chromium.org"},
		Impl:            &fixture{useCameraType: testutil.UseFakeVCDCamera, launchCCA: true, fieldTrialConfig: chrome.FieldTrialConfigDisable},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    testBridgeSetUpTimeout,
		PreTestTimeout:  ccaSetUpTimeout,
		PostTestTimeout: ccaTearDownTimeout,
		TearDownTimeout: tearDownTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "ccaTestBridgeReady",
		Desc:            "Set up test bridge for CCA",
		Contacts:        []string{"chromeos-camera-eng@google.com", "wtlee@chromium.org"},
		Impl:            &fixture{},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    testBridgeSetUpTimeout,
		TearDownTimeout: tearDownTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "ccaTestBridgeReadyLacros",
		Desc:            "Set up test bridge for CCA",
		Contacts:        []string{"chromeos-camera-eng@google.com", "wtlee@chromium.org"},
		Impl:            &fixture{lacros: true},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    testBridgeSetUpTimeout,
		TearDownTimeout: tearDownTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "ccaTestBridgeReadyWithFakeCamera",
		Desc: `Set up test bridge for CCA with fake camera. Any tests using this
		       fixture should switch the camera scene before opening camera`,
		Contacts:        []string{"chromeos-camera-eng@google.com", "wtlee@chromium.org"},
		Impl:            &fixture{useCameraType: testutil.UseFakeVCDCamera, fakeScene: true},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    testBridgeSetUpTimeout,
		TearDownTimeout: tearDownTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "ccaTestBridgeReadyWithFakeCameraWithoutFakeScene",
		Desc:            `Set up test bridge for CCA with fake camera without fake scene`,
		Contacts:        []string{"chromeos-camera-eng@google.com", "wtlee@chromium.org"},
		Impl:            &fixture{useCameraType: testutil.UseFakeVCDCamera},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    testBridgeSetUpTimeout,
		TearDownTimeout: tearDownTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "ccaTestBridgeReadyWithFakeCameraLacros",
		Desc: `Set up test bridge for CCA with fake camera. Any tests using this
		       fixture should switch the camera scene before opening camera`,
		Contacts:        []string{"chromeos-camera-eng@google.com", "wtlee@chromium.org"},
		Impl:            &fixture{useCameraType: testutil.UseFakeVCDCamera, fakeScene: true, lacros: true},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    testBridgeSetUpTimeout,
		TearDownTimeout: tearDownTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "ccaTestBridgeReadyWithFakeHALCamera",
		Desc:            "Set up test bridge for CCA with fake camera HAL input",
		Contacts:        []string{"chromeos-camera-eng@google.com", "pihsun@chromium.org"},
		Impl:            &fixture{useCameraType: testutil.UseFakeHALCamera},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    testBridgeSetUpTimeout,
		TearDownTimeout: tearDownTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "ccaTestBridgeReadyWithFakeHALCameraLacros",
		Desc:            "Set up test bridge for CCA with fake camera HAL input and lacros",
		Contacts:        []string{"chromeos-camera-eng@google.com", "pihsun@chromium.org"},
		Impl:            &fixture{useCameraType: testutil.UseFakeHALCamera, lacros: true},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    testBridgeSetUpTimeout,
		TearDownTimeout: tearDownTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "ccaTestBridgeReadyWithFakeHALCameraBypassPermissionClamshell",
		Desc:            "Set up test bridge for CCA with fake camera HAL input bypassPermission on clamshell mode on",
		Contacts:        []string{"chromeos-camera-eng@google.com", "pihsun@chromium.org"},
		Impl:            &fixture{useCameraType: testutil.UseFakeHALCamera, bypassPermission: true, forceClamshell: true},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    testBridgeSetUpTimeout,
		TearDownTimeout: tearDownTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "ccaTestBridgeReadyWithArc",
		Desc:            "Set up test bridge for CCA with ARC enabled",
		Contacts:        []string{"chromeos-camera-eng@google.com", "wtlee@chromium.org"},
		Impl:            &fixture{arcBooted: true},
		SetUpTimeout:    setUpTimeout + arc.BootTimeout + ui.StartTimeout,
		ResetTimeout:    testBridgeSetUpTimeout,
		TearDownTimeout: tearDownTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "ccaTestBridgeReadyWithArcFakeHALCamera",
		Desc:            "Set up test bridge for CCA with ARC enabled and fake camera HAL",
		Contacts:        []string{"chromeos-camera-eng@google.com", "pihsun@chromium.org"},
		Impl:            &fixture{arcBooted: true, useCameraType: testutil.UseFakeHALCamera},
		SetUpTimeout:    setUpTimeout + arc.BootTimeout + ui.StartTimeout,
		ResetTimeout:    testBridgeSetUpTimeout,
		TearDownTimeout: tearDownTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "ccaTestBridgeReadyWithAutoFramingForceEnabled",
		Desc:     "Set up test bridge for CCA with Auto Framing force enabled",
		Contacts: []string{"chromeos-camera-eng@google.com", "kamesan@chromium.org", "chromeos-camera-eng@google.com"},
		// TODO(b/290826477): Fix users of this fixture for the new quick settings UI (QsRevamp).
		Impl:            &fixture{forceEnableAutoFraming: true, disableFeatures: []feature{"QsRevamp"}},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    testBridgeSetUpTimeout,
		TearDownTimeout: tearDownTimeout,
	})

	// TODO(b/236800499): Remove this fixture once the feature flag is enabled by default.
	testing.AddFixture(&testing.Fixture{
		Name:            "ccaLaunchedWithTimeLapseOnFakeHALCamera",
		Desc:            "Launched CCA with fake camera HAL input and with time-lapse flag enabled",
		Contacts:        []string{"chromeos-camera-eng@google.com", "kamchonlathorn@chromium.org"},
		Impl:            &fixture{useCameraType: testutil.UseFakeHALCamera, enableFeatures: []feature{timeLapse}, launchCCA: true},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    testBridgeSetUpTimeout,
		PreTestTimeout:  ccaSetUpTimeout,
		PostTestTimeout: ccaTearDownTimeout,
		TearDownTimeout: tearDownTimeout,
	})

	// TODO(b/236800499): Remove this fixture once the feature flag is enabled by default.
	testing.AddFixture(&testing.Fixture{
		Name:            "ccaTestBridgeReadyWithTimeLapse",
		Desc:            "Set up test bridge for CCA with time-lapse flag enabled",
		Contacts:        []string{"chromeos-camera-eng@google.com", "kamchonlathorn@chromium.org"},
		Impl:            &fixture{enableFeatures: []feature{timeLapse}},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    testBridgeSetUpTimeout,
		TearDownTimeout: tearDownTimeout,
	})
}

// DebugParams defines some useful flags for debug CCA tests.
type DebugParams struct {
	SaveScreenshotWhenFail   bool
	SaveCameraFolderWhenFail bool
}

// TestWithAppParams defines parameters to control behaviors of running test
// with app.
type TestWithAppParams struct {
	StopAppOnlyIfExist bool
}

// ResetChromeFunc reset chrome used in this fixture.
type ResetChromeFunc func(context.Context) error

// StartAppFunc starts CCA.
type StartAppFunc func(context.Context) (*App, error)

// StopAppFunc stops CCA.
type StopAppFunc func(context.Context, bool) error

// ResetTestBridgeFunc resets the test bridge.
type ResetTestBridgeFunc func(context.Context) error

// TestWithAppFunc is the function to run with app.
type TestWithAppFunc func(context.Context, *App) error

// FixtureData is the struct exposed to tests.
type FixtureData struct {
	Chrome      *chrome.Chrome
	BrowserType browser.Type
	ARC         *arc.ARC
	TestBridge  func() *testutil.TestBridge
	// App returns the CCA instance which lives through the test.
	App func() *App
	// ResetChrome resets chrome used by this fixture.
	ResetChrome ResetChromeFunc
	// StartApp starts CCA which can be used between subtests.
	StartApp StartAppFunc
	// StopApp stops CCA which can be used between subtests.
	StopApp StopAppFunc
	// ResetTestBridgeFunc resets the test bridge. Usually we don't need to call
	// it explicitly unless the sub test launch/tear-down the app itself.
	ResetTestBridge ResetTestBridgeFunc
	// SwitchScene switches the camera scene to the given scene. This only works
	// for fixtures using fake camera stream.
	SwitchScene func(context.Context, SceneData) error
	// RunTestWithApp runs the given function with the handling of the app
	// start/stop.
	RunTestWithApp func(context.Context, TestWithAppFunc, TestWithAppParams) error
	// PrepareChart prepares chart by loading the given scene. It only works for
	// CameraBox.
	PrepareChart func(ctx context.Context, addr, contentPath string) error
	// SetDebugParams sets the debug parameters for current test.
	SetDebugParams func(params DebugParams)
}

type fixture struct {
	cr            *chrome.Chrome
	arc           *arc.ARC
	tb            *testutil.TestBridge
	app           *App
	outDir        string
	chart         *chart.Chart
	cameraScene   string
	brightnessVal string

	lacros                 bool
	useCameraType          testutil.UseCameraType
	fakeScene              bool
	arcBooted              bool
	launchCCA              bool
	bypassPermission       bool
	forceClamshell         bool
	guestMode              bool
	launchCCAInCameraBox   bool
	forceEnableAutoFraming bool
	debugParams            DebugParams
	enableFeatures         []feature
	disableFeatures        []feature
	screenRecorder         *uiauto.ScreenRecorder
	fieldTrialConfig       chrome.FieldTrialConfigMode
	tabletIP               string
}

func (f *fixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	success := false

	var chromeOpts []chrome.Option
	for _, f := range f.enableFeatures {
		chromeOpts = append(chromeOpts, chrome.EnableFeatures(string(f)))
	}
	for _, f := range f.disableFeatures {
		chromeOpts = append(chromeOpts, chrome.DisableFeatures(string(f)))
	}
	chromeOpts = append(chromeOpts, chrome.FieldTrialConfig(f.fieldTrialConfig))

	// Always enable doc scan DLC flag for testing.
	// TODO(b/226262670): Remove this line once the doc scan DLC is completely enabled.
	chromeOpts = append(chromeOpts, chrome.EnableFeatures("CameraAppDocScanDlc"))

	if f.useCameraType == testutil.UseFakeVCDCamera {
		chromeOpts = append(chromeOpts, chrome.ExtraArgs(
			// The default fps of fake device is 20, but CCA requires fps >= 24.
			// Set the fps to 30 to avoid OverconstrainedError.
			"--use-fake-device-for-media-stream=fps=30"))

		if f.fakeScene {
			f.cameraScene = filepath.Join(os.TempDir(), "camera_scene.mjpeg")
			chromeOpts = append(chromeOpts, chrome.ExtraArgs(
				// Set the default camera scene as the input of the fake stream.
				// The content of the scene can be dynamically changed during tests.
				"--use-file-for-fake-video-capture="+f.cameraScene))
		}
	} else if f.useCameraType == testutil.UseFakeHALCamera {
		// Enable device event logs logging to Chrome log to help debugging
		// b:296013012, since device event logs includes camera VCD logs which can
		// be helpful while debugging.
		// TODO(b:297989063): Only collect camera logs, and only collect them when
		// tast fails to reduce noise.
		chromeOpts = append(chromeOpts, chrome.ExtraArgs("--vmodule=device_event_log*=1"))
	}
	if f.guestMode {
		chromeOpts = append(chromeOpts, chrome.GuestLogin())
	}
	if f.arcBooted {
		chromeOpts = append(chromeOpts, chrome.ARCEnabled(), chrome.ExtraArgs("--disable-features=ArcResizeLock"))
	}
	if f.bypassPermission {
		chromeOpts = append(chromeOpts, chrome.ExtraArgs("--use-fake-ui-for-media-stream"))
	}
	if f.forceClamshell {
		chromeOpts = append(chromeOpts, chrome.ExtraArgs("--force-tablet-mode=clamshell"))
	}
	if f.forceEnableAutoFraming {
		chromeOpts = append(chromeOpts, chrome.ExtraArgs("--auto-framing-override=force-enabled"))
	}

	// Enable assistant verbose logging for the CCAUIAssistant test. Since
	// assistant is disabled by default, this should not affect other tests.
	chromeOpts = append(chromeOpts, assistant.VerboseLogging())

	browserType := browser.TypeAsh
	if f.lacros {
		browserType = browser.TypeLacros
		var err error
		chromeOpts, err = lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(chromeOpts...)).Opts()
		if err != nil {
			s.Fatal("Failed to compute Chrome options: ", err)
		}
	}

	cr, err := chrome.New(ctx, chromeOpts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	f.cr = cr
	defer func() {
		if !success {
			f.cr.Close(ctx)
			f.cr = nil
		}
	}()

	if f.arcBooted {
		a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
		if err != nil {
			s.Fatal("Failed to start ARC: ", err)
		}
		f.arc = a
		defer func() {
			if !success {
				f.arc.Close(ctx)
				f.arc = nil
			}
		}()
	}
	if f.launchCCAInCameraBox {
		f.brightnessVal, err = dutcontrol.CCADimBacklight(ctx)
		if err != nil {
			s.Fatal("Failed to set brightness: ", err)
		}
		defer func() {
			if !success {
				dutcontrol.CCARestoreBacklight(ctx, f.brightnessVal)
			}
		}()

		tabletIP := ""
		if err := s.ParentFillValue(&tabletIP); err != nil {
			s.Fatal("Failed to get tabletIP from the remote fixture: ", err)
		}
		f.tabletIP = tabletIP
	}

	tb, err := testutil.NewTestBridge(ctx, cr, f.useCameraType)
	if err != nil {
		s.Fatal("Failed to construct test bridge: ", err)
	}
	f.tb = tb

	success = true
	return FixtureData{
		Chrome:          f.cr,
		BrowserType:     browserType,
		ARC:             f.arc,
		TestBridge:      f.testBridge,
		App:             f.cca,
		ResetChrome:     f.resetChrome,
		StartApp:        f.startApp,
		StopApp:         f.stopApp,
		ResetTestBridge: f.resetTestBridge,
		SwitchScene:     f.switchScene,
		RunTestWithApp:  f.runTestWithApp,
		PrepareChart:    f.prepareChart,
		SetDebugParams:  f.setDebugParams,
	}
}

func (f *fixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := f.tb.TearDown(ctx); err != nil {
		s.Error("Failed to tear down test bridge: ", err)
	}
	f.tb = nil

	if f.arcBooted {
		if err := f.arc.Close(ctx); err != nil {
			s.Error("Failed to tear down ARC: ", err)
		}
		f.arc = nil
	}

	if err := f.cr.Close(ctx); err != nil {
		s.Error("Failed to tear down Chrome: ", err)
	}
	f.cr = nil
	if f.launchCCAInCameraBox {
		if err := dutcontrol.CCARestoreBacklight(ctx, f.brightnessVal); err != nil {
			s.Error("Restore Backlight failed: ", err)
		}
	}
	if f.cameraScene != "" {
		if err := os.RemoveAll(f.cameraScene); err != nil {
			s.Error("Failed to remove camera scene: ", err)
		}
		f.cameraScene = ""
	}
}

func (f *fixture) Reset(ctx context.Context) error {
	return f.resetTestBridge(ctx)
}

func (f *fixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	f.outDir = s.OutDir()

	shouldRecordScreen, err := strconv.ParseBool(recordScreen.Value())
	if err != nil {
		s.Errorf("Unexpected value for `%v`: %v", recordScreen.Name(), recordScreen.Value())
		shouldRecordScreen = false
	}

	if shouldRecordScreen {
		tconn, err := f.cr.TestAPIConn(ctx)
		if err != nil {
			s.Fatal("Failed to get test API: ", err)
		}
		f.screenRecorder = uiauto.CreateAndStartScreenRecorder(ctx, tconn)
		if f.screenRecorder == nil {
			s.Fatal("Failed to create screen recorder")
		}
	}

	if f.launchCCA {
		app, err := f.startApp(ctx)
		if err != nil {
			s.Fatal("Failed to start app: ", err)
		}
		f.app = app
	}
}

func (f *fixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	defer func() {
		f.debugParams = DebugParams{}
	}()

	if f.screenRecorder != nil {
		if s.HasError() {
			uiauto.ScreenRecorderStopSaveRelease(ctx, f.screenRecorder, filepath.Join(f.outDir, "record.webm"))
		} else {
			// If the test passes, stops the recorder without saving the file.
			if err := f.screenRecorder.Stop(ctx); err != nil {
				testing.ContextLogf(ctx, "Failed to stop recording: %s", err)
			}
			f.screenRecorder.Release(ctx)
		}
		f.screenRecorder = nil
	}

	if f.chart != nil {
		if err := f.chart.Close(ctx, f.outDir); err != nil {
			s.Error("Failed to close chart: ", err)
		}
		f.chart = nil
	}

	if f.launchCCA {
		if err := f.stopApp(ctx, s.HasError()); err != nil {
			s.Fatal("Failed to stop app: ", err)
		}
	}
}

func (f *fixture) resetTestBridge(ctx context.Context) error {
	if err := f.tb.TearDown(ctx); err != nil {
		return errors.Wrap(err, "failed to tear down test bridge")
	}
	f.tb = nil

	tb, err := testutil.NewTestBridge(ctx, f.cr, f.useCameraType)
	if err != nil {
		return errors.Wrap(err, "failed to construct test bridge")
	}
	f.tb = tb
	return nil
}

func (f *fixture) resetChrome(ctx context.Context) error {
	if err := f.cr.ResetState(ctx); err != nil {
		return errors.Wrap(err, "failed to reset chrome in fixture")
	}
	tb, err := testutil.NewTestBridge(ctx, f.cr, f.useCameraType)
	if err != nil {
		return errors.Wrap(err, "failed to construct test bridge after reset chrome state")
	}
	f.tb = tb
	return nil
}

func (f *fixture) startApp(ctx context.Context) (*App, error) {
	if err := ClearSavedDir(ctx, f.cr); err != nil {
		return nil, errors.Wrap(err, "failed to clear camera folder")
	}

	app, err := New(ctx, f.cr, f.outDir, f.tb)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open CCA")
	}
	f.app = app
	return f.app, nil
}

func (f *fixture) stopApp(ctx context.Context, hasError bool) (retErr error) {
	if f.app == nil {
		return
	}
	defer func(ctx context.Context) {
		if err := f.app.Close(ctx); err != nil {
			retErr = errors.Wrap(retErr, err.Error())
		}
		if (hasError || retErr != nil) && f.debugParams.SaveCameraFolderWhenFail {
			if err := f.app.SaveCameraFolder(ctx); err != nil {
				retErr = errors.Wrap(retErr, err.Error())
			}
		}
		f.app = nil
	}(ctx)

	if hasError && f.debugParams.SaveScreenshotWhenFail {
		if err := f.app.SaveScreenshot(ctx); err != nil {
			return errors.Wrap(err, "failed to save a screenshot")
		}
	}
	return nil
}

func (f *fixture) stopAppIfExist(ctx context.Context, hasError bool) error {
	if appExist, err := InstanceExists(ctx, f.cr); err != nil {
		return errors.Wrap(err, "failed to check existence of CCA")
	} else if appExist {
		return f.stopApp(ctx, hasError)
	} else {
		// The app does not exist, might be proactively closed by the test flow.
		f.app = nil
	}
	return nil
}

// SceneData is the argument to switchScene.
type SceneData struct {
	// Path is the path to the image of the scene.
	Path string
	// ScaleMode is the mode to be used to scale the image. Can be either
	// "contain", "cover" or "stretch", defaults to "stretch". This only works
	// with fake HAL.
	ScaleMode string
}

// switchScene switches the camera scene of fake camera to the given |scene|.
func (f *fixture) switchScene(ctx context.Context, scene SceneData) error {
	if f.useCameraType == testutil.UseFakeHALCamera {
		if scene.ScaleMode == "" {
			scene.ScaleMode = "stretch"
		}

		targetPath, err := testutil.CopyFakeHALFrameImage(scene.Path)
		if err != nil {
			return err
		}
		if err := testutil.WriteFakeHALConfig(ctx, testutil.FakeHALConfig{
			Cameras: []testutil.FakeCameraConfig{
				{
					ID:        1,
					Connected: true,
					Frames:    &testutil.FakeCameraImageConfig{Path: targetPath, ScaleMode: scene.ScaleMode},
				},
			},
		}); err != nil {
			return err
		}
		return nil
	}

	if f.cameraScene == "" {
		return errors.New("failed to switch scene for non-fake camera stream")
	}

	if scene.ScaleMode != "" {
		return errors.New("ScaleMode only works with fake HAL")
	}

	if err := fsutil.CopyFile(scene.Path, f.cameraScene); err != nil {
		return errors.Wrapf(err, "failed to copy from the given scene: %v", scene)
	}
	return nil
}

func (f *fixture) runTestWithApp(ctx context.Context, testFunc TestWithAppFunc, params TestWithAppParams) (retErr error) {
	app, err := f.startApp(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to start app")
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
	defer cancel()
	defer func(ctx context.Context) {
		hasError := retErr != nil
		stopFunc := f.stopApp
		if params.StopAppOnlyIfExist {
			stopFunc = f.stopAppIfExist
		}
		if err := stopFunc(ctx, hasError); err != nil {
			retErr = errors.Wrap(retErr, err.Error())
		}
		if err := f.resetTestBridge(ctx); err != nil {
			retErr = errors.Wrap(retErr, err.Error())
		}
	}(cleanupCtx)

	return testFunc(ctx, app)
}

func (f *fixture) prepareChart(ctx context.Context, addr, contentPath string) (retErr error) {
	if addr == "" {
		if f.tabletIP == "" {
			return errors.New("chart device is neither found nor specified")
		}
		addr = f.tabletIP
	}

	var sopt ssh.Options
	ssh.ParseTarget(addr, &sopt)
	sopt.KeyDir = chart.SSHKeysDir
	sopt.ConnectTimeout = 10 * time.Second
	conn, err := ssh.New(ctx, &sopt)
	if err != nil {
		return errors.Wrap(err, "failed to connect to chart tablet")
	}
	// No need to close ssh connection since chart will handle it when cleaning
	// up.

	c, namePaths, err := chart.SetUp(ctx, conn, f.outDir, []string{contentPath})
	if err != nil {
		return errors.Wrap(err, "failed to prepare chart tablet")
	}

	if err := c.Display(ctx, namePaths[0]); err != nil {
		return errors.Wrap(err, "failed to display chart on chart tablet")
	}

	f.chart = c
	return nil
}

func (f *fixture) testBridge() *testutil.TestBridge {
	return f.tb
}

func (f *fixture) cca() *App {
	return f.app
}

func (f *fixture) setDebugParams(params DebugParams) {
	f.debugParams = params
}
