// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package certificate

import (
	"context"
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
)

// Destination holds the information and resources required to write a certificate onto a DUT.
type Destination struct {
	certUtil certUtil
	fileName string

	// fullPath is the complete path to the file, including the file name.
	fullPath string
}

// NewRemoteDestination creates a new instance of Destination used in a remote test.
func NewRemoteDestination(dutConn *ssh.Conn, path, fileName string) *Destination {
	return &Destination{
		certUtil: &remoteCertUtil{Conn: dutConn},
		fileName: fileName,
		fullPath: filepath.Join(path, fileName),
	}
}

// NewLocalDestination creates a new instance of Destination used in a local test.
func NewLocalDestination(path, fileName string) *Destination {
	return &Destination{
		certUtil: &localCertUtil{},
		fileName: fileName,
		fullPath: filepath.Join(path, fileName),
	}
}

// FullPath gets the full file path of a certificate.
func (d Destination) FullPath() string { return d.fullPath }

const defaultFileMode fs.FileMode = 0644

// WriteCACert writes the CA certificate in |certStore| to the specified destination |dest|,
// returns an action that can be used to clean up the created file, and the result of the attempt to create the file.
func WriteCACert(ctx context.Context, dest *Destination, certStore CertStore) (action.Action, error) {
	data := []byte(certStore.CACred.Cert)

	cleanup := func(ctx context.Context) error {
		return dest.certUtil.remove(ctx, dest.fullPath)
	}
	return cleanup, dest.certUtil.write(ctx, data, dest.fullPath)
}

// WriteClientCertWithPassword writes the client certificate in |certStore| to the specified
// destination |dest| with the specified password |password|, returns an action that
// can be used to clean up the created file, and the result of the attempt to create the file.
func WriteClientCertWithPassword(ctx context.Context, dest *Destination, certStore CertStore, password string) (_ action.Action, retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	tempDir := dest.certUtil.tempDir()

	pemPath := filepath.Join(tempDir, fmt.Sprintf("%s.pem", dest.fileName))
	pemData := []byte(certStore.ClientCred.Cert)
	if err := dest.certUtil.write(ctx, pemData, pemPath); err != nil {
		return nil, errors.Wrap(err, "failed to create pem file")
	}
	defer dest.certUtil.remove(cleanupCtx, pemPath)

	keyPath := filepath.Join(tempDir, fmt.Sprintf("%s.key", dest.fileName))
	keyData := []byte(certStore.ClientCred.PrivateKey)
	if err := dest.certUtil.write(ctx, keyData, keyPath); err != nil {
		return nil, errors.Wrap(err, "failed to create key file")
	}
	defer dest.certUtil.remove(cleanupCtx, keyPath)

	cleanup := func(ctx context.Context) error {
		return dest.certUtil.remove(ctx, dest.fullPath)
	}
	if err := dest.certUtil.sendCommand(ctx, "openssl", "pkcs12", "-export", "-out", dest.fullPath, "-inkey", keyPath, "-in", pemPath, "-passout", "pass:"+password); err != nil {
		return nil, errors.Wrap(err, "failed to create client certificate file")
	}
	defer func(ctx context.Context) {
		if retErr != nil {
			cleanup(ctx)
		}
	}(cleanupCtx)

	return cleanup, dest.certUtil.sendCommand(ctx, "chmod", fmt.Sprintf("%04o", defaultFileMode), dest.fullPath)
}

// certUtil is an interface of utilities regarding certificates.
type certUtil interface {
	write(context.Context, []byte, string) error
	remove(context.Context, string) error
	sendCommand(context.Context, string, ...string) error
	tempDir() string
}

type remoteCertUtil struct{ *ssh.Conn }

func (w *remoteCertUtil) write(ctx context.Context, data []byte, fullPath string) error {
	return linuxssh.WriteFile(ctx, w.Conn, fullPath, data, defaultFileMode)
}

func (w *remoteCertUtil) remove(ctx context.Context, fullPath string) error {
	return w.sendCommand(ctx, "rm", fullPath)
}

func (w *remoteCertUtil) sendCommand(ctx context.Context, name string, args ...string) error {
	return w.Conn.CommandContext(ctx, name, args...).Run(testexec.DumpLogOnError)
}

func (w *remoteCertUtil) tempDir() string {
	return "/tmp"
}

type localCertUtil struct{}

func (w *localCertUtil) write(ctx context.Context, data []byte, fullPath string) error {
	return ioutil.WriteFile(fullPath, data, defaultFileMode)
}

func (w *localCertUtil) remove(ctx context.Context, fullPath string) error {
	return w.sendCommand(ctx, "rm", fullPath)
}

func (w *localCertUtil) sendCommand(ctx context.Context, name string, args ...string) error {
	return testexec.CommandContext(ctx, name, args...).Run(testexec.DumpLogOnError)
}

func (w *localCertUtil) tempDir() string {
	return os.TempDir()
}
