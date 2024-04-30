// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mojo

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/bluetooth"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: "bluetoothMojoJSObjectWithBlueZ",
		Desc: "Fixture for tests that use the CrosNetworkConfig mojo API using BlueZ",
		Contacts: []string{
			"cros-connectivity@google.com",
			"chadduffin@google.com",
		},
		BugComponent:    "b:1131776", // ChromeOS > Software > Fundamentals > Connectivity > Bluetooth
		Impl:            &crosNetworkConfigConnection{},
		Parent:          "bluetoothEnabledWithBlueZ",
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "bluetoothMojoJSObjectWithFloss",
		Desc: "Fixture for tests that use the CrosNetworkConfig mojo API using Floss",
		Contacts: []string{
			"cros-connectivity@google.com",
			"chadduffin@google.com",
		},
		BugComponent:    "b:1131776", // ChromeOS > Software > Fundamentals > Connectivity > Bluetooth
		Impl:            &crosNetworkConfigConnection{},
		Parent:          "bluetoothEnabledWithFloss",
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
}

// HasBluetoothImpl is an interface for fixture values that contain a Bluetooth implementation.
// It allows retrieval of the underlying Bluetooth implementation.
type HasBluetoothImpl interface {
	BluetoothImpl() bluetooth.Bluetooth
}

// HasConn is an interface for fixture values that contain a Chrome connection.
// It allows retrieval of the underlying Chrome connection.
type HasConn interface {
	Conn() *chrome.Conn
}

// HasJSObject is an interface for fixture values that contain a JS object.
// It allows retrieval of the underlying JS object.
type HasJSObject interface {
	JSObject() *chrome.JSObject
}

type crosNetworkConfigConnection struct {
	btImpl bluetooth.Bluetooth
	conn   *chrome.Conn
	js     *chrome.JSObject
}

// BluetoothImpl returns the Bluetooth implementation.
// It implements the HasBluetoothImpl interface.
func (f *crosNetworkConfigConnection) BluetoothImpl() bluetooth.Bluetooth {
	return f.btImpl
}

// Conn returns the Chrome connection.
// It implements the HasConn interface.
func (f *crosNetworkConfigConnection) Conn() *chrome.Conn {
	return f.conn
}

// JSObject returns the JS object.
// It implements the HasJSObject interface.
func (f *crosNetworkConfigConnection) JSObject() *chrome.JSObject {
	return f.js
}

func (f *crosNetworkConfigConnection) Reset(ctx context.Context) error {
	if err := f.js.Release(ctx); err != nil {
		return errors.Wrap(err, "failed to release Bluetooth mojo JS object")
	}

	if err := f.conn.Call(ctx, &(f.js), BTConfigJS); err != nil {
		return errors.Wrap(err, "failed to create Bluetooth mojo JS")
	}

	if err := f.js.Call(ctx, nil, `function init(){ this.initSysPropObs()}`); err != nil {
		return errors.Wrap(err, "failed to initailize the observer")
	}

	return nil
}

func (*crosNetworkConfigConnection) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (*crosNetworkConfigConnection) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *crosNetworkConfigConnection) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	cr := s.ParentValue().(bluetooth.HasChrome).Chrome()

	// Open OS settings App Bluetooth Subpage
	const url = "chrome://os-settings/bluetooth"
	conn, err := apps.LaunchOSSettings(ctx, cr, url)
	if err != nil {
		s.Fatal("Failed to open settings app: ", err)
	}

	var js chrome.JSObject

	if err := conn.Call(ctx, &js, BTConfigJS); err != nil {
		s.Fatal(errors.Wrap(err, "failed to create Bluetooth mojo JS"))
	}

	if err := js.Call(ctx, nil, `function init(){ this.initSysPropObs()}`); err != nil {
		s.Fatal(errors.Wrap(err, "failed to initailize the observer"))
	}

	f.btImpl = s.ParentValue().(bluetooth.HasBluetoothImpl).BluetoothImpl()
	f.conn = conn
	f.js = &js
	return f
}

func (f *crosNetworkConfigConnection) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := f.js.Release(ctx); err != nil {
		s.Fatal(errors.Wrap(err, "failed to release Bluetooth mojo JS object"))
	}
}
