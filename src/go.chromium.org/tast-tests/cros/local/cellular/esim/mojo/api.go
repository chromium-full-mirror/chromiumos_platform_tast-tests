// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mojo

import (
	"context"
	"sync"
	"time"
	"unicode/utf16"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
)

// ProfileState is the current state of an ESimProfile.
type ProfileState int32

// ProfileInstallResult is the result code for ESimProfile installation.
type ProfileInstallResult int32

// ESimOperationResult is the result code for operations on Euicc and ESimProfile.
type ESimOperationResult int32

const (
	// ProfileStatePending indicates the profile is not installed on the device.
	ProfileStatePending ProfileState = iota
	// ProfileStateInstalling indicates the profile is being installed.
	ProfileStateInstalling
	// ProfileStateInactive indicates the profile is installed but inactive.
	ProfileStateInactive
	// ProfileStateActive indicates the profile is installed and active.
	ProfileStateActive
)

const (
	// ProfileInstallSuccess indicates the profile installation succeeded.
	ProfileInstallSuccess ProfileInstallResult = iota
	// ProfileInstallFailure indicates the profile installation failed.
	ProfileInstallFailure
	// ProfileInstallErrorNeedsConfirmationCode indicates the installation requires a valid confirmation code.
	ProfileInstallErrorNeedsConfirmationCode
	// ProfileInstallErrorInvalidActivationCode indicates the given activation code is invalid.
	ProfileInstallErrorInvalidActivationCode
)

const (
	// ESimOperationSuccess indicates the operation succeeded.
	ESimOperationSuccess ESimOperationResult = iota
	// ESimOperationFailure indicates the operation failed.
	ESimOperationFailure
)

// ESimManager provides access to the Mojo eSIM management methods.
type ESimManager struct {
	cr    *chrome.Chrome
	tconn *chrome.TestConn
	mutex sync.Mutex
}

// Euicc represents an EUICC (Embedded Universal Integrated
// Circuit Card) hardware available on the device and provides operations
// on the EUICC.
type Euicc struct {
	manager *ESimManager
	Eid     string
}

// EuiccProperties are the properties for an Euicc object.
type EuiccProperties struct {
	Eid      string `json:"eid"`
	IsActive bool   `json:"isActive"`
}

// QRCode represents a QRCode image.
type QRCode struct {
	Size uint8   `json:"size"`
	Data []uint8 `json:"data"`
}

// ESimProfile represents an eSIM profile and provides operations
// on the profile.
type ESimProfile struct {
	manager *ESimManager
	Iccid   string
}

// String16 represents a UTF-16 string.
type String16 struct {
	Data []uint16 `json:"data"`
}

// ESimProfileProperties are the properties of an eSIM profile object.
type ESimProfileProperties struct {
	Eid             string       `json:"eid"`
	Iccid           string       `json:"iccid"`
	Name            String16     `json:"name"`
	Nickname        String16     `json:"nickname"`
	ServiceProvider String16     `json:"serviceProvider"`
	State           ProfileState `json:"state"`
	ActivationCode  string       `json:"activationCode"`
}

// NewESimManager returns an ESimManager instance.
func NewESimManager(cr *chrome.Chrome, tconn *chrome.TestConn) *ESimManager {
	return &ESimManager{cr: cr, tconn: tconn}
}

// Call calls the given JavaScript function on the "eSIM manager JavaScript object".
// It establishes an "eSIM manager JavaScript object" on demand and properly releases it when the jobs are done.
// This function launches OS Settings and navigates to the internet page during its runtime to create the JavaScript object,
// then closes OS Settings after the function completes.
func (m *ESimManager) Call(ctx context.Context, out interface{}, fn string, args ...interface{}) error {
	if m.cr == nil || m.tconn == nil {
		return errors.New("invalid Chrome instance or test API connection")
	}

	// Ensure the function is thread-safe at runtime.
	m.mutex.Lock()
	defer m.mutex.Unlock()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	condition := uiauto.New(m.tconn).Exists(ossettings.Internet)
	settings, err := ossettings.LaunchAtPageURL(ctx, m.tconn, m.cr, "internet", condition)
	if err != nil {
		return errors.Wrap(err, "failed to open settings app")
	}
	defer settings.Close(cleanupCtx)

	conn, err := settings.ChromeConn(ctx, m.cr)
	if err != nil {
		return errors.Wrap(err, "failed to create connection to settings app")
	}
	defer conn.Close()

	var js chrome.JSObject
	if err := conn.Call(ctx, &js, ESimManagerJS); err != nil {
		return errors.Wrap(err, "failed to create eSIM mojo JS object")
	}
	defer js.Release(cleanupCtx)

	return js.Call(ctx, out, fn, args...)
}

/*
   Wrapper functions around eSIM mojo JS calls.
*/

// AvailableEuicc returns a list of Euiccs available on the device.
func (m *ESimManager) AvailableEuicc(ctx context.Context) ([]Euicc, error) {
	var result []string

	js := "function() {return this.getAvailableEuiccEids()}"
	if err := m.Call(ctx, &result, js); err != nil {
		return nil, errors.Wrap(err, "getAvailableEuiccs call failed")
	}

	euiccs := make([]Euicc, len(result))
	for i, id := range result {
		euiccs[i] = Euicc{manager: m, Eid: id}
	}

	return euiccs, nil
}

// Properties returns properties struct for this Euicc.
func (e *Euicc) Properties(ctx context.Context) (EuiccProperties, error) {
	var result EuiccProperties

	js := `function(eid) {return this.getEuiccProperties(eid)}`
	if err := e.manager.Call(ctx, &result, js, e.Eid); err != nil {
		return result, errors.Wrap(err, "getProperties call failed")
	}

	return result, nil
}

