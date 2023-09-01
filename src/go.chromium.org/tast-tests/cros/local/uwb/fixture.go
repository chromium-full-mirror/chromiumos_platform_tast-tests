// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package uwb

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/adb"
	localadb "go.chromium.org/tast-tests/cros/local/android/adb"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	resetTimeout = 30 * time.Second
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            "uwbMultiAndroid",
		Desc:            "A fixture that sets up multiple android phones",
		Impl:            &multiAndroidPeerFixture{},
		Parent:          "uwbMixedPeerRemote",
		Contacts:        []string{"chromeos-uwb-team@google.com"},
		SetUpTimeout:    4 * time.Minute,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: resetTimeout,
	})
}

// PhonePeer represents an android peer device.
type PhonePeer struct {
	ADBDevice *adb.Device
}

// FixtData holds information made available to tests that specify this Fixture.
type FixtData struct {
	PhonePeers []*PhonePeer
}

type multiAndroidPeerFixture struct {
	phones []*PhonePeer
}

func (f *multiAndroidPeerFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	var parentData struct {
		PhoneIPs []string
	}

	// Get the phone IP address from the remote parent fixture.
	if err := s.ParentFillValue(&parentData); err != nil {
		s.Fatal("Failed to deserialize fixture data with FixtFillValue: ", err)
	}

	androidPeers, err := setUpAndroidPeers(ctx, parentData.PhoneIPs)
	if err != nil {
		s.Fatal("Failed to setup remote Android peers: ", err)
	}
	f.phones = androidPeers

	return &FixtData{
		PhonePeers: f.phones,
	}
}

func (f *multiAndroidPeerFixture) Reset(ctx context.Context) error                        { return nil }
func (f *multiAndroidPeerFixture) PreTest(ctx context.Context, s *testing.FixtTestState)  {}
func (f *multiAndroidPeerFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}
func (f *multiAndroidPeerFixture) TearDown(ctx context.Context, s *testing.FixtState) {
}

// setUpAndroidPeers launches an ADB service locally and connects to a remote phone by its IP address.
func setUpAndroidPeers(ctx context.Context, phones []string) ([]*PhonePeer, error) {
	// launch ADB server locally
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return localadb.LaunchServer(ctx)
	}, &testing.PollOptions{Interval: time.Second, Timeout: time.Minute}); err != nil {
		return nil, errors.Wrap(err, "failed to launch local adb server")
	}

	devices := make([]*PhonePeer, len(phones))
	for i, phone := range phones {
		testing.ContextLogf(ctx, "Connecting to Android device: %s", phone)
		adbDevice, err := adb.Connect(ctx, phone, 1*time.Minute)
		if err != nil {
			return nil, errors.Wrap(err, "failed to connect to remote adb device")
		}

		testing.ContextLog(ctx, "Connected to remote Android device")
		devices[i] = &PhonePeer{ADBDevice: adbDevice}

		// Wait for the Android device to be ready for use.
		if err := devices[i].ADBDevice.WaitForState(ctx, adb.StateDevice, 30*time.Second); err != nil {
			return nil, errors.Wrap(err, "wait for state failed")
		}
	}

	return devices, nil
}
