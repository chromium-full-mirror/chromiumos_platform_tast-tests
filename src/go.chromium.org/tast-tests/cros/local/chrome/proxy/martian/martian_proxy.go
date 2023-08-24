// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package martianproxy implements a wrapper of martianproxy for testing.
package martianproxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	defaultProxyPort = 4040
	defaultAPIPort   = 4041
	defaultOutDir    = "/var/log/martian_proxy"
)

// MartianProxy API endpoints
type proxyEndPoint string

const (
	certEndPoint      proxyEndPoint = "authority.cer"
	logsEndPoint                    = "logs"
	logsResetEndPoint               = "logs/reset"
	configure                       = "configure"
)

// MartianProxy represents a structure of the proxy wrapper.
type MartianProxy struct {
	proxyPort int
	apiPort   int
	outDir    string
	har       bool // Enable logging in HAR(HTTP Archive format) format.
	// Download log file before actively closing proxy. It does not work on situations like crash.
	downloadLogOnClose bool
	compressLog        bool
	logFileName        string
	cmd                *testexec.Cmd
	isRunning          bool // Is the proxy running? It is set to true on starting proxy.
	httpClient         *http.Client
}

// New creates a new Martian proxy instance with default configuration.
func New() *MartianProxy {
	return &MartianProxy{
		proxyPort:          defaultProxyPort,
		apiPort:            defaultAPIPort,
		outDir:             defaultOutDir,
		compressLog:        true,
		downloadLogOnClose: true,
		httpClient:         &http.Client{},
	}
}

// SetProxyPort sets the proxy port.
func (p *MartianProxy) SetProxyPort(proxyPort int) *MartianProxy {
	p.proxyPort = proxyPort
	return p
}

// ProxyPort returns the port that the proxy listens on.
func (p *MartianProxy) ProxyPort() int {
	return p.proxyPort
}

// SetAPIPort sets the api port.
func (p *MartianProxy) SetAPIPort(apiPort int) *MartianProxy {
	p.apiPort = apiPort
	return p
}

// APIPort returns the port that the api server listens on.
func (p *MartianProxy) APIPort() int {
	return p.apiPort
}

// SetOutDir sets the proxy output path.
// Har and logs will all be written to this path.
func (p *MartianProxy) SetOutDir(path string) *MartianProxy {
	p.outDir = path
	return p
}

// SetHar sets whether to log full logs in har.
func (p *MartianProxy) SetHar(har bool) *MartianProxy {
	p.har = har
	return p
}

// IsRunning returns whether the proxy is running.
func (p *MartianProxy) IsRunning() bool {
	return p.isRunning
}

// updateLogFileName updates the timestamp from when the log starts.
// MartianProxy logs are downloaded from the memory on demand.
// It is impossible to follow the log file naming convention which indicating the time log starts.
// Thus, save the start timepoint and use it for the log name.
func (p *MartianProxy) updateLogFileName() {
	nowStr := time.Now().Format("20230731-150405")
	p.logFileName = fmt.Sprintf("proxy_%s.log", nowStr)
}

// Start launches the martian proxy on localhost.
func (p *MartianProxy) Start(ctx context.Context) error {
	// MartianProxy is launched via command. Available arguments can be found at https://github.com/google/martian/
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

	// Create out dir if it does not exist.
	if err := os.MkdirAll(p.outDir, 0755); err != nil {
		return errors.Wrapf(err, "failed to create directory %q", p.outDir)
	}

	p.cmd = testexec.CommandContext(ctx, "martian_proxy", pOpts...)
	p.updateLogFileName()
	if err := p.cmd.Start(); err != nil {
		return err
	}
	p.isRunning = true
	return nil
}

