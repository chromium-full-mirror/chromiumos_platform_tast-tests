// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"strings"

	uda "go.chromium.org/chromiumos/system_api/user_data_auth_proto"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/shutil"
)

// cryptohomeBinary is used to interact with the cryptohomed process over
// 'cryptohome' executable. For more details of the arguments of the functions in this file,
// please check //src/platform2/cryptohome/cryptohome.cc.
// The arguments here are documented only when they are not directly
// mapped to the ones in so-mentioned cryptohome.cc.
type cryptohomeBinary struct {
	runner CmdRunner
}

// newCryptohomeBinary is a factory function to create a
// cryptohomeBinary instance.
func newCryptohomeBinary(r CmdRunner) *cryptohomeBinary {
	return &cryptohomeBinary{r}
}

func (c *cryptohomeBinary) call(ctx context.Context, args ...string) ([]byte, error) {
	return c.runner.Run(ctx, "cryptohome", args...)
}

func (c *cryptohomeBinary) callTestTool(ctx context.Context, args ...string) ([]byte, error) {
	return c.runner.Run(ctx, "cryptohome-test-tool", args...)
}

func (c *cryptohomeBinary) tempFile(ctx context.Context, prefix string) (string, error) {
	out, err := c.runner.Run(ctx, "mktemp", "/tmp/"+prefix+".XXXXX")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), err
}

func (c *cryptohomeBinary) readFile(ctx context.Context, filename string) ([]byte, error) {
	return c.runner.Run(ctx, "cat", "--", filename)
}

func (c *cryptohomeBinary) writeFile(ctx context.Context, filename string, data []byte) error {
	tmpFile, err := c.tempFile(ctx, "tast_cryptohome_write")
	if err != nil {
		return errors.Wrap(err, "failed to create temp file")
	}
	defer c.removeFile(ctx, tmpFile)
	b64String := base64.StdEncoding.EncodeToString(data)
	if _, err := c.runner.Run(ctx, "sh", "-c", "echo "+shutil.Escape(b64String)+">"+tmpFile); err != nil {
		return errors.Wrap(err, "failed to echo string")
	}
	_, err = c.runner.Run(ctx, "sh", "-c", "base64 -d "+tmpFile+">"+filename)
	return err
}

func (c *cryptohomeBinary) removeFile(ctx context.Context, filename string) error {
	_, err := c.runner.Run(ctx, "rm", "-f", "--", filename)
	return err
}

// installAttributesGetStatus calls "cryptohome --action=install_attributes_get_status".
func (c *cryptohomeBinary) installAttributesGetStatus(ctx context.Context) (string, error) {
	out, err := c.call(ctx, "--action=install_attributes_get_status")
	return string(out), err
}

// installAttributesGet calls "cryptohome --action=install_attributes_get".
func (c *cryptohomeBinary) installAttributesGet(ctx context.Context, attributeName string) (string, error) {
	out, err := c.call(ctx, "--action=install_attributes_get", "--name="+attributeName)
	return string(out), err
}

// installAttributesSet calls "cryptohome --action=install_attributes_set".
func (c *cryptohomeBinary) installAttributesSet(ctx context.Context, attributeName, attributeValue string) (string, error) {
	out, err := c.call(ctx, "--action=install_attributes_set", "--name="+attributeName, "--value="+attributeValue)
	return string(out), err
}

// installAttributesFinalize calls "cryptohome --action=install_attributes_finalize".
func (c *cryptohomeBinary) installAttributesFinalize(ctx context.Context) (string, error) {
	out, err := c.call(ctx, "--action=install_attributes_finalize")
	return string(out), err
}

// installAttributesCount calls "cryptohome --action=install_attributes_count".
func (c *cryptohomeBinary) installAttributesCount(ctx context.Context) (string, error) {
	out, err := c.call(ctx, "--action=install_attributes_count")
	return string(out), err
}

