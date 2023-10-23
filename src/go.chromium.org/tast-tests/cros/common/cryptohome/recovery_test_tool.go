// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package cryptohome operates on encrypted home directories.
package cryptohome

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	uda "chromiumos/system_api/user_data_auth_proto"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	destinationShareFile                 = "dst"
	extendedPcrBoundDestinationShareFile = "dst_extended_pcr"
	ledgerInfoFile                       = "ledger_info"
	rsaPrivKeyFile                       = "rsa_priv_key"
	channelPubKeyFile                    = "channel_pub"
	channelPrivKeyFile                   = "channel_priv"
	hsmPayloadFile                       = "hsm_payload"
	recoverySecretCreatedFile            = "secr_crea"
	ephemeralPubKeyFile                  = "ephemeral_pub"
	recoveryRequestFile                  = "recovery_req"
	recoveryResponseFile                 = "response"
	recoverySecretDecryptedFile          = "secr_decr"
	customRAPTFile                       = "custom_rapt"
	customEpochResponseFile              = "custom_epoch_response"
	epochResponseFile                    = "epoch_response"
	mediatorPubKeyFile                   = "mediator_pub_key"
	customMediatorPubKeyFile             = "custom_mediator_pub_key"
	recoveryIDFile                       = "recovery_id"
)

// LedgerInfo stores public properties of a recovery ledger. This information
// is passed to Cryptohome to let it determine which ledger is used (in
// production, it's Chrome's responsibility to do so).
type LedgerInfo struct {
	Name    string `json:"name"`
	KeyHash string `json:"key_hash"`
	// Ledger's public key (base64 encoded).
	PublicKey string `json:"public_key"`
}

// RecoveryTestTool is a command line test tool for cryptohome recovery testing.
type RecoveryTestTool struct {
	tmpFolderName string
	// custom mediator key, if not set - the fake mediator (with fake mediator key) will be used.
	mediatorPubKeyHex string
	// custom ledger info, is set only when `mediatorPubKeyHex` is set.
	ledgerInfo LedgerInfo
	// Runner to interact with the cryptohome-test-tool binary
	// For more details of the arguments of the functions in this file,
	// please check //src/platform2/cryptohome/cryptohome-test-tool.cc.
	runner hwsec.CmdRunner
}

// NewRecoveryTestToolWithFakeMediator creates a new instance of RecoveryTestTool with generated directory.
// The instance will use fake (local) mediation.
// Call RemoveDir in the end of the test.
func NewRecoveryTestToolWithFakeMediator(r hwsec.CmdRunner) (*RecoveryTestTool, error) {
	// Create a temp directory.
	name, err := ioutil.TempDir("", "cryptohome_test_tool_out_*")
	if err != nil {
		return nil, errors.Wrap(err, "could not create a temp directory")
	}
	return &RecoveryTestTool{
		tmpFolderName:     name,
		mediatorPubKeyHex: "",
		runner:            r,
	}, nil
}

// NewRecoveryTestTool creates a new instance of RecoveryTestTool with generated directory.
// The instance will not use fake mediation. Use Save* methods to set the real server replies.
// Call RemoveDir in the end of the test.
func NewRecoveryTestTool(r hwsec.CmdRunner, mediatorPubKeyHex string, ledgerInfo LedgerInfo) (*RecoveryTestTool, error) {
	// Create a temp directory.
	name, err := ioutil.TempDir("", "cryptohome_test_tool_out_*")
	if err != nil {
		return nil, errors.Wrap(err, "could not create a temp directory")
	}
	return &RecoveryTestTool{
		tmpFolderName:     name,
		mediatorPubKeyHex: mediatorPubKeyHex,
		ledgerInfo:        ledgerInfo,
		runner:            r,
	}, nil
}

// useFakeMediator returns true if fake (local) mediator should be used.
func (c *RecoveryTestTool) useFakeMediator() bool {
	return c.mediatorPubKeyHex == ""
}

// call calls the test tool with provided parameters.
func (c *RecoveryTestTool) call(ctx context.Context, args ...string) ([]byte, error) {
	return c.runner.Run(ctx, "cryptohome-test-tool", args...)
}

// getFullFilePath returns the full file name inside the generated directory.
func (c *RecoveryTestTool) getFullFilePath(fileName string) string {
	return filepath.Join(c.tmpFolderName, fileName)
}

// getFileParam returns a parameter to cryptohome-test-tool in the format of "--param=getFullFilePath(file)"
func (c *RecoveryTestTool) getFileParam(param, file string) string {
	return fmt.Sprintf("--%s=%s", param, c.getFullFilePath(file))
}

