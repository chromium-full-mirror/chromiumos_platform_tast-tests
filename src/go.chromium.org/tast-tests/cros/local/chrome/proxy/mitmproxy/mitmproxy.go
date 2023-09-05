// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package mitmproxy implements a wrapper of mitmproxy for testing.
package mitmproxy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// DefaultListenPort is the default port that the proxy is listening.
	DefaultListenPort = 4040
	// DefaultBinaryPath is the default binary path to be used.
	DefaultBinaryPath = "/usr/local/bin/mitmdump"
)

const (
	defaultConfDir = "/usr/local/tmp/mitmproxy"
	certFile       = "mitmproxy-ca-cert.pem"
	defaultOutDir  = "/usr/local/tmp/mitmproxy"
)

// MitmProxy represents a structure of mitmproxy.
type MitmProxy struct {
	binaryPath   string
	port         int
	outDir       string
	dumpFileName string
	confDir      string
	compressDump bool
	cmd          *testexec.Cmd
	isRunning    bool   // Is the proxy running? It is set to true on starting proxy.
	removeCert   bool   // Should remove cert after test is completed?
	scriptPath   string // Addon script used by mitmproxy
}

// New creates a new MitmDump instance with default configuration.
func New() *MitmProxy {
	return &MitmProxy{
		binaryPath:   DefaultBinaryPath,
		port:         DefaultListenPort,
		confDir:      defaultConfDir,
		outDir:       defaultOutDir,
		compressDump: true,
		removeCert:   true,
	}
}

// SetRemoveCert sets whether to remove cert.
func (mp *MitmProxy) SetRemoveCert(removeCert bool) *MitmProxy {
	mp.removeCert = removeCert
	return mp
}

// SetScriptPath sets the path of addon script.
func (mp *MitmProxy) SetScriptPath(scriptPath string) *MitmProxy {
	mp.scriptPath = scriptPath
	return mp
}

// SetBinaryPath sets the binary path of mitmproxy.
func (mp *MitmProxy) SetBinaryPath(binaryPath string) *MitmProxy {
	mp.binaryPath = binaryPath
	return mp
}

// SetListenPort sets the listening port of mitmproxy.
func (mp *MitmProxy) SetListenPort(port int) *MitmProxy {
	mp.port = port
	return mp
}

// ListenPort returns the port mitmproxy intends to listen on.
func (mp *MitmProxy) ListenPort() int {
	return mp.port
}

// SetOutDir sets the mitmproxy output path.
// Output includes dump and log.
func (mp *MitmProxy) SetOutDir(path string) *MitmProxy {
	mp.outDir = path
	return mp
}

// SetConfDir sets the mitmproxy config path.
func (mp *MitmProxy) SetConfDir(confDir string) *MitmProxy {
	mp.confDir = confDir
	return mp
}

// IsRunning returns whether the proxy is running.
func (mp *MitmProxy) IsRunning() bool {
	return mp.isRunning
}

// Start launches the mitmproxy.
func (mp *MitmProxy) Start(ctx context.Context) error {
	nowStr := time.Now().Format("20230731-150405")
	dumpFileName := fmt.Sprintf("mitmproxy_%s.dump", nowStr)
	dumpFilePath := filepath.Join(mp.outDir, dumpFileName)

	logFileName := fmt.Sprintf("mitmproxy_%s.log", nowStr)
	logFilePath := filepath.Join(mp.outDir, logFileName)

	if err := os.MkdirAll(mp.confDir, 0700); err != nil {
		return errors.Wrapf(err, "failed to create %q for mitmdump config", mp.confDir)
	}
	if err := os.MkdirAll(mp.outDir, 0700); err != nil {
		return errors.Wrapf(err, "failed to create %q for mitmdump output dir", mp.outDir)
	}

	args := []string{
		"--set", fmt.Sprintf("listen_port=%d", mp.port),
		"--set", fmt.Sprintf("confdir=%s", mp.confDir),
		"-w", dumpFilePath,
	}

	if len(mp.scriptPath) > 0 {
		args = append(args, "-s", mp.scriptPath)
	}

	// We redirect mitmproxy output to file.
	// We run proxy in a non-block way, so we cannot print logs until cmd is killed.
	// To avoid any log loss, we rediret log to file to make debug easier.
	proxyStart := fmt.Sprintf("%s %s > %s", mp.binaryPath, strings.Join(args, " "), logFilePath)
	testing.ContextLogf(ctx, "command to start proxy is %s", proxyStart)
	cmd := testexec.CommandContext(ctx, "bash", "-c", proxyStart)

	if err := cmd.Start(); err != nil {
		return errors.Wrap(err, "failed to launch proxy server")
	}

	if err := mp.verifyProxyStart(ctx); err != nil {
		return errors.Wrap(err, "Mitmproxy fails to start")
	}

	testing.ContextLog(ctx, "Mitmproxy is successfully launched. Streaming to ", dumpFilePath)

	mp.cmd = cmd
	mp.dumpFileName = dumpFileName
	mp.isRunning = true

	return nil
}