// installAttributesIsReady calls "cryptohome --action=install_attributes_is_ready".
func (c *cryptohomeBinary) installAttributesIsReady(ctx context.Context) (string, error) {
	out, err := c.call(ctx, "--action=install_attributes_is_ready")
	return string(out), err
}

// installAttributesIsSecure calls "cryptohome --action=install_attributes_is_secure".
func (c *cryptohomeBinary) installAttributesIsSecure(ctx context.Context) (string, error) {
	out, err := c.call(ctx, "--action=install_attributes_is_secure")
	return string(out), err
}

// installAttributesIsInvalid calls "cryptohome --action=install_attributes_is_invalid".
func (c *cryptohomeBinary) installAttributesIsInvalid(ctx context.Context) (string, error) {
	out, err := c.call(ctx, "--action=install_attributes_is_invalid")
	return string(out), err
}

// installAttributesIsFirstInstall calls "cryptohome --action=install_attributes_is_first_install".
func (c *cryptohomeBinary) installAttributesIsFirstInstall(ctx context.Context) (string, error) {
	out, err := c.call(ctx, "--action=install_attributes_is_first_install")
	return string(out), err
}

// isMounted calls "cryptohome --action=is_mounted".
func (c *cryptohomeBinary) isMounted(ctx context.Context) ([]byte, error) {
	return c.call(ctx, "--action=is_mounted")
}

// mountGuestEx calls "cryptohome --action=mount_guest_ex".
func (c *cryptohomeBinary) mountGuestEx(ctx context.Context) ([]byte, error) {
	args := []string{"--action=mount_guest_ex"}
	return c.call(ctx, args...)
}

// getSanitizedUsername calls "cryptohome --action=obfuscate_user".
func (c *cryptohomeBinary) getSanitizedUsername(ctx context.Context, username string, useDBus bool) ([]byte, error) {
	args := []string{"--action=obfuscate_user", "--user=" + username}
	if useDBus {
		args = append(args, "--use_dbus")
	}
	return c.call(ctx, args...)
}

// getSystemSalt calls "cryptohome --action=get_system_salt".
func (c *cryptohomeBinary) getSystemSalt(ctx context.Context, useDBus bool) ([]byte, error) {
	args := []string{"--action=get_system_salt"}
	if useDBus {
		args = append(args, "--use_dbus")
	}
	return c.call(ctx, args...)
}

// remove calls "cryptohome --action=remove".
func (c *cryptohomeBinary) remove(ctx context.Context, username string) ([]byte, error) {
	return c.call(ctx, "--action=remove", "--user="+username, "--force")
}

// unmount calls "cryptohome --action=unmount".
func (c *cryptohomeBinary) unmount(ctx context.Context, username string) ([]byte, error) {
	return c.call(ctx, "--action=unmount", "--user="+username)
}

// unmountAll calls "cryptohome --action=unmount", but without the username.
func (c *cryptohomeBinary) unmountAll(ctx context.Context) ([]byte, error) {
	return c.call(ctx, "--action=unmount")
}

// lockToSingleUserMountUntilReboot calls "cryptohome --action=lock_to_single_user_mount_until_reboot"
func (c *cryptohomeBinary) lockToSingleUserMountUntilReboot(ctx context.Context, username string) ([]byte, error) {
	return c.call(ctx, "--action=lock_to_single_user_mount_until_reboot", "--user="+username)
}

// dumpKeyset calls "cryptohome --action=dump_keyset".
func (c *cryptohomeBinary) dumpKeyset(ctx context.Context, username string) ([]byte, error) {
	return c.call(ctx, "--action=dump_keyset", "--user="+username)
}

// pkcs11SystemTokenInfo calls "cryptohome --action=pkcs11_get_system_token_info".
func (c *cryptohomeBinary) pkcs11SystemTokenInfo(ctx context.Context) ([]byte, error) {
	out, err := c.call(ctx, "--action=pkcs11_get_system_token_info")
	return out, err
}