// ProfileList returns a list of all profiles installed or pending on this Euicc.
func (e *Euicc) ProfileList(ctx context.Context) ([]ESimProfile, error) {
	var result []string

	js := "function(eid) {return this.getProfileIccids(eid)}"
	if err := e.manager.Call(ctx, &result, js, e.Eid); err != nil {
		return nil, errors.Wrap(err, "getProfileIccids call failed")
	}

	profiles := make([]ESimProfile, len(result))
	for i, id := range result {
		profiles[i] = ESimProfile{manager: e.manager, Iccid: id}
	}

	return profiles, nil
}

// RequestAvailableProfiles starts a request for available profilesfor this
// Euicc from SMDS. Returns a status indicating the result of the operation
// and the combined list of all profiles doung across all SM-DS servers.
func (e *Euicc) RequestAvailableProfiles(ctx context.Context) (ESimOperationResult, []ESimProfileProperties, error) {
	var result struct {
		Result                    ESimOperationResult
		ESimProfilePropertiesList []ESimProfileProperties
	}

	js := "function(eid) {return this.requestAvailableProfiles(eid)}"
	if err := e.manager.Call(ctx, &result, js, e.Eid); err != nil {
		return ESimOperationFailure, nil, errors.Wrap(err, "requestAvailableProfiles call failed")
	}

	return result.Result, result.ESimProfilePropertiesList, nil
}

// RequestPendingProfiles starts a request for pending profiles for this
// Euicc from SMDS. Returns a status indicating result of the operation.
func (e *Euicc) RequestPendingProfiles(ctx context.Context) (ESimOperationResult, error) {
	var result ESimOperationResult

	js := "function(eid) {return this.requestPendingProfiles(eid)}"
	if err := e.manager.Call(ctx, &result, js, e.Eid); err != nil {
		return result, errors.Wrap(err, "requestPendingProfiles call failed")
	}

	return result, nil
}

// InstallProfileFromActivationCode installs a profile with given activation_code
// and confirmation_code on this Euicc. Returns the  result code for the operation.
func (e *Euicc) InstallProfileFromActivationCode(
	ctx context.Context,
	activationCode, confirmationCode string) (ProfileInstallResult, *ESimProfile, error) {
	var result struct {
		Result ProfileInstallResult
		Iccid  string
	}

	js := "function(eid, ac, cc) {return this.installProfileFromActivationCode(eid, ac, cc)}"
	if err := e.manager.Call(ctx, &result, js, e.Eid, activationCode, confirmationCode); err != nil {
		return result.Result, nil, errors.Wrap(err, "installProfileFromActivationCode call failed")
	}

	return result.Result, &ESimProfile{manager: e.manager, Iccid: result.Iccid}, nil
}

// EidQRCode returns a QR Code image representing the EID of this Euicc.
// A null value is returned if the QR Code could not be generated.
func (e *Euicc) EidQRCode(ctx context.Context) (QRCode, error) {
	var result QRCode

	js := "function(eid) {return this.getEidQrCode(eid)}"
	if err := e.manager.Call(ctx, &result, js, e.Eid); err != nil {
		return result, errors.Wrap(err, "getEidQrCode call failed")
	}

	return result, nil
}

// NewString16 creates a String16 from a string.
func NewString16(s string) String16 {
	return String16{Data: utf16.Encode([]rune(s))}
}

func (s String16) String() string {
	return string(utf16.Decode(s.Data))
}

// Properties returns properties struct for this ESimProfile.
func (e *ESimProfile) Properties(ctx context.Context) (ESimProfileProperties, error) {
	var result ESimProfileProperties

	js := "function(iccid) {return this.getProfileProperties(iccid)}"
	if err := e.manager.Call(ctx, &result, js, e.Iccid); err != nil {
		return result, errors.Wrap(err, "getProfileProperties call failed")
	}

	return result, nil
}

// InstallProfile installs this eSIM profile with given confirmationCode.
// A non success result code is returned in case of errors.
func (e *ESimProfile) InstallProfile(ctx context.Context, confirmationCode string) (ProfileInstallResult, error) {
	var result ProfileInstallResult

	js := "function(iccid, cc) {return this.installProfile(iccid, cc)}"
	if err := e.manager.Call(ctx, &result, js, e.Iccid, confirmationCode); err != nil {
		return result, errors.Wrap(err, "installProfile call failed")
	}

	return result, nil
}

// UninstallProfile uninstalls this eSIM profile. Returns the result code for the operation.
func (e *ESimProfile) UninstallProfile(ctx context.Context) (ESimOperationResult, error) {
	var result ESimOperationResult

	js := "function(iccid) {return this.uninstallProfile(iccid)}"
	if err := e.manager.Call(ctx, &result, js, e.Iccid); err != nil {
		return result, errors.Wrap(err, "uninstallProfile call failed")
	}

	return result, nil
}

// SetProfileNickname sets a nickname for this eSIM profile. Returns
// the result code for the operation.
func (e *ESimProfile) SetProfileNickname(ctx context.Context, nickname String16) (ESimOperationResult, error) {
	var result ESimOperationResult

	js := "function(iccid, name) {return this.setProfileNickname(iccid, name)}"
	if err := e.manager.Call(ctx, &result, js, e.Iccid, nickname); err != nil {
		return result, errors.Wrap(err, "setProfileNickname call failed")
	}

	return result, nil
}