// writeFile saves a file with provided fileName.
func (c *RecoveryTestTool) writeFile(fileName string, content []byte) error {
	filePath := c.getFullFilePath(fileName)
	if err := ioutil.WriteFile(filePath, content, 0644); err != nil {
		return errors.Wrapf(err, "failed to write file %q", filePath)
	}
	return nil
}

// writeFileHexEncoded saves a file with provided fileName and with content encoded in hex.
func (c *RecoveryTestTool) writeFileHexEncoded(fileName string, content []byte) error {
	return c.writeFile(fileName, []byte(hex.EncodeToString(content)))
}

// RemoveDir removes the folder generated by NewRecoveryTestTool.
func (c *RecoveryTestTool) RemoveDir() error {
	if c.tmpFolderName == "" {
		return errors.New("There is no folder set")
	}
	return os.RemoveAll(c.tmpFolderName)
}

// SaveCustomRAPT saves the provided reauth proof token to be used in CreateRecoveryRequest.
func (c *RecoveryTestTool) SaveCustomRAPT(rapt []byte) error {
	if c.useFakeMediator() {
		return errors.New("cannot use custom RAPT file with fake mediator")
	}
	return c.writeFileHexEncoded(customRAPTFile, rapt)
}

// SaveCustomEpoch saves the provided epoch to be used in CreateRecoveryRequest and Decrypt.
func (c *RecoveryTestTool) SaveCustomEpoch(epoch []byte) error {
	if c.useFakeMediator() {
		return errors.New("cannot use custom epoch response file with fake mediator")
	}
	return c.writeFileHexEncoded(customEpochResponseFile, epoch)
}

// SaveCustomResponse saves the provided epoch to be used in Decrypt.
func (c *RecoveryTestTool) SaveCustomResponse(response []byte) error {
	if c.useFakeMediator() {
		return errors.New("cannot use custom recovery response file with fake mediator")
	}
	return c.writeFileHexEncoded(recoveryResponseFile, response)
}

// FetchRecoveryRequest returns the request generated by CreateRecoveryRequest.
func (c *RecoveryTestTool) FetchRecoveryRequest() ([]byte, error) {
	requestHex, err := ioutil.ReadFile(c.getFullFilePath(recoveryRequestFile))
	if err != nil {
		return nil, errors.Wrapf(err, "could not read the recovery request file (%s)", recoveryRequestFile)
	}
	request, err := hex.DecodeString(string(requestHex))
	if err != nil {
		return nil, errors.Wrapf(err, "could not decode request hex (%s) to bytes", string(requestHex))
	}
	return request, nil
}

// CreateHsmPayload calls "--action=recovery_crypto_create_hsm_payload" step.
func (c *RecoveryTestTool) CreateHsmPayload(ctx context.Context) error {
	args := []string{
		"--action=recovery_crypto_create_hsm_payload",
		c.getFileParam("destination_share_out_file", destinationShareFile),
		c.getFileParam("extended_pcr_bound_destination_share_out_file", extendedPcrBoundDestinationShareFile),
		c.getFileParam("rsa_priv_key_out_file", rsaPrivKeyFile),
		c.getFileParam("channel_pub_key_out_file", channelPubKeyFile),
		c.getFileParam("channel_priv_key_out_file", channelPrivKeyFile),
		c.getFileParam("serialized_hsm_payload_out_file", hsmPayloadFile),
		c.getFileParam("recovery_secret_out_file", recoverySecretCreatedFile),
		c.getFileParam("recovery_id_file", recoveryIDFile),
	}

	if !c.useFakeMediator() {
		if err := c.writeFile(customMediatorPubKeyFile, []byte(c.mediatorPubKeyHex)); err != nil {
			return errors.Wrapf(err, "could not write the mediator public key file (%s)", customMediatorPubKeyFile)
		}

		args = append(args, c.getFileParam("mediator_pub_key_in_file", customMediatorPubKeyFile))
	}

	_, err := c.call(ctx, args...)
	return err
}

// CreateRecoveryRequest calls "--action=recovery_crypto_create_recovery_request" step.
func (c *RecoveryTestTool) CreateRecoveryRequest(ctx context.Context) error {
	args := []string{
		"--action=recovery_crypto_create_recovery_request",
		c.getFileParam("rsa_priv_key_in_file", rsaPrivKeyFile),
		c.getFileParam("channel_pub_key_in_file", channelPubKeyFile),
		c.getFileParam("channel_priv_key_in_file", channelPrivKeyFile),
		c.getFileParam("serialized_hsm_payload_in_file", hsmPayloadFile),
		c.getFileParam("ephemeral_pub_key_out_file", ephemeralPubKeyFile),
		c.getFileParam("recovery_request_out_file", recoveryRequestFile),
	}
	if !c.useFakeMediator() {
		args = append(args,
			c.getFileParam("gaia_rapt_in_file", customRAPTFile),
			c.getFileParam("epoch_response_in_file", customEpochResponseFile),
		)
	}
	_, err := c.call(ctx, args...)
	return err
}