// pkcs11UserTokenInfo calls "cryptohome --action=pkcs11_get_user_token_info". (and gets the user token status)
func (c *cryptohomeBinary) pkcs11UserTokenInfo(ctx context.Context, username string) ([]byte, error) {
	out, err := c.call(ctx, "--action=pkcs11_get_user_token_info", "--user="+username)
	return out, err
}

// pkcs11Terminate calls "cryptohome --action=pkcs11_terminate"
func (c *cryptohomeBinary) pkcs11Terminate(ctx context.Context, username string) ([]byte, error) {
	return c.call(ctx, "--action=pkcs11_terminate", "--user="+username)
}

// getFirmwareManagementParameters calls "cryptohome --action=get_firmware_management_parameters".
func (c *cryptohomeBinary) getFirmwareManagementParameters(ctx context.Context) ([]byte, error) {
	return c.call(ctx, "--action=get_firmware_management_parameters")
}

// setFirmwareManagementParameters calls "cryptohome --action=set_firmware_management_parameters".
func (c *cryptohomeBinary) setFirmwareManagementParameters(ctx context.Context, flags, hash string) ([]byte, error) {
	return c.call(ctx, "--action=set_firmware_management_parameters", "--flags="+flags, "--developer_key_hash="+hash)
}

// removeFirmwareManagementParameters calls "cryptohome --action=remove_firmware_management_parameters".
func (c *cryptohomeBinary) removeFirmwareManagementParameters(ctx context.Context) ([]byte, error) {
	return c.call(ctx, "--action=remove_firmware_management_parameters")
}

// getAccountDiskUsage calls "cryptohome --action=get_account_disk_usage".
func (c *cryptohomeBinary) getAccountDiskUsage(ctx context.Context, username string) ([]byte, error) {
	return c.call(ctx, "--action=get_account_disk_usage", "--user="+username)
}

// getSupportedKeyPolicies calls "cryptohome --action=get_supported_key_policies".
func (c *cryptohomeBinary) getSupportedKeyPolicies(ctx context.Context) ([]byte, error) {
	return c.call(ctx, "--action=get_supported_key_policies")
}

// startAuthSession calls "cryptohome --action=start_auth_session".
func (c *cryptohomeBinary) startAuthSession(ctx context.Context, username string, isEphemeral bool, authIntent uda.AuthIntent) ([]byte, error) {
	args := []string{"--action=start_auth_session", "--output-format=binary-protobuf", "--user=" + username, "--auth_intent=" + authIntent.String()}
	if isEphemeral {
		args = append(args, "--ensure_ephemeral")
	}
	return c.call(ctx, args...)
}

// authenticateAuthFactor calls "cryptohome --action=authenticate_auth_factor".
func (c *cryptohomeBinary) authenticateAuthFactor(ctx context.Context, authSessionID, label, password string) ([]byte, error) {
	args := []string{"--action=authenticate_auth_factor", "--output-format=binary-protobuf", "--auth_session_id=" + authSessionID, "--key_label=" + label, "--password=" + password}
	return c.call(ctx, args...)
}

// removeAuthFactor calls "cryptohome --action=remove_auth_factor".
func (c *cryptohomeBinary) removeAuthFactor(ctx context.Context, authSessionID, label string) ([]byte, error) {
	args := []string{"--action=remove_auth_factor", "--auth_session_id=" + authSessionID, "--key_label=" + label}
	return c.call(ctx, args...)
}

// authenticatePinAuthFactorWithStatusUpdate calls "cryptohome --action=authenticate_with_status_update --pin=<pin>" and gets the output in a delimited format so it could be
// parsed to get both AuthenticateAuthFactorReply and AuthFactorStatusUpdateReply.
func (c *cryptohomeBinary) authenticatePinAuthFactorWithStatusUpdate(ctx context.Context, authSessionID, label, pin string, broadcastID []byte) ([]byte, error) {
	args := []string{"--action=authenticate_with_status_update", "--output-format=delimited-binary-protobuf", "--auth_session_id=" + authSessionID, "--broadcast_id=" + hex.EncodeToString(broadcastID), "--key_label=" + label, "--pin=" + pin}
	return c.call(ctx, args...)
}

