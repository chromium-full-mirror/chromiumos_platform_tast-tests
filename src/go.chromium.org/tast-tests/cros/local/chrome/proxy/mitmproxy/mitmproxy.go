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
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/process"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome/proxy"
	"go.chromium.org/tast-tests/cros/local/procutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// DefaultListenPort is the default port that the proxy is listening.
	DefaultListenPort = 4040
	// DefaultBinaryPath is the default binary path to be used.
	DefaultBinaryPath = "/usr/local/bin/mitmdump"
	// MitmdumpBinFile is the name of mitmdump binary.
	MitmdumpBinFile = "mitmdump_bin"
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
	isRunning    bool // Is the proxy running? It is set to true on starting proxy.
	removeCert   bool // Should remove cert after test is completed?
	healthCheck  bool
	scriptPaths  []string // Addon scripts used by mitmproxy.
	options      []string // Other options provided by users. We will add --set option to command.
}

// New creates a new MitmDump instance with default configuration.
func New(opts ...Option) (*MitmProxy, error) {
	mp := &MitmProxy{
		binaryPath:   DefaultBinaryPath,
		port:         DefaultListenPort,
		confDir:      defaultConfDir,
		outDir:       defaultOutDir,
		compressDump: true,
		removeCert:   true,
		healthCheck:  true,
		scriptPaths:  []string{},
		options:      []string{},
	}

	// Get values from command line.
	if proxy.ScriptPath() != "" {
		mp.scriptPaths = []string{proxy.ScriptPath()}
	}

	// Override any value if users pass option from test.
	for _, opt := range opts {
		if err := opt(mp); err != nil {
			return nil, err
		}
	}

	return mp, nil
}

// IsRunning returns whether the proxy is running.
func (mp *MitmProxy) IsRunning() bool {
	return mp.isRunning
}

// Start launches the mitmproxy.
func (mp *MitmProxy) Start(ctx context.Context) error {
	if err := killProcessesIfFound(ctx); err != nil {
		return errors.Wrap(err, "fail to cleanup existing mitmproxy process")
	}

	nowStr := time.Now().Format("20060102-150405")

	dumpFileName := fmt.Sprintf("mitmproxy_%s.dump", nowStr)
	dumpFilePath := filepath.Join(mp.outDir, dumpFileName)

	// If root certificate is not present,
	// we should delete the folder and then mitmproxy will recreate them.
	if _, err := mp.RootCertificate(ctx); err != nil {
		if err = mp.cleanupCert(); err != nil {
			return errors.Wrap(err, "root certificate is not present, but fail to delete conf folder")
		}
	}

	if err := os.MkdirAll(mp.confDir, 0700); err != nil {
		return errors.Wrapf(err, "failed to create %q for mitmdump config", mp.confDir)
	}
	if err := os.MkdirAll(mp.outDir, 0700); err != nil {
		return errors.Wrapf(err, "failed to create %q for mitmdump output dir", mp.outDir)
	}

	// Create a new config file in the current config directory.
	configFilePath := filepath.Join(mp.confDir, "config.yaml")
	if err := mp.writeConfigFile(ctx, configFilePath); err != nil {
		return errors.Wrapf(err, "failed to create config file at %s", configFilePath)
	}

	cmd := testexec.CommandContext(ctx, "/sbin/minijail0", mp.binaryPath, "--set", fmt.Sprintf("confdir=%s", mp.confDir), "-w", dumpFilePath)

	if err := cmd.Start(); err != nil {
		return errors.Wrap(err, "failed to launch proxy server")
	}

	if mp.healthCheck {
		if err := mp.verifyProxyStart(ctx); err != nil {
			return errors.Wrap(err, "failed to health check proxy server")
		}
	}

	testing.ContextLog(ctx, "Mitmproxy is successfully launched. Streaming to ", dumpFilePath)

	mp.cmd = cmd
	mp.dumpFileName = dumpFileName
	mp.isRunning = true

	return nil
}

