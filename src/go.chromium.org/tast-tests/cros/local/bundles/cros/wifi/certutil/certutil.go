// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package certutil contains utilities for manipulating certificates.
package certutil

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"io/ioutil"
	"os"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/common/crypto/certificate"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// PrepareCertSHA256Fingerprint gets the client certificate's SHA256 fingerprint.
func PrepareCertSHA256Fingerprint(certs certificate.CertStore) (string, error) {
	block, _ := pem.Decode([]byte(certs.ClientCred.Cert))
	if block == nil {
		return "", errors.New("failed to parse PEM file")
	}
	hash := sha256.New()
	hash.Write(block.Bytes)
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// PreparePKCS12Cert create a PKCS#12 format certificate from its client's certificate and private key.
func PreparePKCS12Cert(ctx context.Context, certs certificate.CertStore) (cert string, retErr error) {
	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		return "", errors.Wrap(err, "failed to create a temporary directory")
	}
	defer func() {
		if retErr != nil {
			if err := os.RemoveAll(tmpDir); err != nil {
				testing.ContextLogf(ctx, "Failed to clean up dir %s, %v", tmpDir, err)
			}
		}
	}()

	clientCertPath := filepath.Join(tmpDir, "cert")
	privateKeyPath := filepath.Join(tmpDir, "private_key")

	for _, p := range []struct {
		path     string
		contents string
	}{
		{clientCertPath, certs.ClientCred.Cert},
		{privateKeyPath, certs.ClientCred.PrivateKey},
	} {
		if err := ioutil.WriteFile(p.path, []byte(p.contents), 0644); err != nil {
			return "", errors.Wrapf(err, "failed to write file %q", p.path)
		}
	}
	out, err := testexec.CommandContext(ctx, "openssl", "pkcs12", "-export", "-inkey", privateKeyPath, "-in", clientCertPath, "-password", "pass:", "-legacy").Output(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "failed to create PKCS#12 certificate")
	}
	return string(out), nil
}
