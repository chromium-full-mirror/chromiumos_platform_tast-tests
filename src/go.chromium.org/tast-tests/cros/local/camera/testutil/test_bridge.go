// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testutil provides utilities to setup testing environment for camera
// tests.
package testutil

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/mafredri/cdp/protocol/target"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

// UseCameraType defines what camera type is used in a test.
type UseCameraType int

const (
	// UseRealCamera is used when the test should test with real camera which exercises cros-camera service.
	UseRealCamera UseCameraType = iota
	// UseVividCamera is used when the test should use vivid camera, which is virtual video test driver.
	UseVividCamera
	// UseFakeVCDCamera is used when the test should use fake camera in Chrome VCD stack.
	UseFakeVCDCamera
	// UseFakeHALCamera is used when the test should use fake camera from fake camera HAL.
	UseFakeHALCamera

	jsonConfigPath    = "/var/cache/camera/test_config.json"
	fakeHALConfigPath = "/run/camera/fake_hal.json"
	fakeHALImageDir   = "/var/cache/camera/"
)

// TestBridge is used to communicate with CCA for test specific logic, such as test environment set-up/tear-down flow, performance/error monitoring.
type TestBridge struct {
	cr       *chrome.Chrome
	pageConn *chrome.Conn
	bridge   *chrome.JSObject

	// TODO(b/177800595): Store this information in fixture.
	CameraType UseCameraType
}

func getTestConfig(cameraType UseCameraType) ([]byte, error) {
	if cameraType == UseFakeHALCamera {
		return json.Marshal(map[string]interface{}{
			"abort_when_capture_monitor_timeout": true,
			// Guarantee that only the fake HAL camera is available and all other cameras are disabled.
			"enabled_hals": []string{"fake.so"},
		})
	}
	return json.Marshal(map[string]bool{
		"abort_when_capture_monitor_timeout": true,
	})
}

// SetupTestConfig writes the test config based on camera type.
func SetupTestConfig(ctx context.Context, cameraType UseCameraType) error {
	jsonCfg, err := getTestConfig(cameraType)
	if err != nil {
		return errors.Wrap(err, "failed to encode test config as json")
	}
	if err := ioutil.WriteFile(jsonConfigPath, jsonCfg, 0644); err != nil {
		return errors.Wrap(err, "failed to write json config file")
	}
	return nil
}

// FakeCameraImageConfig represents the config of the path to a real image.
type FakeCameraImageConfig struct {
	Path      string `json:"path,omitempty"`
	ScaleMode string `json:"scale_mode,omitempty"`
}

// FakeCameraFormatsConfig represents the config of the video format.
type FakeCameraFormatsConfig struct {
	Width  int `json:"width"`
	Height int `json:"height"`
	// Support only constant frame rates [30, 60] but not frame rate ranges [[1, 999], [10, 30]] for now.
	FrameRates []int `json:"frame_rates"`
}

// FakeCameraConfig represents the config for a single camera for fake HAL.
type FakeCameraConfig struct {
	ID               int                        `json:"id"`
	Connected        bool                       `json:"connected"`
	Frames           *FakeCameraImageConfig     `json:"frames,omitempty"`
	SupportedFormats []*FakeCameraFormatsConfig `json:"supported_formats,omitempty"`
	// TODO(pihsun): Add other fields.
}

// FakeHALConfig represents the config for the fake HAL.
type FakeHALConfig struct {
	Cameras []FakeCameraConfig `json:"cameras"`
}

// WriteFakeHALConfig writes the config to fake HAL config file.
func WriteFakeHALConfig(ctx context.Context, config FakeHALConfig) error {
	jsonCfg, err := json.Marshal(config)
	if err != nil {
		return errors.Wrap(err, "failed to encode fake hal config as json")
	}
	if err := ioutil.WriteFile(fakeHALConfigPath, jsonCfg, 0644); err != nil {
		return errors.Wrap(err, "failed to write json config file")
	}
	return nil
}

// SetupFakeHALConfig writes the default config for fake HAL tests.
func SetupFakeHALConfig(ctx context.Context) error {
	return WriteFakeHALConfig(ctx, FakeHALConfig{
		Cameras: []FakeCameraConfig{
			{ID: 1, Connected: true},
		},
	})
}

// CopyFakeHALFrameImage copies the input image to a folder that camera service
// can access, for fake HAL tests.
func CopyFakeHALFrameImage(path string) (string, error) {
	targetPath := filepath.Join(fakeHALImageDir, filepath.Base(path))
	if err := fsutil.CopyFile(path, targetPath); err != nil {
		return "", errors.Wrapf(err, "failed to copy from the given image: %v", path)
	}
	return targetPath, nil
}

// RemoveTestConfig removes the test config if it exists or returns nil otherwise.
func RemoveTestConfig(ctx context.Context) error {
	return os.RemoveAll(jsonConfigPath)
}

// RemoveFakeHALConfig removes the fake hal config if it exists or returns nil otherwise.
func RemoveFakeHALConfig(ctx context.Context) error {
	return os.RemoveAll(fakeHALConfigPath)
}

