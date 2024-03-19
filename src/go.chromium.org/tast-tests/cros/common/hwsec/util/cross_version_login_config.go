// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package util

import (
	"context"
	"crypto/rsa"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/common/u2fd"
	"go.chromium.org/tast/core/errors"
)

// VaultKeyInfo contains the information of a vault key
type VaultKeyInfo struct {
	Password   string
	KeyLabel   string
	LowEntropy bool
}

// NewVaultKeyInfo creates VaultKeyInfo from password, label and lowEntropy
func NewVaultKeyInfo(password, label string, lowEntropy bool) *VaultKeyInfo {
	return &VaultKeyInfo{
		Password:   password,
		KeyLabel:   label,
		LowEntropy: lowEntropy,
	}
}

// PinWeaverLabelsInfo contains the information of the PinWeaver hash tree
type PinWeaverLabelsInfo struct {
	NormalLabels    []int
	LockedOutLabels []int
}

// VaultFSType indicates the type of the file system used for the user vault.
type VaultFSType int

// CrossVersionLoginConfig contains the information for cross-version login
type CrossVersionLoginConfig struct {
	AuthConfig      hwsec.AuthConfig
	RsaKey          *rsa.PrivateKey
	KeyLabel        string
	ExtraVaultKeys  []VaultKeyInfo
	VaultFSType     VaultFSType
	WebAuthnCred    *u2fd.WebAuthnCredential
	InstallAttrs    map[string]string
	PinWeaverLabels *PinWeaverLabelsInfo
}

// NewPassAuthCrossVersionLoginConfig creates cross version-login config from password auth config
func NewPassAuthCrossVersionLoginConfig(authConfig *hwsec.AuthConfig, keyLabel string) *CrossVersionLoginConfig {
	return &CrossVersionLoginConfig{
		AuthConfig: *authConfig,
		KeyLabel:   keyLabel,
	}
}

// AddVaultKeyData adds a vault key to cryptohome and store the VaultKeyInfo to the config
func (config *CrossVersionLoginConfig) AddVaultKeyData(ctx context.Context, cryptohome *hwsec.CryptohomeClient, authID string, info *VaultKeyInfo) error {
	if info.LowEntropy {
		if err := cryptohome.AddPinAuthFactor(ctx, authID, info.KeyLabel, info.Password); err != nil {
			return errors.Wrap(err, "failed to add pin")
		}
		if _, err := cryptohome.AuthenticatePinAuthFactor(ctx, authID, info.KeyLabel, info.Password); err != nil {
			return errors.Wrap(err, "failed to verify added pin")
		}
	} else {
		if err := cryptohome.AddAuthFactor(ctx, authID, info.KeyLabel, info.Password); err != nil {
			return errors.Wrap(err, "failed to add password")
		}
		if _, err := cryptohome.AuthenticateAuthFactor(ctx, authID, info.KeyLabel, info.Password); err != nil {
			return errors.Wrap(err, "failed to verify added password")
		}
	}
	config.ExtraVaultKeys = append(config.ExtraVaultKeys, *info)
	return nil
}