// startAuthSessionWithStatusUpdate calls "cryptohome --action=start_auth_session" and gets the output in a delimited format so it could be parsed to get both startAuthSessionReply
// and AuthFactorStatusUpdateReply.
func (c *cryptohomeBinary) startAuthSessionWithStatusUpdate(ctx context.Context, username string, isEphemeral bool, authIntent uda.AuthIntent) ([]byte, error) {
	args := []string{"--action=start_auth_session_with_status_update", "--output-format=delimited-binary-protobuf", "--user=" + username, "--auth_intent=" + authIntent.String()}
	if isEphemeral {
		args = append(args, "--ensure_ephemeral")
	}
	return c.call(ctx, args...)
}

func (c *cryptohomeBinary) fetchStatusUpdateSignal(ctx context.Context, broadcastID []byte) ([]byte, error) {
	args := []string{"--action=fetch_status_update", "--output-format=binary-protobuf", "--broadcast_id=" + hex.EncodeToString(broadcastID)}
	return c.call(ctx, args...)
}

// authenticatePinAuthFactor calls "cryptohome --action=authenticate_auth_factor --pin=<pin>".
func (c *cryptohomeBinary) authenticatePinAuthFactor(ctx context.Context, authSessionID, label, pin string) ([]byte, error) {
	args := []string{"--action=authenticate_auth_factor", "--output-format=binary-protobuf", "--auth_session_id=" + authSessionID, "--key_label=" + label, "--pin=" + pin}
	return c.call(ctx, args...)
}

// authenticateKioskAuthFactor calls "cryptohome --action=authenticate_auth_factor --public_mount".
func (c *cryptohomeBinary) authenticateKioskAuthFactor(ctx context.Context, authSessionID, label string) ([]byte, error) {
	args := []string{"--action=authenticate_auth_factor", "--auth_session_id=" + authSessionID, "--key_label=" + label, "--public_mount"}
	return c.call(ctx, args...)
}

// authenticateRecoveryAuthFactor calls "cryptohome --action=authenticate_auth_factor --recovery_epoch_response=<epochResponseHex> --recovery_response=<recoveryResponseHex>".
func (c *cryptohomeBinary) authenticateRecoveryAuthFactor(ctx context.Context, authSessionID, label, epochResponseHex, recoveryResponseHex, ledgerName, ledgerPubKeyHash, ledgerPubKey string) ([]byte, error) {
	args := []string{"--action=authenticate_auth_factor",
		"--auth_session_id=" + authSessionID,
		"--key_label=" + label,
		"--recovery_epoch_response=" + epochResponseHex,
		"--recovery_response=" + recoveryResponseHex,
		"--recovery_ledger_name=" + ledgerName,
		"--recovery_ledger_pub_key_hash=" + ledgerPubKeyHash,
		"--recovery_ledger_pub_key=" + ledgerPubKey}
	return c.call(ctx, args...)
}

// fetchRecoveryIDs calls "cryptohome --action=get_recovery_ids --user=<username> --key_label=<label>".
func (c *cryptohomeBinary) fetchRecoveryIDs(ctx context.Context, username, label string) ([]byte, error) {
	args := []string{"--action=get_recovery_ids",
		"--output-format=binary-protobuf",
		"--user=" + username,
		"--key_label=" + label}
	return c.call(ctx, args...)
}

// authenticateSmartCardAuthFactor calls "cryptohome --action=authenticate_auth_factor --challenge_response_algo=<algorithm>".
func (c *cryptohomeBinary) authenticateSmartCardAuthFactor(ctx context.Context, authSessionID, label string, extraFlags []string) ([]byte, error) {
	args := []string{"--action=authenticate_auth_factor", "--output-format=binary-protobuf", "--auth_session_id=" + authSessionID, "--key_label=" + label}
	args = append(args, extraFlags...)
	return c.call(ctx, args...)
}

