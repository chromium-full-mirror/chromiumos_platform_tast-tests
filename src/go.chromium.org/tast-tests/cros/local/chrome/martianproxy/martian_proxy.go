// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package martianproxy implements a wrapper of martianproxy for testing.
package martianproxy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	defaultProxyPort = 4040
	defaultAPIPort   = 4041
	defaultOutDir    = "/var/log/martian_proxy"
)

// Proxy API endpoints
type proxyEndPoint string

const (
	certEndPoint      proxyEndPoint = "authority.cer"
	logsEndPoint                    = "logs"
	logsResetEndPoint               = "logs/reset"
)

// Proxy represents a structure of the proxy wrapper.
type Proxy struct {
	proxyPort int
	apiPort   int
	outDir    string
	har       bool // Enable logging in HAR(HTTP Archive format) format.
	// Download log file before actively closing proxy. It does not work on situations like crash.
	downloadLogOnClose bool
	compressLog        bool
	logFileName        string
	cmd                *testexec.Cmd
}

// New creates a new Martian proxy instance with default configuration.
func New() *Proxy {
	return &Proxy{
		proxyPort:          defaultProxyPort,
		apiPort:            defaultAPIPort,
		outDir:             defaultOutDir,
		compressLog:        true,
		downloadLogOnClose: true,
	}
}

// SetProxyPort sets the proxy port.
func (p *Proxy) SetProxyPort(proxyPort int) *Proxy {
	p.proxyPort = proxyPort
	return p
}

// ProxyPort returns the port that the proxy listens on.
func (p *Proxy) ProxyPort() int {
	return p.proxyPort
}

// SetAPIPort sets the api port.
func (p *Proxy) SetAPIPort(apiPort int) *Proxy {
	p.apiPort = apiPort
	return p
}

// APIPort returns the port that the api server listens on.
func (p *Proxy) APIPort() int {
	return p.apiPort
}

// SetOutDir sets the proxy output path.
// Har and logs will all be written to this path.
func (p *Proxy) SetOutDir(path string) *Proxy {
	p.outDir = path
	return p
}

// SetHar sets whether to log full logs in har.
func (p *Proxy) SetHar(har bool) *Proxy {
	p.har = har
	return p
}

// updateLogFileName updates the timestamp from when the log starts.
// Proxy logs are downloaded from the memory on demand.
// It is impossible to follow the log file naming convention which indicating the time log starts.
// Thus, save the start timepoint and use it for the log name.
func (p *Proxy) updateLogFileName() {
	nowStr := time.Now().Format("20230731-150405")
	p.logFileName = fmt.Sprintf("proxy_%s.log", nowStr)
}

// Start launches the martian proxy on localhost.
func (p *Proxy) Start(ctx context.Context) error {
	// Proxy is launched via command. Available arguments can be found at https://github.com/google/martian/blob/master/cmd/proxy/main.go
	// martian_proxy -generate-ca-cert -har -addr=:4040 -api-addr=:4041 -api=localhost
	pOpts := []string{
		"-generate-ca-cert",                     // Generate ca cert that can be downloaded from http://localhost:{api_port}/certificate.cer
		"-api=localhost",                        // Use localhost as api server to avoid dead loop.
		fmt.Sprintf("-addr=:%d", p.proxyPort),   // set proxy port
		fmt.Sprintf("-api-addr=:%d", p.apiPort), // set api port
	}
	if p.har {
		pOpts = append(pOpts, "-har")
	}

	p.cmd = testexec.CommandContext(ctx, "martian_proxy", pOpts...)
	p.updateLogFileName()
	return p.cmd.Start()
}

// Close closes proxy.
func (p *Proxy) Close(ctx context.Context) error {
	if p.cmd == nil {
		testing.ContextLog(ctx, "Martian proxy is not running before close")
		return nil
	}

	if p.downloadLogOnClose {
		if err := p.downloadLogs(ctx, false); err != nil {
			return errors.Wrap(err, "failed to download proxy log")
		}
	}

	if err := p.cmd.Kill(); err != nil {
		return errors.Wrap(err, "failed to close proxy")
	}
	return p.cmd.Wait()
}

// ProxyAddress returns the proxy address to be set in browser.
func (p *Proxy) ProxyAddress() string {
	return fmt.Sprintf("localhost:%d", p.proxyPort)
}

func (p *Proxy) apiAddress(endPoint proxyEndPoint) string {
	return fmt.Sprintf("http://localhost:%d/%s", p.apiPort, endPoint)
}

// ImportRootCertificate imports the proxy root certificate to user trusted cert store using certutil cmd.
func (p *Proxy) ImportRootCertificate(ctx context.Context, normalizedUser string) error {
	downloadedCertFile := filepath.Join(p.outDir, string(certEndPoint))

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return p.downloadDataFromProxy(ctx, certEndPoint, downloadedCertFile)
	}, &testing.PollOptions{Interval: time.Second, Timeout: 10 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to download the cert file")
	}
	defer os.Remove(downloadedCertFile)

	userHome, err := cryptohome.UserPath(ctx, normalizedUser)
	if err != nil {
		return errors.Wrap(err, "failed to get user path")
	}

	cmd := testexec.CommandContext(ctx, "certutil", "-d", fmt.Sprintf("sql:%s/.pki/nssdb", userHome),
		"-A", "-t", "C,C,C", "-n", "martian.proxy", "-i", downloadedCertFile)
	if err := cmd.Run(); err != nil {
		cmd.DumpLog(ctx)
		return errors.Wrap(err, "failed to import certificate")
	}
	return nil
}

func (p *Proxy) downloadDataFromProxy(ctx context.Context, endpoint proxyEndPoint, downloadPath string) error {
	return testexec.CommandContext(ctx, "curl", p.apiAddress(endpoint), "-o", downloadPath).Run()
}

// resetLog calls the proxy api to delete logs in memory and restart logging.
func (p *Proxy) resetLog(ctx context.Context) error {
	defer p.updateLogFileName()
	return testexec.CommandContext(ctx, "curl", "-X", "DELETE", p.apiAddress(logsResetEndPoint)).Run(testexec.DumpLogOnError)
}

// downloadLogs downloads proxy log from API and resets logging.
func (p *Proxy) downloadLogs(ctx context.Context, reset bool) error {
	logFile := filepath.Join(p.outDir, p.logFileName)
	if err := p.downloadDataFromProxy(ctx, logsEndPoint, logFile); err != nil {
		return errors.Wrap(err, "failed to download proxy log")
	}

	if p.compressLog {
		targetTar := logFile + ".tar.gz"
		if err := testexec.CommandContext(ctx, "tar", "-czf", targetTar, "-C", p.outDir, p.logFileName, "--remove-files").Run(testexec.DumpLogOnError); err != nil {
			return errors.Wrap(err, "failed to compress proxy log")
		}
	}

	if reset {
		return p.resetLog(ctx)
	}
	return nil
}
