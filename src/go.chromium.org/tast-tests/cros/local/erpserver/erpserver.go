// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package erpserver implements a local fake encrypted reporting server.
package erpserver

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strconv"
	"time"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/chromiumos/reporting"

	"go.chromium.org/tast-tests/cros/local/missiveutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"golang.org/x/crypto/chacha20"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
)

// MissiveDefaultFeature is the feature that should be passed to Chrome when
// creating a new instance for local testing.
const MissiveDeaultFeature = "CrOSLateBootMissiveStorage:signature_verification_dev_enabled/true/compression_enabled/false"

// SequenceInfo is a struct that maps to sequence info JSON in ERP upload requests and responses.
type SequenceInfo struct {
	GenerationID string `json:"generationId"`
	Priority     int    `json:"priority"`
	SequencingID string `json:"sequencingId"`
}

// RequestCompressionInfo is a struct that maps to ERP compression
// info in the upload request JSON.
type RequestCompressionInfo struct {
	CompressionAlgorithm int
}

// RequestEncryptionInfo is a struct that maps to ERP encryption
// info in the upload request JSON.
type RequestEncryptionInfo struct {
	EncryptionKey string
	PublicKeyID   string `json:"publicKeyId"`
}

// RequestEncryptedRecord is a struct that maps to ERP encrypted
// record data in the upload request JSON.
type RequestEncryptedRecord struct {
	EncryptedWrappedRecord string
	SequenceInformation    SequenceInfo
	CompressionInformation RequestCompressionInfo
	EncryptionInfo         RequestEncryptionInfo
}

// UploadRequest is a struct that maps to ERP upload requests JSON.
type UploadRequest struct {
	AttachEncryptionSettings bool
	ConfigurationFileVersion *int `json:"configurationFileVersion,omitempty"`
	EncryptedRecord          []RequestEncryptedRecord
}

// ResponseEncryptionSettings is a struct that maps to ERP encryption
// settings in the JSON response.
type ResponseEncryptionSettings struct {
	PublicKey          string `json:"publicKey"`
	PublicKeyID        int    `json:"publicKeyId"`
	PublicKeySignature string `json:"publicKeySignature"`
}

// ResponseConfigFile is a struct that maps to the ERP config file in the
// JSON response.
type ResponseConfigFile struct {
	Version             int           `json:"version"`
	EventConfigs        []EventConfig `json:"eventConfigs"`
	ConfigFileSignature string        `json:"configFileSignature"`
}

// EventConfig is a struct that maps to the ERP event config in the
// JSON response.
type EventConfig struct {
	Destination           string `json:"destination"`
	MinimumReleaseVersion int32  `json:"minimumReleaseVersion,omitempty"`
	MaximumReleaseVersion int32  `json:"maximumReleaseVersion,omitempty"`
}

// UploadResponse is a struct that maps to ERP upload responses JSON.
type UploadResponse struct {
	LastSucceedUploadedRecord *SequenceInfo               `json:"lastSucceedUploadedRecord,omitempty"`
	EncryptionSettings        *ResponseEncryptionSettings `json:"encryptionSettings,omitempty"`
	ConfigurationFile         *ResponseConfigFile         `json:"configurationFile,omitempty"`
}

// getKey decrypt the symemtric key used for record encryption using the private key.
func getKey(encryptedSecret string, privateKey []byte) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(encryptedSecret)
	if err != nil {
		return nil, errors.Wrap(err, "could not decode encrypted secret")
	}
	secret, err := curve25519.X25519(privateKey, decoded)
	if err != nil {
		return nil, err
	}
	kdf := hkdf.New(sha256.New, secret, nil, nil)
	key := make([]byte, chacha20.KeySize)
	io.ReadFull(kdf, key)
	return key, nil
}

// decrypt decrypts the encrypted record using key.
func decrypt(record string, key []byte) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(record)
	if err != nil {
		return nil, errors.Wrap(err, "could not decode encrypted record")
	}
	const overhead = 16
	nonce := decoded[:chacha20.NonceSize]
	cipherText := decoded[chacha20.NonceSize : len(decoded)-overhead]
	result := make([]byte, len(cipherText))
	cipher, err := chacha20.NewUnauthenticatedCipher(key[:], nonce)
	if err != nil {
		return nil, errors.Wrap(err, "could not create stream cipher")
	}
	cipher.SetCounter(1)
	cipher.XORKeyStream(result, cipherText)
	return result, nil
}