// verifyProxyStart verifys that the proxy starts successfully by get google home page.
func (mp *MitmProxy) verifyProxyStart(ctx context.Context) error {
	// Get cert.
	certFilePath, err := mp.RootCertificate(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to find cert file path")
	}

	caCert, err := os.ReadFile(certFilePath)
	if err != nil {
		return errors.Wrap(err, "failed to read cert file")
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	// Get Proxy.
	proxyURLStr := fmt.Sprintf("http://localhost:%d", mp.port)
	proxyURL, err := url.Parse(proxyURLStr)
	if err != nil {
		return errors.Wrapf(err, "failed to parse url: %s", proxyURLStr)
	}

	// Setup http client.
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: caCertPool,
			},
			Proxy: http.ProxyURL(proxyURL),
		},
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := client.Get("https://www.google.com"); err != nil {
			return errors.Wrap(err, "failed to access google homepage")
		}
		return nil
	}, &testing.PollOptions{Timeout: 2 * time.Second}); err != nil {
		return errors.Wrap(err, "mitmproxy fails to start")
	}

	return nil
}

// RootCertificate returns the file path of the root certificate and ensures its existence.
func (mp *MitmProxy) RootCertificate(ctx context.Context) (string, error) {
	certFilePath := filepath.Join(mp.confDir, certFile)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := os.Stat(certFilePath); err != nil {
			return errors.Wrapf(err, "%s is unavailable", certFilePath)
		}
		return nil
	}, &testing.PollOptions{Timeout: 20 * time.Second}); err != nil {
		return "", errors.Wrapf(err, "failed to locate cert file at %q, maybe the proxy server is not launched successfully", certFilePath)
	}

	return certFilePath, nil
}

// ProxyAddress returns the proxy address to be set in browser.
func (mp *MitmProxy) ProxyAddress() string {
	return fmt.Sprintf("localhost:%d", mp.port)
}

// Close closes proxy.
func (mp *MitmProxy) Close(ctx context.Context) error {
	if !mp.isRunning {
		testing.ContextLog(ctx, "mitmproxy is not running before close")
		return nil
	}

	var cleanupErrs []error
	dumpFilePath := filepath.Join(mp.outDir, mp.dumpFileName)

	// Terminate mitmproxy.
	if err := mp.cmd.Kill(); err != nil {
		cleanupErrs = append(cleanupErrs, errors.Wrap(err, "failed to terminate mitmproxy"))
	} else {
		mp.isRunning = false
	}

	// Compress dump file.
	if mp.compressDump {
		targetTar := dumpFilePath + ".tar.gz"
		if err := testexec.CommandContext(ctx, "tar", "-czf", targetTar, "-C", mp.outDir, mp.dumpFileName, "--remove-files").Run(testexec.DumpLogOnError); err != nil {
			cleanupErrs = append(cleanupErrs, errors.Wrap(err, "failed to compress mitmproxy dump"))
		}
	}

	if mp.removeCert {
		// Remove config files, such as certificates generated by Mitmproxy launch.
		if err := os.RemoveAll(mp.confDir); err != nil {
			cleanupErrs = append(cleanupErrs, errors.Wrap(err, "failed to clean mitmproxy config"))
		}
	}

	return errors.Join(cleanupErrs...)
}