// CreateVaultKeyset calls "--action=create_vault_keyset".
func (c *RecoveryTestTool) CreateVaultKeyset(ctx context.Context, authSessionID, passkey, keyDataLabel string, authFactorType uda.AuthFactorType, disableKeyData bool) error {
	args := []string{
		"--action=create_vault_keyset",
		"--auth_session_id=" + authSessionID,
		"--passkey=" + passkey,
		"--key_data_label=" + keyDataLabel,
	}
	if disableKeyData {
		args = append(args, "--disable_key_data=true")
	}
	switch authFactorType {
	case uda.AuthFactorType_AUTH_FACTOR_TYPE_PASSWORD:
		args = append(args, "--auth_factor_type=password")
	case uda.AuthFactorType_AUTH_FACTOR_TYPE_PIN:
		args = append(args, "--auth_factor_type=pin")
	case uda.AuthFactorType_AUTH_FACTOR_TYPE_KIOSK:
		args = append(args, "--use_public_mount_salt=true",
			"--auth_factor_type=kiosk")
	}
	_, err := c.call(ctx, args...)
	return err
}

// CreateSmartCardVaultKeyset calls "--action=create_vault_keyset" with extra flags for challenge credentials.
func (c *RecoveryTestTool) CreateSmartCardVaultKeyset(ctx context.Context, authSessionID, keyDataLabel string, authConfig *hwsec.AuthConfig) error {
	args := []string{
		"--action=create_vault_keyset",
		"--auth_session_id=" + authSessionID,
		"--key_data_label=" + keyDataLabel,
		"--auth_factor_type=smart_card",
		"--challenge_spki=" + hex.EncodeToString(authConfig.ChallengeSPKI),
		"--key_delegate_name=" + authConfig.KeyDelegateName,
	}

	_, err := c.call(ctx, args...)
	return err
}

// FakeMediate calls "--action=recovery_crypto_mediate" step.
func (c *RecoveryTestTool) FakeMediate(ctx context.Context) error {
	if !c.useFakeMediator() {
		return errors.New("cannot use fake mediator")
	}
	_, err := c.call(ctx,
		"--action=recovery_crypto_mediate",
		c.getFileParam("recovery_request_in_file", recoveryRequestFile),
		c.getFileParam("recovery_response_out_file", recoveryResponseFile),
	)
	return err
}

// Decrypt calls "--action=recovery_crypto_decrypt" step.
func (c *RecoveryTestTool) Decrypt(ctx context.Context) error {
	args := []string{
		"--action=recovery_crypto_decrypt",
		c.getFileParam("recovery_response_in_file", recoveryResponseFile),
		c.getFileParam("channel_priv_key_in_file", channelPrivKeyFile),
		c.getFileParam("ephemeral_pub_key_in_file", ephemeralPubKeyFile),
		c.getFileParam("destination_share_in_file", destinationShareFile),
		c.getFileParam("serialized_hsm_payload_in_file", hsmPayloadFile),
		c.getFileParam("extended_pcr_bound_destination_share_in_file", extendedPcrBoundDestinationShareFile),
		c.getFileParam("recovery_secret_out_file", recoverySecretDecryptedFile),
	}
	if !c.useFakeMediator() {
		ledgerInfoJSON, err := json.Marshal(c.ledgerInfo)
		if err != nil {
			return errors.Wrap(err, "failed to marshal ledgerInfo to json")
		}
		if err := c.writeFile(ledgerInfoFile, []byte(ledgerInfoJSON)); err != nil {
			return errors.Wrapf(err, "could not write the ledger info file (%s)", ledgerInfoFile)
		}

		args = append(args,
			c.getFileParam("epoch_response_in_file", customEpochResponseFile),
			c.getFileParam("ledger_info_in_file", ledgerInfoFile),
		)
	}
	_, err := c.call(ctx, args...)
	return err
}

// Validate compares secret created by CreateHsmPayload with secret derived by Decrypt. They are expected to be the same.
func (c *RecoveryTestTool) Validate(ctx context.Context) error {
	recoverySecretCreatedData, err := ioutil.ReadFile(c.getFullFilePath(recoverySecretCreatedFile))
	if err != nil {
		return errors.Wrapf(err, "could not read the created recovery secret file (%s)", recoverySecretCreatedFile)
	}
	recoverySecretDecrypedData, err := ioutil.ReadFile(c.getFullFilePath(recoverySecretDecryptedFile))
	if err != nil {
		return errors.Wrapf(err, "could not read the decrypted recovery secret file (%s)", recoverySecretDecryptedFile)
	}
	// The content of `recoverySecretCreatedFile` and `recoverySecretDecryptedFile` is hexadecimal-encoded.
	if bytes.Compare(recoverySecretCreatedData, recoverySecretDecrypedData) != 0 {
		return errors.Errorf("Created %v, decrypted %v", string(recoverySecretCreatedData), string(recoverySecretDecrypedData))
	}
	return nil
}