// recordData contains the decrypted record and sequence info for deduplication.
type recordData struct {
	record   *reporting.WrappedRecord
	sequence SequenceInfo
}

// ErpServer contains information about a fake Erp server instance.
type ErpServer struct {
	server           *httptest.Server
	keyID            int
	privateKey       []byte
	publicKeyEncoded string
	signatureEncoded string
	queue            chan recordData
	sequenceInfoList []SequenceInfo // List of sequence info of reported records that matches filter.
	filter           func(*reporting.WrappedRecord) bool
	fakeConfigFile   *ResponseConfigFile
}

// New creates a fake ERP server. If bufferSize is 0, default buffer size will be used.
func New(publicKey, privateKey, signature string, keyID, bufferSize int) (*ErpServer, error) {
	privateKeyDecoded, err := base64.StdEncoding.DecodeString(privateKey)
	if err != nil {
		return nil, errors.Wrap(err, "could not decode private key")
	}
	const defaultBufferSize = 10
	if bufferSize == 0 {
		bufferSize = defaultBufferSize
	}
	return &ErpServer{
		keyID:            keyID,
		privateKey:       privateKeyDecoded,
		publicKeyEncoded: publicKey,
		signatureEncoded: signature,
		queue:            make(chan recordData, bufferSize),
		filter: func(*reporting.WrappedRecord) bool {
			return true
		},
	}, nil
}

// SetFilter sets the filter used to decided which uploaded records should be stored.
func (erpserver *ErpServer) SetFilter(filter func(*reporting.WrappedRecord) bool) {
	erpserver.filter = filter
}

// SetFakeConfigFile sets a fake configuration file that will be returned to the
// client if requested.
func (erpserver *ErpServer) SetFakeConfigFile(configFile *ResponseConfigFile) {
	erpserver.fakeConfigFile = configFile
}

