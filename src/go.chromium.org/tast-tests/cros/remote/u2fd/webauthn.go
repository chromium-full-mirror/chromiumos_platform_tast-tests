// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package u2fd contains functionality shared by WebAuthn/U2F tests.
package u2fd

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/remote/dutfs"
	hwsecpb "go.chromium.org/tast-tests/cros/services/cros/hwsec"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh/linuxssh"
)

// CopyFilesToRemote mirrors the given file paths from DataPath to a
// tempdir on the remote, and returns the tempdir path.
// `files` is the mapping of the data path of a file to its name in the tempdir.
func CopyFilesToRemote(ctx context.Context, dut *dut.DUT, client *dutfs.Client, files map[string]string) (string, error) {
	tempdir, err := client.TempDir(ctx, "", "webauthn")
	if err != nil {
		return "", errors.Wrap(err, "failed to create remote data path directory")
	}
	dataPath := strings.TrimSpace(string(tempdir))
	for key, value := range files {
		files[key] = filepath.Join(dataPath, value)
	}
	if _, err := linuxssh.PutFiles(ctx, dut.Conn(), files, linuxssh.DereferenceSymlinks); err != nil {
		return "", errors.Wrapf(err, "failed to send data to remote data path %v", dataPath)
	}

	return dataPath, nil
}

type remoteMakeCredentialResult struct {
	cred *hwsecpb.WebAuthnCredential
	err  error
}

// RemoteMakeCredentialInLocalSite performs MakeCredential in the local testing site
// on the remote using WebauthnServiceClient.
// MakeCredential is the process of requesting the authenticator (in our case,
// the ChromeOS device itself) to create a WebAuthn credential and return its
// handle and public key to the server.
func RemoteMakeCredentialInLocalSite(ctx context.Context, client hwsecpb.WebauthnServiceClient, authCallback func(context.Context) error) (*hwsecpb.WebAuthnCredential, error) {
	channel := make(chan remoteMakeCredentialResult)
	go func() {
		cred, err := client.StartMakeCredential(ctx, &empty.Empty{})
		channel <- remoteMakeCredentialResult{cred, err}
	}()

	if _, err := client.DoMakeCredential(ctx, &empty.Empty{}); err != nil {
		return nil, errors.Wrap(err, "failed to perform DoMakeCredential")
	}
	if err := authCallback(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to call the auth callback")
	}
	if _, err := client.EndMakeCredential(ctx, &empty.Empty{}); err != nil {
		return nil, errors.Wrap(err, "failed to perform EndMakeCredential")
	}

	select {
	case res := <-channel:
		return res.cred, res.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// RemoteGetAssertionInLocalSite performs GetAssertion in the local testing site.
// on the remote using WebauthnServiceClient.
// GetAssertion is the process of requesting the authenticator (in our case,
// the ChromeOS device itself) to sign a server challenge using the WebAuthn
// credential specified by the given handle.
func RemoteGetAssertionInLocalSite(ctx context.Context, client hwsecpb.WebauthnServiceClient, cred *hwsecpb.WebAuthnCredential, authCallback func(context.Context) error) error {
	channel := make(chan error)
	go func() {
		_, err := client.StartGetAssertion(ctx, &hwsecpb.StartGetAssertionRequest{Cred: cred})
		channel <- err
	}()

	if _, err := client.DoGetAssertion(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to perform DoGetAssertion")
	}
	if err := authCallback(ctx); err != nil {
		return errors.Wrap(err, "failed to call the auth callback")
	}
	if _, err := client.EndGetAssertion(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to perform EndGetAssertion")
	}

	select {
	case err := <-channel:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