// FakeMediateWithRequest calls "--action=recovery_crypto_mediate" step.
// Returns hex-encoded response on success.
func (c *RecoveryTestTool) FakeMediateWithRequest(ctx context.Context, requestHex string) (string, error) {
	if !c.useFakeMediator() {
		return "", errors.New("cannot use fake mediator")
	}

	if err := c.writeFile(recoveryRequestFile, []byte(requestHex)); err != nil {
		return "", errors.Wrapf(err, "could not write the recovery request file (%s)", recoveryRequestFile)
	}

	if _, err := c.call(ctx,
		"--action=recovery_crypto_mediate",
		c.getFileParam("recovery_request_in_file", recoveryRequestFile),
		c.getFileParam("recovery_response_out_file", recoveryResponseFile),
	); err != nil {
		return "", errors.Wrap(err, "could not perform recovery_crypto_mediate")
	}

	responseHex, err := ioutil.ReadFile(c.getFullFilePath(recoveryResponseFile))
	if err != nil {
		return "", errors.Wrapf(err, "could not read the recovery response file (%s)", recoveryResponseFile)
	}
	return string(responseHex), nil
}

// FetchFakeEpochResponseHex calls "--action=recovery_crypto_get_fake_epoch".
// Returns hex-encoded epoch response on success.
func (c *RecoveryTestTool) FetchFakeEpochResponseHex(ctx context.Context) (string, error) {
	if !c.useFakeMediator() {
		return "", errors.New("cannot use fake mediator")
	}

	if _, err := c.call(ctx,
		"--action=recovery_crypto_get_fake_epoch",
		c.getFileParam("epoch_response_out_file", epochResponseFile),
	); err != nil {
		return "", errors.Wrap(err, "could not perform recovery_crypto_get_fake_epoch")
	}

	responseHex, err := ioutil.ReadFile(c.getFullFilePath(epochResponseFile))
	if err != nil {
		return "", errors.Wrapf(err, "could not read the epoch response file (%s)", epochResponseFile)
	}
	return string(responseHex), nil
}

// FetchFakeMediatorPubKeyHex calls "--action=recovery_crypto_get_fake_mediator_pub_key".
// Returns hex-encoded mediator key on success.
func (c *RecoveryTestTool) FetchFakeMediatorPubKeyHex(ctx context.Context) (string, error) {
	if !c.useFakeMediator() {
		return "", errors.New("cannot use fake mediator")
	}

	if _, err := c.call(ctx,
		"--action=recovery_crypto_get_fake_mediator_pub_key",
		c.getFileParam("mediator_pub_key_out_file", mediatorPubKeyFile),
	); err != nil {
		return "", errors.Wrap(err, "could not perform recovery_crypto_get_fake_mediator_pub_key")
	}

	mediatorPubKeyHex, err := ioutil.ReadFile(c.getFullFilePath(mediatorPubKeyFile))
	if err != nil {
		return "", errors.Wrapf(err, "could not read the mediator pub key file (%s)", mediatorPubKeyHex)
	}
	return string(mediatorPubKeyHex), nil
}

// FetchFakeLedgerInfo calls "--action=recovery_crypto_get_fake_ledger_info".
// Returns ledger info on success.
func (c *RecoveryTestTool) FetchFakeLedgerInfo(ctx context.Context) (*LedgerInfo, error) {
	if !c.useFakeMediator() {
		return nil, errors.New("cannot use fake mediator")
	}

	if _, err := c.call(ctx,
		"--action=recovery_crypto_get_fake_ledger_info",
		c.getFileParam("ledger_info_out_file", ledgerInfoFile),
	); err != nil {
		return nil, errors.Wrap(err, "could not perform recovery_crypto_get_fake_ledger_info")
	}

	ledgerInfoJSON, err := ioutil.ReadFile(c.getFullFilePath(ledgerInfoFile))
	if err != nil {
		return nil, errors.Wrapf(err, "could not read the ledger info file (%s)", ledgerInfoFile)
	}

	var result LedgerInfo
	if err := json.Unmarshal(ledgerInfoJSON, &result); err != nil {
		testing.ContextLogf(ctx, "Failed unmarshaling json: %s", ledgerInfoJSON)
		return nil, errors.Wrap(err, "failed unmarshaling json")
	}
	return &result, nil
}
