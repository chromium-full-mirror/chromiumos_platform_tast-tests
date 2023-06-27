// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"math/big"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/golang/protobuf/proto"

	messages "chromiumos/system_api/biod_messages_proto"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	dbusPath        = "/org/chromium/BiometricsDaemon/CrosFpAuthStackManager"
	dbusIface       = "org.chromium.BiometricsDaemon.AuthStackManager"
	dbusEnrollIface = "org.chromium.BiometricsDaemon.EnrollSession"
)

type sessionState int

const (
	noSession sessionState = iota
	enrollSession
	authSession
)

// EnrollmentProgress represents the fingerprint enrollment progress signals.
type EnrollmentProgress struct {
	Percentage int32
	ScanResult messages.ScanResult
}

// FakeAuthStackManager is a testing implementation of the
// org.chromium.BiometricsDaemon.AuthStackManager D-Bus interface.
type FakeAuthStackManager struct {
	ctx                  context.Context
	dbusConn             *dbus.Conn
	dbusPath             string
	dbusIface            string
	sessionDBusPath      string
	sessionState         sessionState
	privateKey           *ecdh.PrivateKey
	authSecret           []byte
	createCredStatus     *messages.CreateCredentialReply_CreateCredentialStatus
	enrollmentProgresses []EnrollmentProgress
	mutex                sync.Mutex
}

func fpPublicKeyToEcdhPublicKey(pub *messages.FpPublicKey) (*ecdh.PublicKey, error) {
	var x, y big.Int
	x.SetBytes(pub.X)
	y.SetBytes(pub.Y)
	return ecdh.P256().NewPublicKey(elliptic.Marshal(elliptic.P256(), &x, &y))
}

func ecdhPublicKeyToFpPublicKey(pub *ecdh.PublicKey) *messages.FpPublicKey {
	var opub messages.FpPublicKey
	marshaledBytes := pub.Bytes()
	x, y := elliptic.Unmarshal(elliptic.P256(), marshaledBytes)
	opub.X = x.Bytes()
	opub.Y = y.Bytes()
	return &opub
}

// StartEnrollSession handles the incoming same name D-Bus call. It starts listening on a specific dbus
// path for the enroll session.
func (m *FakeAuthStackManager) StartEnrollSession() (dbus.ObjectPath, *dbus.Error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.sessionDBusPath != "" || m.sessionState != noSession {
		return "", dbus.MakeFailedError(errors.New("cannot start concurrent enroll session"))
	}
	m.sessionDBusPath = m.dbusPath + "/EnrollSession"
	if err := m.dbusConn.Export(m, dbus.ObjectPath(m.sessionDBusPath), dbusEnrollIface); err != nil {
		return "", dbus.MakeFailedError(errors.Wrap(err, "failed to listen on EnrollSession path"))
	}
	m.sessionState = enrollSession

	go m.emitEnrollmentSignals()
	return dbus.ObjectPath(m.sessionDBusPath), nil
}

// Cancel handles the incoming same name D-Bus call. It stops the enrollment session.
func (m *FakeAuthStackManager) Cancel() *dbus.Error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if err := m.terminateEnrollSession(); err != nil {
		return dbus.MakeFailedError(err)
	}
	return nil
}

func (m *FakeAuthStackManager) terminateEnrollSession() error {
	defer m.resetSessionState()
	if m.sessionDBusPath != "" && m.sessionState == enrollSession {
		if err := m.dbusConn.Export(nil, dbus.ObjectPath(m.sessionDBusPath), dbusEnrollIface); err != nil {
			return errors.Wrap(err, "failed to terminate enroll session interface")
		}
	}
	return nil
}

func (m *FakeAuthStackManager) resetSessionState() {
	m.sessionDBusPath = ""
	m.sessionState = noSession
}

// CreateCredential handles the incoming same name D-Bus call. It will consume the manager's |createCredStatus|.
func (m *FakeAuthStackManager) CreateCredential(reqBytes []byte) ([]byte, *dbus.Error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.sessionState != enrollSession {
		return nil, dbus.MakeFailedError(errors.New("cannot respond to CreateCredential outside active enroll sessions"))
	}
	var request messages.CreateCredentialRequest
	if err := proto.Unmarshal(reqBytes, &request); err != nil {
		return nil, dbus.MakeFailedError(errors.Wrap(err, "failed to unmarshal request"))
	}

	// cryptohome's PublicKey comes from the request.
	cpub, err := fpPublicKeyToEcdhPublicKey(request.Pub)
	if err != nil {
		return nil, dbus.MakeFailedError(errors.Wrap(err, "failed to convert public key from the request"))
	}

	var iv, encryptedSecret []byte
	if *m.createCredStatus == messages.CreateCredentialReply_SUCCESS {
		sharedSecret, err := m.privateKey.ECDH(cpub)
		if err != nil {
			return nil, dbus.MakeFailedError(errors.Wrap(err, "failed to compute the shared secret"))
		}
		aesKey := sha256.Sum256(sharedSecret)
		block, err := aes.NewCipher(aesKey[:])
		if err != nil {
			return nil, dbus.MakeFailedError(errors.Wrap(err, "failed to create aes block"))
		}

		// generate random iv, and encrypt |AuthSecret| into |encryptedSecret|
		iv = make([]byte, aes.BlockSize)
		encryptedSecret = make([]byte, 32)
		rand.Read(iv)
		stream := cipher.NewCTR(block, iv)
		stream.XORKeyStream(encryptedSecret, m.authSecret)
	}

	fpPubKey := ecdhPublicKeyToFpPublicKey(m.privateKey.PublicKey())
	recordID := "fake record id"
	reply := messages.CreateCredentialReply{
		Status:          m.createCredStatus,
		EncryptedSecret: encryptedSecret,
		Iv:              iv,
		Pub:             fpPubKey,
		RecordId:        &recordID,
	}
	marshaledReply, err := proto.Marshal(&reply)
	if err != nil {
		return nil, dbus.MakeFailedError(errors.Wrap(err, "failed to marshal the response"))
	}
	m.createCredStatus = nil
	return marshaledReply, nil
}

