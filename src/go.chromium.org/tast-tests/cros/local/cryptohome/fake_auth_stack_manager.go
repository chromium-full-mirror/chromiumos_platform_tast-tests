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
	dbusAuthIface   = "org.chromium.BiometricsDaemon.AuthSession"
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

// AuthScanResult represents one fingerprint authentication result.
type AuthScanResult struct {
	AuthCredStatus messages.AuthenticateCredentialReply_AuthenticateCredentialStatus
	ScanResult     messages.ScanResult
}

// FakeAuthStackManager is a testing implementation of the
// org.chromium.BiometricsDaemon.AuthStackManager D-Bus interface.
type FakeAuthStackManager struct {
	ctx             context.Context
	dbusConn        *dbus.Conn
	dbusPath        string
	dbusIface       string
	sessionDBusPath string
	sessionState    sessionState
	// ECDH private key for establish encrypted session with cryptohome.
	// Initialized once and never change.
	privateKey *ecdh.PrivateKey
	// Auth secret to be used by cryptohome for key derivation.
	// Initialized once and never change.
	authSecret []byte
	// recordID identifies the fingerprint template being enrolled or authenticated.
	// Should be set prior to enroll or auth session.
	recordID string
	// createCredStatus is returned as the status in CreateCredentialReply.
	// Should be set prior to enroll session.
	createCredStatus *messages.CreateCredentialReply_CreateCredentialStatus
	// EnrollmentProgreses should be set prior to enroll session.
	enrollmentProgresses []EnrollmentProgress
	// Entries of authScanResults are consume one by one to be returned in AuthenticateCredentialReply.
	// Should be set prior to auth session.
	authScanResults []AuthScanResult

	// mutex ensures only one thread is running at any given time.
	// Also it ensures no concurrent DBus method call will be running
	// in parallel to change the state of the auth stack.
	mutex sync.Mutex
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

// GetNonce handles the incoming same name D-Bus call. It returns a random nonce.
func (m *FakeAuthStackManager) GetNonce() ([]byte, *dbus.Error) {
	nonce := make([]byte, 32)
	rand.Read(nonce)
	reply := messages.GetNonceReply{
		Nonce: nonce,
	}
	marshaledReply, err := proto.Marshal(&reply)
	if err != nil {
		return nil, dbus.MakeFailedError(errors.Wrap(err, "failed to marshal the response"))
	}
	return marshaledReply, nil
}

// StartEnrollSession handles the incoming same name D-Bus call. It starts listening on a specific dbus
// path for the enroll session.
func (m *FakeAuthStackManager) StartEnrollSession(request []byte) (dbus.ObjectPath, *dbus.Error) {
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

// StartAuthSession handles the incoming same name D-Bus call. It starts listening on a specific dbus
// path for the auth session.
func (m *FakeAuthStackManager) StartAuthSession(reqBytes []byte) (dbus.ObjectPath, *dbus.Error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.sessionDBusPath != "" || m.sessionState != noSession {
		return "", dbus.MakeFailedError(errors.New("cannot start concurrent auth session"))
	}
	m.sessionDBusPath = m.dbusPath + "/AuthSession"
	if err := m.dbusConn.Export(m, dbus.ObjectPath(m.sessionDBusPath), dbusAuthIface); err != nil {
		return "", dbus.MakeFailedError(errors.Wrap(err, "failed to listen on AuthSession path"))
	}
	m.sessionState = authSession

	go m.emitAuthSignalAndTerminate()
	return dbus.ObjectPath(m.sessionDBusPath), nil
}

// End handles the incoming same name D-Bus call. It stops the auth session.
func (m *FakeAuthStackManager) End() *dbus.Error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if err := m.terminateAuthSession(); err != nil {
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

func (m *FakeAuthStackManager) terminateAuthSession() error {
	defer m.resetSessionState()
	if m.sessionDBusPath != "" && m.sessionState == authSession {
		if err := m.dbusConn.Export(nil, dbus.ObjectPath(m.sessionDBusPath), dbusAuthIface); err != nil {
			return errors.Wrap(err, "failed to terminate auth session interface")
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
	var request messages.CreateCredentialRequestV2
	if err := proto.Unmarshal(reqBytes, &request); err != nil {
		return nil, dbus.MakeFailedError(errors.Wrap(err, "failed to unmarshal request"))
	}

	// cryptohome's PublicKey comes from the request.
	cpub, err := fpPublicKeyToEcdhPublicKey(request.Pub)
	if err != nil {
		return nil, dbus.MakeFailedError(errors.Wrap(err, "failed to convert public key from the request"))
	}

	var iv, encryptedSecret []byte
	if m.createCredStatus != nil && *m.createCredStatus == messages.CreateCredentialReply_SUCCESS {
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
	reply := messages.CreateCredentialReply{
		Status:          m.createCredStatus,
		EncryptedSecret: encryptedSecret,
		Iv:              iv,
		Pub:             fpPubKey,
		RecordId:        &m.recordID,
	}
	marshaledReply, err := proto.Marshal(&reply)
	if err != nil {
		return nil, dbus.MakeFailedError(errors.Wrap(err, "failed to marshal the response"))
	}
	m.createCredStatus = nil
	m.recordID = ""
	return marshaledReply, nil
}

// AuthenticateCredential handles the incoming same name D-Bus call. It will consume entries in |authScanResults| one by one.
func (m *FakeAuthStackManager) AuthenticateCredential(reqBytes []byte) ([]byte, *dbus.Error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.sessionState != noSession {
		return nil, dbus.MakeFailedError(errors.New("cannot respond to AuthenticateCredential with active sessions"))
	}
	var request messages.AuthenticateCredentialRequestV2
	if err := proto.Unmarshal(reqBytes, &request); err != nil {
		return nil, dbus.MakeFailedError(errors.Wrap(err, "failed to unmarshal request"))
	}

	// cryptohome's PublicKey comes from the request.
	cpub, err := fpPublicKeyToEcdhPublicKey(request.Pub)
	if err != nil {
		return nil, dbus.MakeFailedError(errors.Wrap(err, "failed to convert public key from the request"))
	}

	var reply messages.AuthenticateCredentialReply
	// When there is no pending auth scan result, set the status to report an incorrect state.
	if len(m.authScanResults) == 0 {
		authStatus := messages.AuthenticateCredentialReply_INCORRECT_STATE
		reply.Status = &authStatus
	} else {
		result := m.authScanResults[0]
		m.authScanResults = m.authScanResults[1:]
		var iv, encryptedSecret []byte
		var recordID string
		if result.AuthCredStatus == messages.AuthenticateCredentialReply_SUCCESS &&
			result.ScanResult == messages.ScanResult_SCAN_RESULT_SUCCESS {
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

			// consume the record id
			recordID = m.recordID
			m.recordID = ""
		}

		fpPubKey := ecdhPublicKeyToFpPublicKey(m.privateKey.PublicKey())
		reply = messages.AuthenticateCredentialReply{
			Status:          &result.AuthCredStatus,
			ScanResult:      &result.ScanResult,
			EncryptedSecret: encryptedSecret,
			Iv:              iv,
			Pub:             fpPubKey,
			RecordId:        &recordID,
		}
	}
	marshaledReply, err := proto.Marshal(&reply)
	if err != nil {
		return nil, dbus.MakeFailedError(errors.Wrap(err, "failed to marshal the response"))
	}
	return marshaledReply, nil
}

// DeleteCredential handles the incoming same name D-Bus call.
func (m *FakeAuthStackManager) DeleteCredential(reqBytes []byte) ([]byte, *dbus.Error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	var request messages.DeleteCredentialRequest
	if err := proto.Unmarshal(reqBytes, &request); err != nil {
		return nil, dbus.MakeFailedError(errors.Wrap(err, "failed to unmarshal request"))
	}
	statusSuccess := messages.DeleteCredentialReply_SUCCESS
	statusFailure := messages.DeleteCredentialReply_DELETION_FAILED
	var reply messages.DeleteCredentialReply
	if m.recordID != "" && m.recordID == *request.RecordId {
		reply.Status = &statusSuccess
	} else {
		reply.Status = &statusFailure
	}
	marshaledReply, err := proto.Marshal(&reply)
	if err != nil {
		return nil, dbus.MakeFailedError(errors.Wrap(err, "failed to marshal the response"))
	}
	m.recordID = ""
	return marshaledReply, nil
}

// Close disconnects all FakeAuthStackManager services from D-Bus.
func (m *FakeAuthStackManager) Close() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Close the enroll session interface.
	if m.sessionState == enrollSession {
		if err := m.terminateEnrollSession(); err != nil {
			testing.ContextLog(m.ctx, "Failed to unregister the biod enroll session service: ", err)
		}
	}

	// Close the auth session interface.
	if m.sessionState == authSession {
		if err := m.terminateAuthSession(); err != nil {
			testing.ContextLog(m.ctx, "Failed to unregister the biod auth session service: ", err)
		}
	}

	// Close the main interface.
	if err := m.dbusConn.Export(nil, dbus.ObjectPath(m.dbusPath), m.dbusIface); err != nil {
		testing.ContextLog(m.ctx, "Failed to unregister the biod service: ", err)
	}
}

// SetRecordID sets |recordID|.
func (m *FakeAuthStackManager) SetRecordID(id string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.recordID = id
}

// GetRecordID returns |recordID|.
func (m *FakeAuthStackManager) GetRecordID() string {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.recordID
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

// SetAuthScanResults sets |authScanResults|.
func (m *FakeAuthStackManager) SetAuthScanResults(r []AuthScanResult) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.authScanResults = r
}

// GetAuthScanResults returns |authScanResults|.
func (m *FakeAuthStackManager) GetAuthScanResults() []AuthScanResult {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.authScanResults
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

// emitAuthSignalAndTerminate emits a auth signal and terminates the auth session afterwards
// when there is at least one pending scan results.
func (m *FakeAuthStackManager) emitAuthSignalAndTerminate() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if len(m.authScanResults) > 0 {
		marshaledSignal, err := proto.Marshal(&messages.AuthScanDone{})
		if err != nil {
			testing.ContextLog(m.ctx, "Failed to marshal AuthScanDone: ", err)
		}
		if err := m.dbusConn.Emit(dbus.ObjectPath(m.dbusPath), m.dbusIface+".AuthScanDone", marshaledSignal); err != nil {
			testing.ContextLog(m.ctx, "Failed to emit signal: ", err)
		}
		if err := m.terminateAuthSession(); err != nil {
			testing.ContextLog(m.ctx, "Failed to unregister the biod auth session service: ", err)
		}
		m.resetSessionState()
	}
}

// emitEnrollment emits an EnrollScanDone signal.
func (m *FakeAuthStackManager) emitEnrollment(p EnrollmentProgress) {
	done := p.Percentage >= 100
	marshaledSignal, err := proto.Marshal(&messages.EnrollScanDone{
		ScanResult:      &p.ScanResult,
		PercentComplete: &p.Percentage,
		Done:            &done,
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
