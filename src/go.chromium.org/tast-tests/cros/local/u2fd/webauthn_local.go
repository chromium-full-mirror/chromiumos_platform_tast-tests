// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package u2fd contains functionality shared by WebAuthn/U2F tests.
package u2fd

import (
	"context"
	"net/http"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// AuthCallback is a callback executed when the WebAuthn request is successfully
// dispatched, and it is expected to perform actions that satisfy the request.
type AuthCallback = func(context.Context, *uiauto.Context) error

// WebAuthnHTTPServer is a wrapped http.Server structure that provides some
// useful fields and helpers for our WebAuthn tests.
type WebAuthnHTTPServer struct {
	server *http.Server
	URL    string
}

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

// NewWebAuthnHTTPServer creates a WebAuthnHTTPServer using the given file
// system.
func NewWebAuthnHTTPServer(ctx context.Context, root http.FileSystem) *WebAuthnHTTPServer {
	mux := http.NewServeMux()
	fs := http.FileServer(root)
	mux.Handle("/", fs)
	server := &http.Server{Addr: ":8080", Handler: mux}
	go func() {
		server.ListenAndServe()
	}()
	return &WebAuthnHTTPServer{
		server: server,
		URL:    "http://localhost:8080",
	}
}

// Close closes the WebAuthn http server, and should be called before the test
// ends.
func (s *WebAuthnHTTPServer) Close(ctx context.Context) {
	if err := s.server.Shutdown(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to stop http server: ", err)
	}
}

// WebAuthnInLocalSite performs the WebAuthn procedure in the local testing
// site in data/.
func WebAuthnInLocalSite(ctx context.Context, conn *chrome.Conn, tconn *chrome.TestConn, authCallback AuthCallback) error {
	cred, err := MakeCredentialInLocalSite(ctx, conn, tconn, WebAuthnRegistrationConfig{Uv: "preferred"}, authCallback)
	if err != nil {
		return errors.Wrap(err, "failed to perform MakeCredential")
	}

	assertionConfig := WebAuthnAssertionConfig{
		Keys: []WebAuthnCredential{*cred},
		Uv:   "preferred",
	}
	if err = GetAssertionInLocalSite(ctx, conn, tconn, assertionConfig, authCallback); err != nil {
		return errors.Wrap(err, "failed to perform GetAssertion")
	}

	return nil
}

// InitiateMakeCredentialInLocalSite initiates the routine that dispatches a
// MakeCredential request in the local testing site.
func InitiateMakeCredentialInLocalSite(ctx context.Context, conn *chrome.Conn, config WebAuthnRegistrationConfig) (resultChannel chan MakeCredentialResult) {
	resultChannel = make(chan MakeCredentialResult)
	go func(ctx context.Context) {
		var cred WebAuthnCredential
		if err := conn.Call(ctx, &cred, "testApp.register", config); err != nil {
			resultChannel <- MakeCredentialResult{
				Cred: nil,
				Err:  errors.Wrap(err, "failed to complete WebAuthn registration"),
			}
		}
		resultChannel <- MakeCredentialResult{
			Cred: &cred,
			Err:  nil,
		}
	}(ctx)
	return
}

// MakeCredentialInLocalSite performs MakeCredential in the local testing site.
// MakeCredential is the process of requesting the authenticator (in our case,
// the ChromeOS device itself) to create a WebAuthn credential and return its
// handle and public key to the server.
func MakeCredentialInLocalSite(ctx context.Context, conn *chrome.Conn, tconn *chrome.TestConn, config WebAuthnRegistrationConfig, authCallback AuthCallback) (*WebAuthnCredential, error) {
	channel := InitiateMakeCredentialInLocalSite(ctx, conn, config)

	ui := uiauto.New(tconn)

	if err := ChoosePlatformAuthenticator(ctx, tconn); err != nil {
		return nil, err
	}
	if err := WaitForWebAuthnDialog(ctx, tconn); err != nil {
		return nil, err
	}
	if err := authCallback(ctx, ui); err != nil {
		return nil, errors.Wrap(err, "failed to call authCallback")
	}

	select {
	case res := <-channel:
		return res.Cred, res.Err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// InitiateGetAssertionInLocalSite initiates the routine that dispatches a
// GetAssertion request in the local testing site.
func InitiateGetAssertionInLocalSite(ctx context.Context, conn *chrome.Conn, config WebAuthnAssertionConfig) (errorChannel chan error) {
	errorChannel = make(chan error)
	var ret interface{}
	go func() {
		if err := conn.Call(ctx, &ret, "testApp.assert", config); err != nil {
			errorChannel <- errors.Wrap(err, "failed to complete WebAuthn assertion")
		}
		errorChannel <- nil
	}()
	return
}

// GetAssertionInLocalSite performs GetAssertion in the local testing site.
// GetAssertion is the process of requesting the authenticator (in our case,
// the ChromeOS device itself) to sign a server challenge using the WebAuthn
// credential specified by the given handle.
func GetAssertionInLocalSite(ctx context.Context, conn *chrome.Conn, tconn *chrome.TestConn, config WebAuthnAssertionConfig, authCallback AuthCallback) error {
	channel := InitiateGetAssertionInLocalSite(ctx, conn, config)

	ui := uiauto.New(tconn)

	if err := WaitForWebAuthnDialog(ctx, tconn); err != nil {
		return err
	}
	if err := authCallback(ctx, ui); err != nil {
		return errors.Wrap(err, "failed to call authCallback")
	}

	select {
	case err := <-channel:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