// authenticateFingerprintAuthFactor calls "cryptohome --action=authenticate_auth_factor --fingerprint --key_labels=<labels>".
func (c *cryptohomeBinary) authenticateFingerprintAuthFactor(ctx context.Context, authSessionID string, labels []string) ([]byte, error) {
	args := []string{"--action=authenticate_auth_factor", "--output-format=binary-protobuf", "--auth_session_id=" + authSessionID, "--key_labels=" + strings.Join(labels, ","), "--fingerprint"}
	return c.call(ctx, args...)
}

// addAuthFactor calls "cryptohome --action=add_auth_factor".
func (c *cryptohomeBinary) addAuthFactor(ctx context.Context, authSessionID, label, password string, hashInfo *uda.KnowledgeFactorHashInfo) ([]byte, error) {
	args := []string{"--action=add_auth_factor", "--auth_session_id=" + authSessionID, "--key_label=" + label, "--password=" + password}
	if hashInfo != nil {
		args = append(args, "--hash_algorithm="+hashInfo.Algorithm.String(), "--hash_salt="+hex.EncodeToString(hashInfo.Salt))
		if hashInfo.ShouldGenerateKeyStore {
			args = append(args, "--generate_key_store")
		}
	}
	return c.call(ctx, args...)
}

// addPinAuthFactor calls "cryptohome --action=add_auth_factor --pin=<pin>".
func (c *cryptohomeBinary) addPinAuthFactor(ctx context.Context, authSessionID, label, pin string, hashInfo *uda.KnowledgeFactorHashInfo) ([]byte, error) {
	args := []string{"--action=add_auth_factor", "--auth_session_id=" + authSessionID, "--key_label=" + label, "--pin=" + pin}
	if hashInfo != nil {
		args = append(args, "--hash_algorithm="+hashInfo.Algorithm.String(), "--hash_salt="+hex.EncodeToString(hashInfo.Salt))
		if hashInfo.ShouldGenerateKeyStore {
			args = append(args, "--generate_key_store")
		}
	}
	return c.call(ctx, args...)
}

// addModernPinAuthFactor calls "cryptohome --action=add_auth_factor --pin=<pin> with --timed_lockout parameter to set up a modern pin".
func (c *cryptohomeBinary) addModernPinAuthFactor(ctx context.Context, authSessionID, label, pin string) ([]byte, error) {
	args := []string{"--action=add_auth_factor", "--auth_session_id=" + authSessionID, "--key_label=" + label, "--pin=" + pin, "--timed_lockout"}
	return c.call(ctx, args...)
}

// addFingerprintAuthFactor calls "cryptohome --action=add_auth_factor --fingerprint".
func (c *cryptohomeBinary) addFingerprintAuthFactor(ctx context.Context, authSessionID, label string) ([]byte, error) {
	args := []string{"--action=add_auth_factor", "--auth_session_id=" + authSessionID, "--key_label=" + label, "--fingerprint"}
	return c.call(ctx, args...)
}

// addRecoveryAuthFactor calls "cryptohome --action=add_auth_factor --recovery_mediator_pub_key=mediatorPubKeyHex".
func (c *cryptohomeBinary) addRecoveryAuthFactor(ctx context.Context, authSessionID, label, mediatorPubKeyHex, userGaiaID, deviceUserID string) ([]byte, error) {
	args := []string{"--action=add_auth_factor",
		"--auth_session_id=" + authSessionID,
		"--key_label=" + label,
		"--recovery_mediator_pub_key=" + mediatorPubKeyHex,
		"--recovery_user_gaia_id=" + userGaiaID,
		"--recovery_device_user_id=" + deviceUserID,
		"--ensure_fresh_recovery_id"}
	return c.call(ctx, args...)
}