// Close closes proxy.
func (p *MartianProxy) Close(ctx context.Context) error {
	if !p.isRunning {
		testing.ContextLog(ctx, "Martian proxy is not running before close")
		return nil
	}

	if p.har && p.downloadLogOnClose {
		if err := p.downloadLogs(ctx, false); err != nil {
			testing.ContextLog(ctx, "Failed to download proxy log: ", err)
		}
	}

	if err := p.cmd.Kill(); err != nil {
		return errors.Wrap(err, "failed to close proxy")
	}
	p.isRunning = false
	return p.cmd.Wait()
}

// ProxyAddress returns the proxy address to be set in browser.
func (p *MartianProxy) ProxyAddress() string {
	return fmt.Sprintf("localhost:%d", p.proxyPort)
}

func (p *MartianProxy) apiAddress(endPoint proxyEndPoint) string {
	return fmt.Sprintf("http://localhost:%d/%s", p.apiPort, endPoint)
}

type requestGeneratorFunc func() (*http.Request, error)

// ConfigureWithJSON configures the proxy with json configuration file.
// Refer to https://github.com/google/martian#configure
func (p *MartianProxy) ConfigureWithJSON(ctx context.Context, jsonFilePath string) error {
	requestGenerater := func() (*http.Request, error) {
		fileReader, err := os.Open(jsonFilePath)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to read file %s", jsonFilePath)
		}
		req, err := http.NewRequest(http.MethodPost, p.apiAddress(configure), fileReader)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create request")
		}
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	}

	resp, err := p.sendRequest(ctx, requestGenerater)
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	return err
}

// sendRequest sends the HTTP request. It automatically retries on non-2xx result.
func (p *MartianProxy) sendRequest(ctx context.Context, requestGenerater requestGeneratorFunc) (*http.Response, error) {
	var resp *http.Response
	return resp, testing.Poll(ctx, func(ctx context.Context) error {
		req, err := requestGenerater()
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to generate request"))
		}
		resp, err = p.httpClient.Do(req)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to send request to proxy"))
		} else if resp.StatusCode != http.StatusOK {
			defer resp.Body.Close()
			resBody, err := io.ReadAll(resp.Body)
			if err != nil {
				return errors.Wrap(err, "failed to read the response body")
			}
			testing.ContextLog(ctx, "Resp body: ", string(resBody[:]))
			return errors.Errorf("status code error: got %d; want %d", resp.StatusCode, http.StatusOK)
		}
		return nil
	}, &testing.PollOptions{Interval: time.Second, Timeout: 10 * time.Second})
}

// RootCertificate returns the file path of the root certificate and ensures its existence.
func (p *MartianProxy) RootCertificate(ctx context.Context) (string, error) {
	downloadedCertFile := filepath.Join(p.outDir, string(certEndPoint))
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return p.downloadDataFromProxy(ctx, certEndPoint, downloadedCertFile)
	}, &testing.PollOptions{Interval: time.Second, Timeout: 10 * time.Second}); err != nil {
		return "", errors.Wrap(err, "failed to download the cert file")
	}
	return downloadedCertFile, nil
}

func (p *MartianProxy) downloadDataFromProxy(ctx context.Context, endpoint proxyEndPoint, downloadPath string) error {
	out, err := os.Create(downloadPath)
	if err != nil {
		return err
	}
	defer out.Close()

	req := func() (*http.Request, error) {
		return http.NewRequest(http.MethodGet, p.apiAddress(endpoint), nil)
	}

	resp, err := p.sendRequest(ctx, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// resetLog calls the proxy api to delete logs in memory and restart logging.
func (p *MartianProxy) resetLog(ctx context.Context) error {
	defer p.updateLogFileName()
	req := func() (*http.Request, error) {
		return http.NewRequest(http.MethodDelete, p.apiAddress(logsResetEndPoint), nil)
	}
	resp, err := p.sendRequest(ctx, req)
	defer resp.Body.Close()
	return err
}

// downloadLogs downloads proxy log from API and resets logging.
func (p *MartianProxy) downloadLogs(ctx context.Context, reset bool) error {
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
