// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package u2fd contains functionality shared by WebAuthn/U2F tests.
package u2fd

// PublicKey is the PublicKey structure included in WebAuthnCredential.
type PublicKey struct {
	DataB64 string `json:"dataB64"`
	KeyType string `json:"keyType,omitempty"`
}

// WebAuthnCredential is the WebAuthn credential structure used in the local
// testing site.
type WebAuthnCredential struct {
	CredentialIDB64 string `json:"credentialIDB64"`
	PublicKey       `json:"publicKey"`
	SerialNumberB64 string `json:"serialNumberB64"`
}

// WebAuthnRegistrationConfig is the config to specify options of WebAuthn
// registration (MakeCredential).
type WebAuthnRegistrationConfig struct {
	Attestation             string `json:"attestation,omitempty"`
	AuthenticatorAttachment string `json:"authenticatorAttachment,omitempty"`
	Uv                      string `json:"uv,omitempty"`
}

// WebAuthnAssertionConfig is the config to specify options of WebAuthn
// assertion (GetAssertion).
type WebAuthnAssertionConfig struct {
	Keys []WebAuthnCredential `json:"keys,omitempty"`
	Uv   string               `json:"uv,omitempty"`
}

// MakeCredentialResult is the structure communicated by the MakeCredential
// routine using a channel.
type MakeCredentialResult struct {
	Cred *WebAuthnCredential
	Err  error
}