// addKioskAuthFactor calls "cryptohome --action=add_auth_factor --public_mount".
func (c *cryptohomeBinary) addKioskAuthFactor(ctx context.Context, authSessionID string) ([]byte, error) {
	args := []string{"--action=add_auth_factor", "--auth_session_id=" + authSessionID, "--public_mount", "--key_label=public_mount"}
	return c.call(ctx, args...)
}

// addSmartCardAuthFactor calls "cryptohome --action=add_auth_factor --challnge_response_algorithm=<algo>
// --challenge_spki=<spki> --key_delegate_name=<key_delegate_name>".
func (c *cryptohomeBinary) addSmartCardAuthFactor(ctx context.Context, authSessionID, label string, extraFlags []string) ([]byte, error) {
	args := []string{"--action=add_auth_factor", "--auth_session_id=" + authSessionID, "--key_label=" + label}
	args = append(args, extraFlags...)
	return c.call(ctx, args...)
}

// updatePasswordAuthFactor calls "cryptohome --action=update_auth_factor".
func (c *cryptohomeBinary) updatePasswordAuthFactor(ctx context.Context, authSessionID, label, password string, hashInfo *uda.KnowledgeFactorHashInfo) ([]byte, error) {
	args := []string{"--action=update_auth_factor",
		"--auth_session_id=" + authSessionID,
		"--key_label=" + label,
		"--password=" + password}
	if hashInfo != nil {
		args = append(args, "--hash_algorithm="+hashInfo.Algorithm.String(), "--hash_salt="+hex.EncodeToString(hashInfo.Salt))
		if hashInfo.ShouldGenerateKeyStore {
			args = append(args, "--generate_key_store")
		}
	}
	return c.call(ctx, args...)
}

// updateRecoveryAuthFactor calls "cryptohome --action=update_auth_factor --recovery_mediator_pub_key=mediatorPubKeyHex".
func (c *cryptohomeBinary) updateRecoveryAuthFactor(
	ctx context.Context,
	authSessionID, label, mediatorPubKeyHex, userGaiaID, deviceUserID string,
	ensureFreshRecoveryID bool) ([]byte, error) {
	args := []string{"--action=update_auth_factor",
		"--auth_session_id=" + authSessionID,
		"--key_label=" + label,
		"--recovery_mediator_pub_key=" + mediatorPubKeyHex,
		"--recovery_user_gaia_id=" + userGaiaID,
		"--recovery_device_user_id=" + deviceUserID}
	if ensureFreshRecoveryID {
		args = append(args, "--ensure_fresh_recovery_id")
	}
	return c.call(ctx, args...)
}

// updatePinAuthFactor calls "cryptohome --action=update_auth_factor --pin=<pin>".
func (c *cryptohomeBinary) updatePinAuthFactor(ctx context.Context, authSessionID, label, pin string) ([]byte, error) {
	args := []string{"--action=update_auth_factor",
		"--auth_session_id=" + authSessionID,
		"--key_label=" + label,
		"--pin=" + pin}
	return c.call(ctx, args...)
}

// relabelAuthFactor calls "cryptohome --action=relabel_auth_factor".
func (c *cryptohomeBinary) relabelAuthFactor(ctx context.Context, authSessionID, label, newLabel string) ([]byte, error) {
	args := []string{"--action=relabel_auth_factor",
		"--auth_session_id=" + authSessionID,
		"--key_label=" + label,
		"--new_key_label=" + newLabel}
	return c.call(ctx, args...)
}