// NewTestBridge returns a new test bridge instance.
func NewTestBridge(ctx context.Context, cr *chrome.Chrome, cameraType UseCameraType) (*TestBridge, error) {
	if cameraType != UseFakeVCDCamera {
		if err := SetupTestConfig(ctx, cameraType); err != nil {
			return nil, errors.Wrap(err, "failed to setup test config")
		}
		if cameraType == UseFakeHALCamera {
			if err := SetupFakeHALConfig(ctx); err != nil {
				return nil, errors.Wrap(err, "failed to setup fake hal config")
			}
		}
		if err := upstart.RestartJob(ctx, "cros-camera"); err != nil {
			return nil, errors.Wrap(err, "failed to restart cros-camera after test config setup")
		}
	}

	pageConn, bridge, err := setUpTestBridge(ctx, cr)
	if err != nil {
		return nil, err
	}
	return &TestBridge{cr, pageConn, bridge, cameraType}, nil
}

// NewTestBridgeWithoutTestConfig returns a new test bridge instance without test config.
func NewTestBridgeWithoutTestConfig(ctx context.Context, cr *chrome.Chrome, cameraType UseCameraType) (*TestBridge, error) {
	if err := RemoveTestConfig(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to remove test config")
	}
	if err := upstart.RestartJob(ctx, "cros-camera"); err != nil {
		return nil, errors.Wrap(err, "failed to restart cros-camera after removing test config")
	}

	pageConn, bridge, err := setUpTestBridge(ctx, cr)
	if err != nil {
		return nil, err
	}
	return &TestBridge{cr, pageConn, bridge, cameraType}, nil
}

func setUpTestBridge(ctx context.Context, cr *chrome.Chrome) (*chrome.Conn, *chrome.JSObject, error) {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to get test API connection")
	}

	if err := tconn.Call(ctx, nil, `tast.promisify(chrome.autotestPrivate.waitForSystemWebAppsInstall)`); err != nil {
		return nil, nil, errors.Wrap(err, "failed to wait for system web apps installed")
	}

	pageConn, err := cr.NewConn(ctx, "chrome://camera-app/test/test.html")
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to construct bridge page connection")
	}

	if err := pageConn.Eval(ctx, "location.reload()", nil); err != nil {
		return nil, nil, errors.Wrap(err, "failed to reload the test extension")
	}
	if err := pageConn.WaitForExpr(ctx, "document.readyState === 'complete'"); err != nil {
		return nil, nil, errors.Wrap(err, "failed to wait for the ready state")
	}

	const code = `
	  async function() {
		const testBridgeUrlPolicy = trustedTypes.createPolicy(
		'ash-webui-test-script', {createScriptURL: () => '/js/test_bridge.js'});
		const workerPath = testBridgeUrlPolicy.createScriptURL('');
		const sharedWorker = new SharedWorker(workerPath, {type: 'module'});
		const Comlink = await import('/js/lib/comlink.js');
		return Comlink.wrap(sharedWorker.port);
	  }`
	var testBridge chrome.JSObject
	if err := pageConn.Call(ctx, &testBridge, code); err != nil {
		if err2 := tearDownBridgePageConnection(ctx, cr, pageConn); err2 != nil {
			testing.ContextLog(ctx, "Failed to tear down bridge page connection", err2)
		}
		return nil, nil, errors.Wrap(err, "failed to get test bridge")
	}
	return pageConn, &testBridge, nil
}

func tearDownBridgePageConnection(ctx context.Context, cr *chrome.Chrome, conn *chrome.Conn) error {
	checkTestPage := func(t *target.Info) bool {
		return t.URL == "chrome://camera-app/test/test.html"
	}
	if testPageAlive, err := cr.IsTargetAvailable(ctx, checkTestPage); err == nil {
		if testPageAlive {
			if err := conn.CloseTarget(ctx); err != nil {
				return errors.Wrap(err, "failed to call CloseTarget() on the bridge page connection")
			}
		}
	} else {
		testing.ContextLog(ctx, "Failed to check if test page is alive or not: ", err)
	}
	if err := conn.Close(); err != nil {
		return errors.Wrap(err, "failed to call Close() on the bridge page connection")
	}
	return nil
}

// AppWindow registers and returns the app window which is used to communicate with the foreground window of CCA instance.
func (t *TestBridge) AppWindow(ctx context.Context) (*AppWindow, error) {
	var appWindow chrome.JSObject
	if err := t.bridge.Call(ctx, &appWindow, "function() { return this.registerUnboundWindow(); }"); err != nil {
		return nil, errors.Wrap(err, "failed to register app window")
	}
	return &AppWindow{&appWindow}, nil
}

// TearDown tears down the connection of test bridge.
func (t *TestBridge) TearDown(ctx context.Context) error {
	if t.bridge != nil {
		if err := t.bridge.Call(ctx, nil, `function() { this.close(); }`); err != nil {
			testing.ContextLog(ctx, "Failed to close worker: ", err)
		}
		if err := t.bridge.Release(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to release test bridge object: ", err)
		}
		t.bridge = nil
	}
	if t.pageConn != nil {
		if err := tearDownBridgePageConnection(ctx, t.cr, t.pageConn); err != nil {
			testing.ContextLog(ctx, "Failed to release bridge page connection: ", err)
		}
		t.pageConn = nil
	}
	if err := RemoveFakeHALConfig(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to remove fake hal config: ", err)
	}
	if err := RemoveTestConfig(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to remove test config: ", err)
	}
	return nil
}