// writeConfigFile creates the mitmdump configuration file from the existing mp.options and mp.scripts.
// The mitmdump config file format is yaml. Config file example:
//
// ---
// listen_port: 4040
// allowed_endpoints_yaml: /usr/local/share/tast/data_pushed/go.chromium.org/tast-tests/cros/local/bundles/cros/meta/data/endpoints.yml
// scripts:
// - /usr/local/share/tast/data_pushed/go.chromium.org/tast-tests/cros/local/bundles/cros/meta/data/allowed_endpoints.py
// - /usr/local/share/tast/data_pushed/go.chromium.org/tast-tests/cros/local/bundles/cros/meta/data/allowed_endpoints_yaml.py
func (mp *MitmProxy) writeConfigFile(ctx context.Context, path string) error {
	configs := []string{"---"}
	configs = append(configs, fmt.Sprintf(`listen_port: %d`, mp.port))
	configs = append(configs, mp.options...)

	if len(mp.scriptPaths) > 0 {
		scripts := append([]string{"scripts:"}, mp.scriptPaths...)
		configs = append(configs, strings.Join(scripts, "\n - "))
	}

	yamlConfig := strings.Join(configs, "\n")
	testing.ContextLog(ctx, "Using the mitmproxy configuration: ", yamlConfig)

	f, err := os.Create(path)

	if err != nil {
		return errors.Wrap(err, "failed to create config file")
	}
	defer f.Close()

	_, err = f.WriteString(yamlConfig)
	if err != nil {
		return errors.Wrap(err, "failed to write config file")
	}
	return nil
}

// verifyProxyStart verifies that the proxy starts successfully by get youtube home page.
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
	proxyURLStr := "http://" + mp.ProxyAddress()
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

	// This would pass if the magic domain "mitm.it" returns a 200 OK without the following error message in response.
	const respRootCANotInstalled = "traffic is not passing through mitmproxy"
	const testURL = "https://mitm.it/"
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		resp, err := client.Get(testURL)
		if err != nil {
			return errors.Wrap(err, "failed to visit the magic domain")
		}
		if resp.StatusCode != http.StatusOK {
			return errors.Errorf("%s returns %v, want %v", testURL, resp.StatusCode, http.StatusOK)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to read response"))
		}
		if strings.Contains(string(body), respRootCANotInstalled) {
			return testing.PollBreak(errors.New("mitmproxy root certificate is not properly installed"))
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 2 * time.Second}); err != nil {
		return errors.Wrap(err, "mitmproxy: failed to start with root certificate")
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
		if err := mp.cleanupCert(); err != nil {
			cleanupErrs = append(cleanupErrs, errors.Wrap(err, "failed to clean mitmproxy config"))
		}
	}

	return errors.Join(cleanupErrs...)
}

func (mp *MitmProxy) cleanupCert() error {
	if err := os.RemoveAll(mp.confDir); err != nil {
		return err
	}

	return nil
}

func processes(ctx context.Context) ([]*process.Process, error) {
	return procutil.FindAll(func(p *process.Process) bool {
		exe, err := p.Exe()
		// Both mitmdump and mitmproxy are process of mitmproxy.
		// mitmdump is often used in automation test.
		// mitmproxy is often used in manual test.
		// Besides, we only try best but not guarantee to kill any proxy process.
		// because I believe the error is highly likely from other unrelated processes.
		return err == nil && (strings.HasSuffix(exe, "mitmdump") || strings.HasSuffix(exe, "mitmproxy"))
	})
}

func killProcessesIfFound(ctx context.Context) error {
	procs, err := processes(ctx)
	// ErrNotFound is returned when no proc is found.
	if err == procutil.ErrNotFound {
		return nil
	} else if err != nil {
		return errors.Wrap(err, "fail to get mitmproxy processes")
	}

	for _, proc := range procs {
		if err := proc.Kill(); err != nil {
			return errors.Wrapf(err, "fail to send kill signal %s", proc.String())
		}
		if err := procutil.WaitForTerminated(ctx, proc, 10*time.Second); err != nil {
			return errors.Wrapf(err, "fail to kill mitmproxy process %s", proc.String())
		}
	}

	return nil
}