// replacePasswordAuthFactor calls "cryptohome --action=replace_auth_factor".
func (c *cryptohomeBinary) replacePasswordAuthFactor(ctx context.Context, authSessionID, label, newKeyLabel, password string, hashInfo *uda.KnowledgeFactorHashInfo) ([]byte, error) {
	args := []string{"--action=replace_auth_factor",
		"--auth_session_id=" + authSessionID,
		"--key_label=" + label,
		"--new_key_label=" + newKeyLabel,
		"--password=" + password}
	if hashInfo != nil {
		args = append(args, "--hash_algorithm="+hashInfo.Algorithm.String(), "--hash_salt="+hex.EncodeToString(hashInfo.Salt))
		if hashInfo.ShouldGenerateKeyStore {
			args = append(args, "--generate_key_store")
		}
	}
	return c.call(ctx, args...)
}

// prepareGuestVault calls "cryptohome --action=prepare_guest_vault"
func (c *cryptohomeBinary) prepareGuestVault(ctx context.Context) ([]byte, error) {
	return c.call(ctx, "--output-format=binary-protobuf", "--action=prepare_guest_vault")
}

// prepareEphemeralVault calls "cryptohome --action=prepare_ephemeral_vault" with "--auth_session_id".
func (c *cryptohomeBinary) prepareEphemeralVault(ctx context.Context, authSessionID string) ([]byte, error) {
	return c.call(ctx, "--action=prepare_ephemeral_vault", "--auth_session_id="+authSessionID)
}

// preparePersistentVault calls "cryptohome --action=prepare_persistent_vault" with "--auth_session_id"
// and optionally "--ecryptfs".
func (c *cryptohomeBinary) preparePersistentVault(ctx context.Context, authSessionID string, ecryptfs bool) ([]byte, error) {
	args := []string{"--output-format=binary-protobuf", "--action=prepare_persistent_vault", "--auth_session_id=" + authSessionID}
	if ecryptfs {
		args = append(args, "--ecryptfs")
	}
	return c.call(ctx, args...)
}

// prepareVaultForMigration calls "cryptohome --action=prepare_vault_for_migration" with "--auth_session_id"
func (c *cryptohomeBinary) prepareVaultForMigration(ctx context.Context, authSessionID string) ([]byte, error) {
	return c.call(ctx, "--action=prepare_vault_for_migration", "--auth_session_id="+authSessionID)
}

// createPersistentUser calls "cryptohome --action=create_persistent_user" with "--auth_session_id"
func (c *cryptohomeBinary) createPersistentUser(ctx context.Context, authSessionID string) ([]byte, error) {
	return c.call(ctx, "--action=create_persistent_user", "--auth_session_id="+authSessionID)
}

// migrateToDircrypto calls "cryptohome --action=migrate_to_dircrypto" with "--user"
func (c *cryptohomeBinary) migrateToDircrypto(ctx context.Context, userName string) ([]byte, error) {
	return c.call(ctx, "--action=migrate_to_dircrypto", "--user="+userName)
}

// invalidateAuthSession calls "cryptohome --action=invalidate_auth_session".
// password is ignored if publicMount is set to true.
func (c *cryptohomeBinary) invalidateAuthSession(ctx context.Context, authSessionID string) ([]byte, error) {
	args := []string{"--action=invalidate_auth_session", "--auth_session_id=" + authSessionID}
	return c.call(ctx, args...)
}

// fetchRecoveryRequest returns cryptohome recovery request to be sent to the mediator
// by calling "cryptohome --action=get_recovery_request --recovery_epoch_response=epochResponseHex".
func (c *cryptohomeBinary) fetchRecoveryRequest(ctx context.Context, authSessionID, label, epochResponseHex string) ([]byte, error) {
	args := []string{"--action=get_recovery_request", "--auth_session_id=" + authSessionID, "--key_label=" + label, "--recovery_epoch_response=" + epochResponseHex}
	return c.call(ctx, args...)
}

// listAuthFactors returns auth factors by calling "cryptohome --action=list_auth_factors".
func (c *cryptohomeBinary) listAuthFactors(ctx context.Context, username string) ([]byte, error) {
	args := []string{"--output-format=binary-protobuf", "--action=list_auth_factors", "--user=" + username}
	return c.call(ctx, args...)
}