// Close disconnects all FakeAuthStackManager services from dbus.
func (m *FakeAuthStackManager) Close() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Close the enroll session interface.
	if err := m.terminateEnrollSession(); err != nil {
		testing.ContextLog(m.ctx, "Failed to unregister the biod enroll session service: ", err)
	}

	// Close the main interface.
	if err := m.dbusConn.Export(nil, dbus.ObjectPath(m.dbusPath), m.dbusIface); err != nil {
		testing.ContextLog(m.ctx, "Failed to unregister the biod service: ", err)
	}
}

// SetCreateCredStatus sets |createCredStatus|.
func (m *FakeAuthStackManager) SetCreateCredStatus(status *messages.CreateCredentialReply_CreateCredentialStatus) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.createCredStatus = status
}

// GetCreateCredStatus returns |createCredStatus|.
func (m *FakeAuthStackManager) GetCreateCredStatus() *messages.CreateCredentialReply_CreateCredentialStatus {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.createCredStatus
}

// SetEnrollmentProgresses sets pre-defined EnrollmentProgresses.
func (m *FakeAuthStackManager) SetEnrollmentProgresses(p []EnrollmentProgress) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.enrollmentProgresses = p
}

// GetEnrollmentProgresses returns |enrollmentProgresses|.
func (m *FakeAuthStackManager) GetEnrollmentProgresses() []EnrollmentProgress {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.enrollmentProgresses
}

// emitEnrollmentSignals emits EnrollScanDone signals pre-defeined as []EnrollmentProgress.
// |enrollmentProgress| will be set to nil upon finish.
func (m *FakeAuthStackManager) emitEnrollmentSignals() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.sessionState != enrollSession {
		testing.ContextLog(m.ctx, "Cannot emit enrollment signals outside active enroll sessions")
		return
	}
	for _, p := range m.enrollmentProgresses {
		m.emitEnrollment(p)
	}
	m.enrollmentProgresses = nil
}

// emitEnrollment emits an EnrollScanDone signal.
func (m *FakeAuthStackManager) emitEnrollment(p EnrollmentProgress) {
	done := p.Percentage >= 100
	var nonce []byte
	if done && p.ScanResult == messages.ScanResult_SCAN_RESULT_SUCCESS {
		nonce = make([]byte, 32)
		rand.Read(nonce)
	}
	marshaledSignal, err := proto.Marshal(&messages.EnrollScanDone{
		ScanResult:      &p.ScanResult,
		PercentComplete: &p.Percentage,
		Done:            &done,
		AuthNonce:       nonce,
	})
	if err != nil {
		testing.ContextLog(m.ctx, "Failed to marshal EnrollScanDone: ", err)
	}
	err = m.dbusConn.Emit(dbus.ObjectPath(m.dbusPath), m.dbusIface+".EnrollScanDone", marshaledSignal)
	if err != nil {
		testing.ContextLog(m.ctx, "Failed to emit signal: ", err)
	}
}

// NewFakeAuthStackManager creates a FakeAuthStackManager, which starts to registers itself
// on dbus for biod methods calls. Close() must be called to properly clean up.
// Passed in ctx is kept with FakeAuthStackManager instance and will be used for
// logging, therefore it must be valid through the lifetime of FakeAuthStackManager.
func NewFakeAuthStackManager(ctx context.Context, dbusConn *dbus.Conn) (*FakeAuthStackManager, error) {
	privateKey, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	authSecret := make([]byte, 32)
	rand.Read(authSecret)
	m := FakeAuthStackManager{
		ctx:        ctx,
		dbusConn:   dbusConn,
		dbusPath:   dbusPath,
		dbusIface:  dbusIface,
		privateKey: privateKey,
		authSecret: authSecret,
	}

	if err := dbusConn.Export(&m, dbusPath, dbusIface); err != nil {
		return nil, err
	}
	return &m, nil
}
