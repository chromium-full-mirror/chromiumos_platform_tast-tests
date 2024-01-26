// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"

	"github.com/godbus/dbus/v5"
	"github.com/golang/protobuf/proto"

	uda "chromiumos/system_api/user_data_auth_proto"

	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	cryptohomeName                  = "cryptohomed"
	udaDbusName                     = "org.chromium.UserDataAuth"
	udaDbusPath                     = "/org/chromium/UserDataAuth"
	udaDbusInterface                = "org.chromium.UserDataAuthInterface"
	prepareAuthFactorProgressSignal = "PrepareAuthFactorProgress"
)

// // The interface is defined with generic type because go doesn't allow using self-referencing
// // types in interface method signatures. The restriction will be put in functions that utilizes
// // this interface.
// type fingerprintSignal[T any] interface {
// 	parseFromSignal(*dbus.Signal) (T, error)
// }

type signalParser[T any] func(*dbus.Signal) (T, error)

type fingerprintWatcher[T any] struct {
	watcher *dbusutil.SignalWatcher
	err     error
	done    chan struct{}
	Signals chan T
}

// FingerprintEnrollmentSignal holds values created from the PrepareAuthFactorProgress D-Bus
// signal.
type FingerprintEnrollmentSignal struct {
	ScanResult      uda.FingerprintScanResult
	Done            bool
	PercentComplete int32
}

// FingerprintAuthenticationSignal holds values created from the PrepareAuthFactorProgress D-Bus
// signal.
type FingerprintAuthenticationSignal struct {
	ScanResult uda.FingerprintScanResult
}

// FingerprintEnrollmentWatcher converts the PrepareAuthFactorProgress D-Bus signal to a
// channel of AuthEnrollmentProgress.
type FingerprintEnrollmentWatcher = fingerprintWatcher[*FingerprintEnrollmentSignal]

// FingerprintAuthenticationWatcher converts the PrepareAuthFactorProgress D-Bus signal to a
// channel of AuthScanDone.
type FingerprintAuthenticationWatcher = fingerprintWatcher[*FingerprintAuthenticationSignal]

// NewFingerprintEnrollmentWatcher starts listening for PrepareAuthFactorProgress D-Bus
// signals. Call FingerprintEnrollmentWatcher.Close when finished.
func NewFingerprintEnrollmentWatcher(ctx context.Context) (*FingerprintEnrollmentWatcher, error) {
	return newFingerprintWatcher[*FingerprintEnrollmentSignal](ctx, parseAuthEnrollmentProgress)
}

// NewFingerprintAuthenticationWatcher starts listening for PrepareAuthFactorProgress D-Bus
// signals. Call FingerprintAuthenticationWatcher.Close when finished.
func NewFingerprintAuthenticationWatcher(ctx context.Context) (*FingerprintAuthenticationWatcher, error) {
	return newFingerprintWatcher[*FingerprintAuthenticationSignal](ctx, parseAuthScanDoneProgress)
}

func newFingerprintWatcher[T any](ctx context.Context, parser signalParser[T]) (*fingerprintWatcher[T], error) {
	watcher, err := dbusutil.NewSignalWatcherForSystemBus(ctx, dbusutil.MatchSpec{
		Type:      "signal",
		Path:      udaDbusPath,
		Interface: udaDbusInterface,
		Member:    prepareAuthFactorProgressSignal,
	})
	ret := &fingerprintWatcher[T]{watcher: watcher}
	if err != nil {
		return nil, errors.Wrap(err, "failed to watch PrepareAuthFactorProgress signal")
	}
	ret.done = make(chan struct{})
	ret.Signals = make(chan T)
	go func() {
		defer close(ret.Signals)
		for dbusSig := range ret.watcher.Signals {
			sig, err := parser(dbusSig)
			if err != nil {
				// NB: set err before close, so it will be set by the time a
				// consumer unblocks.
				ret.err = err
				return
			}
			select {
			case ret.Signals <- sig:
			case <-ret.done:
				return
			}
		}
	}()
	return ret, nil
}

// Close stops listening to the PrepareAuthFactorProgress D-Bus signal, and closes Signals.
func (w *fingerprintWatcher[S]) Close(ctx context.Context) error {
	defer close(w.done)
	if err := w.watcher.Close(ctx); err != nil {
		if w.err == nil {
			return errors.Wrap(err, "failed to close SignalWatcher")
		}
		// Log the error, we will return the earlier error.
		testing.ContextLog(ctx, "Failed to close SignalWatcher after another failure: ", err)
	}
	return w.err
}

func parseAuthEnrollmentProgress(sig *dbus.Signal) (*FingerprintEnrollmentSignal, error) {
	out := uda.PrepareAuthFactorProgress{}
	var marshOut []byte

	if err := dbus.Store(sig.Body, &marshOut); err != nil {
		return nil, errors.Wrap(err, "failed reading PrepareAuthFactorProgress signal to bytes")
	}
	if err := proto.Unmarshal(marshOut, &out); err != nil {
		return nil, errors.Wrap(err, "failed unmarshaling PrepareAuthFactorProgress response")
	}
	gotPurpose, expectedPurpose := out.Purpose, uda.AuthFactorPreparePurpose_PURPOSE_ADD_AUTH_FACTOR
	if gotPurpose != expectedPurpose {
		return nil, errors.Errorf("unexpected PrepareAuthFactorProgress purpose: got %v, expected %v", gotPurpose, expectedPurpose)
	}
	gotType, expectedType := out.GetAddProgress().AuthFactorType, uda.AuthFactorType_AUTH_FACTOR_TYPE_FINGERPRINT
	if gotType != expectedType {
		return nil, errors.Errorf("unexpected PrepareAuthFactorProgress auth factor type: got %v, expected %v", gotType, expectedType)
	}
	authEnrollment := out.GetAddProgress().GetBiometricsProgress()
	res := &FingerprintEnrollmentSignal{}
	res.ScanResult = authEnrollment.ScanResult.GetFingerprintResult()
	res.Done = authEnrollment.Done
	res.PercentComplete = authEnrollment.GetFingerprintProgress().GetPercentComplete()
	return res, nil
}

func parseAuthScanDoneProgress(sig *dbus.Signal) (*FingerprintAuthenticationSignal, error) {
	out := uda.PrepareAuthFactorProgress{}
	var marshOut []byte

	if err := dbus.Store(sig.Body, &marshOut); err != nil {
		return nil, errors.Wrap(err, "failed reading PrepareAuthFactorProgress signal to bytes")
	}
	if err := proto.Unmarshal(marshOut, &out); err != nil {
		return nil, errors.Wrap(err, "failed unmarshaling PrepareAuthFactorProgress response")
	}
	gotPurpose, expectedPurpose := out.Purpose, uda.AuthFactorPreparePurpose_PURPOSE_AUTHENTICATE_AUTH_FACTOR
	if gotPurpose != expectedPurpose {
		return nil, errors.Errorf("unexpected PrepareAuthFactorProgress purpose: got %v, expected %v", gotPurpose, expectedPurpose)
	}
	gotType, expectedType := out.GetAuthProgress().AuthFactorType, uda.AuthFactorType_AUTH_FACTOR_TYPE_FINGERPRINT
	if gotType != expectedType {
		return nil, errors.Errorf("unexpected PrepareAuthFactorProgress auth factor type: got %v, expected %v", gotType, expectedType)
	}
	authScanDone := out.GetAuthProgress().GetBiometricsProgress()
	res := &FingerprintAuthenticationSignal{}
	res.ScanResult = authScanDone.ScanResult.GetFingerprintResult()
	return res, nil
}