// prepareAddFpAuthFactor returns the responses by calling "cryptohome --action=prepare_auth_factor --add --fingerprint".
func (c *cryptohomeBinary) prepareAddFpAuthFactor(ctx context.Context, authSessionID string) ([]byte, error) {
	args := []string{"--output-format=binary-protobuf", "--action=prepare_auth_factor", "--add", "--fingerprint", "--auth_session_id=" + authSessionID}
	return c.call(ctx, args...)
}

// prepareAuthFpAuthFactor returns the responses by calling "cryptohome --action=prepare_auth_factor --auth --fingerprint".
func (c *cryptohomeBinary) prepareAuthFpAuthFactor(ctx context.Context, authSessionID string) ([]byte, error) {
	args := []string{"--output-format=binary-protobuf", "--action=prepare_auth_factor", "--auth", "--fingerprint", "--auth_session_id=" + authSessionID}
	return c.call(ctx, args...)
}

// terminateFpAuthFactor returns the responses by calling "cryptohome --action=terminate_auth_factor --fingerprint".
func (c *cryptohomeBinary) terminateFpAuthFactor(ctx context.Context, authSessionID string) ([]byte, error) {
	args := []string{"--output-format=binary-protobuf", "--action=terminate_auth_factor", "--fingerprint", "--auth_session_id=" + authSessionID}
	return c.call(ctx, args...)
}

// prepareThenAddFpAuthFactor returns the responses by calling "cryptohome --action=prepare_and_add_auth_factor --fingerprint".
func (c *cryptohomeBinary) prepareThenAddFpAuthFactor(ctx context.Context, authSessionID, label string) ([]byte, error) {
	args := []string{"--action=prepare_and_add_auth_factor", "--fingerprint", "--auth_session_id=" + authSessionID, "--key_label=" + label}
	return c.call(ctx, args...)
}

// prepareThenAuthFpAuthFactor returns the responses by calling "cryptohome --action=prepare_and_auth_auth_factor --fingerprint".
func (c *cryptohomeBinary) prepareThenAuthFpAuthFactor(ctx context.Context, authSessionID string, labels []string) ([]byte, error) {
	args := []string{"--action=prepare_and_authenticate_auth_factor", "--fingerprint", "--auth_session_id=" + authSessionID, "--key_labels=" + strings.Join(labels, ",")}
	return c.call(ctx, args...)
}

// createVaultKeyset calls "--action=create_vault_keyset".
func (c *cryptohomeBinary) createVaultKeyset(ctx context.Context, authSessionID, passkey, keyDataLabel string, authFactorType uda.AuthFactorType, disableKeyData bool) error {
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
	_, err := c.callTestTool(ctx, args...)
	return err
}

// getRecoverableKeyStores returns the responses by calling "cryptohome --action=get_recoverable_key_stores".
func (c *cryptohomeBinary) getRecoverableKeyStores(ctx context.Context, username string) ([]byte, error) {
	args := []string{"--output-format=binary-protobuf", "--action=get_recoverable_key_stores", "--user=" + username}
	return c.call(ctx, args...)
}

// lockRecoveryFactorUntilReboot returns the responses by calling "cryptohome --action=lock_factor_until_reboot".
func (c *cryptohomeBinary) lockRecoveryFactorUntilReboot(ctx context.Context) ([]byte, error) {
	args := []string{"--output-format=binary-protobuf", "--action=lock_factor_until_reboot", "--recovery_mediator_pub_key"}
	return c.call(ctx, args...)
}

// isPinWeaverPkEstablishmentBlocked returns the responses by calling "cryptohome --action=is_pw_pk_establishment_blocked".
func (c *cryptohomeBinary) isPinWeaverPkEstablishmentBlocked(ctx context.Context) ([]byte, error) {
	args := []string{"--action=is_pw_pk_establishment_blocked"}
	return c.call(ctx, args...)
}
