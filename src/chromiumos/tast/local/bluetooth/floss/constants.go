// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package floss

const defaultFlossAdapterHCI = 0

// BtTransport enumerates different transport values, as specified by the
// floss D-Bus API.
type BtTransport uint32

const (
	// BtTransportAuto is the BtTransport value for Auto.
	BtTransportAuto BtTransport = 0

	// BtTransportBREDR is the BtTransport value for BREDR.
	BtTransportBREDR BtTransport = 1

	// BtTransportLE is the BtTransport value for LE.
	BtTransportLE BtTransport = 2
)

// BtSspVariant enumerates different SSP variant values, as specified by the
// floss D-Bus API.
type BtSspVariant uint32

const (
	// BtSspVariantPasskeyConfirmation is the BtSspVariant value for passkey
	// confirmation.
	BtSspVariantPasskeyConfirmation BtSspVariant = 0

	// BtSspVariantPasskeyEntry is the BtSspVariant value for passkey entry.
	BtSspVariantPasskeyEntry BtSspVariant = 1

	// BtSspVariantConsent is the BtSspVariant value for consent.
	BtSspVariantConsent BtSspVariant = 2

	// BtSspVariantPasskeyNotification is the BtSspVariant value for passkey
	// notification.
	BtSspVariantPasskeyNotification BtSspVariant = 3
)

// BtBondState enumerates different bond state values, as specified by the
// floss D-Bus API.
type BtBondState uint32

const (
	// BtBondStateNotBonded is the BtBondState value for the not bonded state.
	BtBondStateNotBonded BtBondState = 0

	// BtBondStateBonding is the BtBondState value for the bonding state.
	BtBondStateBonding BtBondState = 1

	// BtBondStateBonded is the BtBondState value for the bonded state.
	BtBondStateBonded BtBondState = 2
)

// BtConnectionState enumerates different connection state values, as specified
// by the floss D-Bus API.
type BtConnectionState uint32

const (
	// BtConnectionStateNotConnected is the BtConnectionState value for the not
	// connected state.
	BtConnectionStateNotConnected BtConnectionState = 0

	// BtConnectionStateConnectedOnly is the BtConnectionState value for the
	// connected only state.
	BtConnectionStateConnectedOnly BtConnectionState = 1

	// BtConnectionStateEncryptedBREDR is the BtConnectionState value for the
	// encrypted BREDR state.
	BtConnectionStateEncryptedBREDR BtConnectionState = 3

	// BtConnectionStateLE is the BtConnectionState value for the LE state.
	BtConnectionStateLE BtConnectionState = 5

	// BtConnectionStateBoth is the BtConnectionState value for the both state.
	BtConnectionStateBoth BtConnectionState = 7
)

// BtACLState enumerates different ACL state values, as specified by the
// floss D-Bus API.
type BtACLState uint32

const (
	// BtACLStateConnected is the BtACLState value for the connected state.
	BtACLStateConnected BtACLState = 0

	// BtACLStateDisconnected is the BtACLState value for the disconnected state.
	BtACLStateDisconnected BtACLState = 1
)

// BtDeviceType enumerates different device type values, as specified by the
// floss D-Bus API.
type BtDeviceType uint32

const (
	// BtDeviceTypeUnknown is the BtDeviceType value for the unknown device type.
	BtDeviceTypeUnknown BtDeviceType = 0

	// BtDeviceTypeBREDR is the BtDeviceType value for the BREDR device type.
	BtDeviceTypeBREDR BtDeviceType = 1

	// BtDeviceTypeBLE is the BtDeviceType value for the BLE device type.
	BtDeviceTypeBLE BtDeviceType = 2

	// BtDeviceTypeDual is the BtDeviceType value for the dual device type.
	BtDeviceTypeDual BtDeviceType = 3
)

// BtStatus enumerates different bluetooth status values, as specified by the
// floss D-Bus API.
type BtStatus uint32

const (
	// BtStatusSuccess is the BtStatus value for the success status.
	BtStatusSuccess BtStatus = 0

	// BtStatusFail is the BtStatus value for the fail status.
	BtStatusFail BtStatus = 1

	// BtStatusNotReady is the BtStatus value for the not ready status.
	BtStatusNotReady BtStatus = 2

	// BtStatusNoMemory is the BtStatus value for the no memory status.
	BtStatusNoMemory BtStatus = 3

	// BtStatusBusy is the BtStatus value for the busy status.
	BtStatusBusy BtStatus = 4

	// BtStatusDone is the BtStatus value for the done status.
	BtStatusDone BtStatus = 5

	// BtStatusUnsupported is the BtStatus value for the unsupported status.
	BtStatusUnsupported BtStatus = 6

	// BtStatusInvalidParam is the BtStatus value for the invalid param status.
	BtStatusInvalidParam BtStatus = 7

	// BtStatusUnhandled is the BtStatus value for the unhandled status.
	BtStatusUnhandled BtStatus = 8

	// BtStatusAuthFailure is the BtStatus value for the auth failure status.
	BtStatusAuthFailure BtStatus = 9

	// BtStatusRemoteDeviceDown is the BtStatus value for the remote device down
	// status.
	BtStatusRemoteDeviceDown BtStatus = 10

	// BtStatusAuthRejected is the BtStatus value for the auth rejected status.
	BtStatusAuthRejected BtStatus = 11

	// BtStatusJniEnvironmentError is the BtStatus value for the JNI environment
	// error status.
	BtStatusJniEnvironmentError BtStatus = 12

	// BtStatusJniThreadAttachError is the BtStatus value for the JNI thread
	// attach error status.
	BtStatusJniThreadAttachError BtStatus = 13

	// BtStatusWakeLockError is the BtStatus value for the wake lock error status.
	BtStatusWakeLockError BtStatus = 14

	// BtStatusUnknown is the BtStatus value for the unknown status.
	BtStatusUnknown BtStatus = 0xff
)

// BtConnectionDirection enumerates different connection direction values, as
// specified by the floss D-Bus API.
type BtConnectionDirection uint32

const (
	// BtConnectionDirectionUnknown is the BtConnectionDirection for the unknown
	// connection direction.
	BtConnectionDirectionUnknown BtConnectionDirection = 0

	// BtConnectionDirectionOutgoing is the BtConnectionDirection for the outgoing
	// connection direction.
	BtConnectionDirectionOutgoing BtConnectionDirection = 1

	// BtConnectionDirectionIncoming is the BtConnectionDirection for the incoming
	// connection direction.
	BtConnectionDirectionIncoming BtConnectionDirection = 2
)

// BtDiscoverableMode enumerates the different discoverable modes, as specified
// by the floss D-Bus API.
// ref: system/gd/rust/topshim/src/btif.rs
type BtDiscoverableMode uint32

const (
	// BtDiscoverableModeNon is the BtDiscoverableMode for disabling
	// discoverability.
	BtDiscoverableModeNon BtDiscoverableMode = 0

	// BtDiscoverableModeLimit is the BtDiscoverableMode for limited
	// discoverability (60s maximum timeout).
	BtDiscoverableModeLimit BtDiscoverableMode = 1

	// BtDiscoverableModeGeneral is the BtDiscoverableMode for general
	// discoverability.
	BtDiscoverableModeGeneral BtDiscoverableMode = 2
)
