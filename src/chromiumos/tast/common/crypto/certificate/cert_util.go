// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package certificate

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"

	"chromiumos/tast/common/testexec"
)

const tempDir = "/tmp"

// WriteCACertToFile uses a certificate |certificate|, creates the CA
// certificate file accordingly and writes the file with a specified file name
// |caCertFileName| onto the specified DUT |dutConn|, returns the path of
// created file and a closure to cleanup the created file.
func WriteCACertToFile(ctx context.Context, certificate CertStore, caCertFileName string,
	dutConn *ssh.Conn) (caCertPath string, cleanup func(context.Context) error, _ error) {

	caCertPath = filepath.Join(tempDir, fmt.Sprintf("%s.crt", caCertFileName))
	if err := linuxssh.WriteFile(ctx, dutConn, caCertPath, []byte(certificate.CACred.Cert), 0644); err != nil {
		return "", nil, errors.Wrap(err, "failed to create CA certificate file")
	}

	return caCertPath, func(ctx context.Context) error {
		return dutConn.CommandContext(ctx, "rm", caCertPath).Run(testexec.DumpLogOnError)
	}, nil
}

// WriteClientCertToFile uses a certificate |certificate|, creates the client
// certificate file accordingly with a specified password |password|, writes
// the file with a specified file name |clientCertFileName| onto the specified
// DUT |dutConn|, returns the path of created file and a closure to cleanup
// the created file.
func WriteClientCertToFile(ctx context.Context, certificate CertStore, password, clientCertFileName string,
	dutConn *ssh.Conn) (clientCertPath string, cleanup func(context.Context) error, retErr error) {

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	pemPath := filepath.Join(tempDir, fmt.Sprintf("%s.pem", clientCertFileName))
	if err := linuxssh.WriteFile(ctx, dutConn, pemPath, []byte(certificate.ClientCred.Cert), 0644); err != nil {
		return "", nil, errors.Wrap(err, "failed to create pem file")
	}
	defer dutConn.CommandContext(cleanupCtx, "rm", pemPath).Run(testexec.DumpLogOnError)

	keyPath := filepath.Join(tempDir, fmt.Sprintf("%s.key", clientCertFileName))
	if err := linuxssh.WriteFile(ctx, dutConn, keyPath, []byte(certificate.ClientCred.PrivateKey), 0644); err != nil {
		return "", nil, errors.Wrap(err, "failed to create key file")
	}
	defer dutConn.CommandContext(cleanupCtx, "rm", keyPath).Run(testexec.DumpLogOnError)

	clientCertPath = filepath.Join(tempDir, fmt.Sprintf("%s.p12", clientCertFileName))
	cleanup = func(ctx context.Context) error {
		return dutConn.CommandContext(ctx, "rm", clientCertPath).Run(testexec.DumpLogOnError)
	}
	if err := dutConn.CommandContext(ctx, "openssl", "pkcs12", "-export", "-out", clientCertPath, "-inkey", keyPath, "-in", pemPath, "-passout", "pass:"+password).Run(testexec.DumpLogOnError); err != nil {
		return "", nil, errors.Wrap(err, "failed to create client certificate file")
	}
	defer func(ctx context.Context) {
		if retErr != nil {
			cleanup(ctx)
		}
	}(cleanupCtx)

	if err := dutConn.CommandContext(ctx, "chmod", "0644", clientCertPath).Run(testexec.DumpLogOnError); err != nil {
		return "", cleanup, errors.Wrap(err, "failed to change permission of the client certificate file")
	}

	return clientCertPath, cleanup, nil
}