func (erpserver *ErpServer) handleUpload(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	testing.ContextLog(ctx, "ERP new request")
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		testing.ContextLog(ctx, "Could not read request body: ", err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	var request UploadRequest
	if err = json.Unmarshal(body, &request); err != nil {
		testing.ContextLog(ctx, "Could not parse request body: ", err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	var response UploadResponse
	if request.AttachEncryptionSettings {
		testing.ContextLog(ctx, "ERP attach encryption settings requested")
		response.EncryptionSettings = &ResponseEncryptionSettings{
			PublicKeyID:        erpserver.keyID,
			PublicKey:          erpserver.publicKeyEncoded,
			PublicKeySignature: erpserver.signatureEncoded,
		}
	}

	if request.ConfigurationFileVersion != nil {
		if erpserver.fakeConfigFile != nil && *request.ConfigurationFileVersion != erpserver.fakeConfigFile.Version {
			testing.ContextLog(ctx, "ERP attach configuration file requested, with version= ", *request.ConfigurationFileVersion)
			response.ConfigurationFile = erpserver.fakeConfigFile
		}
	}

	if len(request.EncryptedRecord) > 0 {
		testing.ContextLog(ctx, "ERP records upload requested")
		lastRecord := request.EncryptedRecord[len(request.EncryptedRecord)-1]
		response.LastSucceedUploadedRecord = &lastRecord.SequenceInformation
	}

	keyID := strconv.Itoa(erpserver.keyID)
	for _, encryptedRecord := range request.EncryptedRecord {
		publicKeyID := encryptedRecord.EncryptionInfo.PublicKeyID
		// Continue handling upcoming records on errors without returning an
		// http error to avoid blocking records needed to be verified by the test.
		if publicKeyID != keyID {
			testing.ContextLog(ctx, "Uknown public key ID: ", publicKeyID)
			continue
		}

		key, err := getKey(encryptedRecord.EncryptionInfo.EncryptionKey, erpserver.privateKey[:])
		if err != nil {
			testing.ContextLog(ctx, "Could not decrypt secret: ", err)
			continue
		}
		result, err := decrypt(encryptedRecord.EncryptedWrappedRecord, key)
		if err != nil {
			testing.ContextLog(ctx, "Could not decrypt record: ", err)
			continue
		}
		compression := encryptedRecord.CompressionInformation.CompressionAlgorithm
		if compression != 0 {
			testing.ContextLog(ctx, "Compressed records are not supported in fake server")
			continue
		}
		var record reporting.WrappedRecord
		if err = proto.Unmarshal(result, &record); err != nil {
			testing.ContextLog(ctx, "Could not parse decrypted record: ", err)
			continue
		}
		if erpserver.filter(&record) {
			erpserver.queue <- recordData{record: &record, sequence: encryptedRecord.SequenceInformation}
		}
	}

	responseJSON, err := json.Marshal(response)
	if err != nil {
		testing.ContextLog(ctx, "Could not encode response to json: ", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	if _, err := io.WriteString(w, string(responseJSON)); err != nil {
		testing.ContextLog(ctx, "Could not write response: ", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
}

// Start clears existing records and key on device and starts a new fake ERP server.
func (erpserver *ErpServer) Start(ctx context.Context) error {
	if erpserver.server != nil {
		return errors.New("server already started")
	}
	if err := missiveutil.ResetMissived(ctx); err != nil {
		return errors.Wrap(err, "could not prepare missived")
	}

	erpserver.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		erpserver.handleUpload(ctx, w, r)
	}))
	testing.ContextLog(ctx, "ERP fake server started on: ", erpserver.server.URL)
	return nil
}

// URL returns the server url if it was started.
func (erpserver *ErpServer) URL() string {
	return erpserver.server.URL + "/"
}

// NextRecord blocks until a new record is received and returns it.
func (erpserver *ErpServer) NextRecord() *reporting.WrappedRecord {
	for {
		// Block until a new record is uploaded and added.
		record := <-erpserver.queue
		// Search for the new sequence info in the existing sequence
		// list to avoid records duplication.
		exists := false
		for _, sequenceInfoItem := range erpserver.sequenceInfoList {
			if sequenceInfoItem.Priority == record.sequence.Priority &&
				sequenceInfoItem.GenerationID == record.sequence.GenerationID &&
				sequenceInfoItem.SequencingID == record.sequence.SequencingID {
				exists = true
				break
			}
		}
		// If the record does not exist add the sequence info to the list and return
		// the record, otherwise keep waiting for a new record to arrive.
		if !exists {
			erpserver.sequenceInfoList = append(erpserver.sequenceInfoList, SequenceInfo{
				GenerationID: record.sequence.GenerationID,
				Priority:     record.sequence.Priority,
				SequencingID: record.sequence.SequencingID,
			})
			return record.record
		}
	}
}

// NextRecordAsync waits for receiving a new record asynchronously and will
// timeout if it takes more than the specified timeout. If `expectEvents` is
// true, timeout will result in an error.
func (erpserver *ErpServer) NextRecordAsync(timeout time.Duration, expectEvents bool) (*reporting.WrappedRecord, error) {
	// Create a channel to communicate between the server's main thread and the thread that is responsible for receiving new records.
	recordChan := make(chan *recordData, 1)

	// Start a new goroutine to receive new records and add them to the channel.
	go func() {
		for {
			// Block until a new record is received or until the timeout expires.
			select {
			case record := <-erpserver.queue:
				// Search for the new sequence info in the existing sequence
				// list to avoid records duplication.
				exists := false
				for _, sequenceInfoItem := range erpserver.sequenceInfoList {
					if sequenceInfoItem.Priority == record.sequence.Priority &&
						sequenceInfoItem.GenerationID == record.sequence.GenerationID &&
						sequenceInfoItem.SequencingID == record.sequence.SequencingID {
						exists = true
						break
					}
				}

				// If the record does not exist add the sequence info to the list and add the record to the channel.
				if !exists {
					erpserver.sequenceInfoList = append(erpserver.sequenceInfoList, SequenceInfo{
						GenerationID: record.sequence.GenerationID,
						Priority:     record.sequence.Priority,
						SequencingID: record.sequence.SequencingID,
					})
					recordChan <- &record
				}
			case <-time.After(timeout):
				return
			}
		}
	}()

	// Block until a record is received from the channel or until the timeout expires.
	select {
	case record := <-recordChan:
		// Return the record.
		return record.record, nil
	case <-time.After(timeout):
		if expectEvents {
			return nil, errors.New("timed out waiting for new record")
		}
		return nil, nil
	}
}

// Close shutdown the server.
func (erpserver *ErpServer) Close() {
	if erpserver.server == nil {
		return
	}
	erpserver.server.Close()
}
