// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package mitmproxy implements a wrapper of mitmproxy for testing.
package mitmproxy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/process"
	"go.chromium.org/tast-tests/cros/common/testexec"
	patchpanel "go.chromium.org/tast-tests/cros/local/network/patchpanel_client"
	"go.chromium.org/tast-tests/cros/local/procutil"
	"go.chromium.org/tast-tests/cros/local/testenv"
	"go.chromium.org/tast-tests/cros/local/testenv/proxy"
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
	defaultCertFile = "mitmproxy-ca-cert.pem"
	defaultConfDir  = "/usr/local/tmp/mitmproxy"
)

// MitmProxy represents a structure of mitmproxy.
type MitmProxy struct {
	binaryPath   string
	port         int
	host         string
	outDir       string
	dumpFilePath string
	confDir      string
	compressDump bool
	cmd          *testexec.Cmd
	isRunning    bool // Is the proxy running? It is set to true on starting proxy.
	removeCert   bool // Should remove cert after test is completed?
	healthCheck  bool
	scriptPaths  []string // Addon scripts used by mitmproxy.
	options      []string // Other options provided by users. We will add --set option to command.
	lifelineFD   *os.File // Used by pathcpanel to track the lifetime of the proxy server.
}

// New creates a new MitmProxy instance with default configuration and option overrides.
func New(ctx context.Context, opts ...Option) (*MitmProxy, error) {
	mp := &MitmProxy{
		binaryPath:   DefaultBinaryPath,
		port:         DefaultListenPort,
		confDir:      defaultConfDir,
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

	// Set OutDir for saving logs and dump files per-test.
	// OutDir could be set by the context or the `OutDir` option passed in as args. If both are set, the option takes precedence.
	// In most cases, it is just okay to use the context by default except for
	// when a local RPC server calls this func with context.Background() that has no OutDir associated.
	// In this case, the `OutDir` option should be used explicitly.
	if mp.outDir == "" {
		outDir, ok := testing.ContextOutDir(ctx)
		if !ok || outDir == "" {
			return nil, errors.New("OutDir should be set in the context or in the option")
		}
		mp.outDir = outDir
	}

	return mp, nil
}

// IsRunning returns whether the proxy is running.
func (mp *MitmProxy) IsRunning() bool {
	return mp.isRunning
}

// Start launches the mitmproxy.
func (mp *MitmProxy) Start(ctx context.Context) (retErr error) {
	if err := killProcesses(ctx); err != nil {
		return errors.Wrap(err, "failed to kill running mitmproxy processes")
	}

	// If root certificate is not present,
	// we should delete the folder and then mitmproxy will recreate them.
	if _, err := mp.RootCertificate(ctx); err != nil {
		if err = mp.removeCertDir(); err != nil {
			return errors.Wrap(err, "root certificate not present, failed to delete conf dir")
		}
	}

	if err := os.MkdirAll(mp.confDir, 0700); err != nil {
		return errors.Wrapf(err, "failed to create %q for mitmdump config", mp.confDir)
	}
	// Clean up when it fails to Start.
	defer func() {
		if retErr != nil {
			mp.Close(ctx)
		}
	}()

	// The pid of the proxy process is required to configure the isolated network namespace in which the process runs.
	// The mitmdump process forks, resulting in two running processes. The pid addon will ensure the pid of the actual
	// proxy server process will be used to configure the network namespace.
	pidPath, err := mp.configurePidAddon()
	if err != nil {
		return errors.Wrap(err, "failed to configure pid addon")
	}

	// Create a new config file in the current config directory.
	configFilePath := filepath.Join(mp.confDir, "config.yaml")
	if err = mp.configureMitmdump(ctx, configFilePath); err != nil {
		return errors.Wrapf(err, "failed to create config file at %s", configFilePath)
	}

	// Run a proxy server process in non-interactive mode (mitmdump).
	nowStr := time.Now().Format("20060102-150405")
	dumpFilePath := filepath.Join(mp.outDir, fmt.Sprintf("mitmproxy_%s.dump", nowStr))
	cmd := testexec.CommandContext(ctx,
		"/sbin/minijail0", "-e", "--", mp.binaryPath, "--set", fmt.Sprintf("confdir=%s", mp.confDir), "-w", dumpFilePath)
	testing.ContextLogf(ctx, "mitmproxy: starting with cmd: %s", cmd)

	// Required for remote tast tests. mitmproxy is written in Python and uses the PyInstaller
	// to bundle the mitmproxy scripts and all its dependencies into a single file. The file
	// is written in a temp directory named "_MEI<random-number>" which is re-created for every
	// execution. The default location for the temp directory is the OS temp directory (/tmp on
	// Chrome OS). On Chrome OS, executables cannot be ran from the TMP file (b/40615995) so
	// local tast tests change the tmp dir to /usr/local/tmp; for remote tests, the temp dir
	// has to be manually changed.
	cmd.Env = append(cmd.Env, "TMPDIR=/usr/local/tmp")

	if err := cmd.Start(); err != nil {
		return errors.Wrap(err, "failed to launch proxy server")
	}

	if err = mp.configureNetwork(ctx, pidPath); err != nil {
		return errors.Wrap(err, "failed to configure the proxy server network namespace")
	}

	if mp.healthCheck {
		if err := mp.verifyProxyStart(ctx); err != nil {
			return errors.Wrap(err, "failed to health check proxy server")
		}
	}

	testing.ContextLog(ctx, "mitmproxy: started successfully, stream to: ", dumpFilePath)
	mp.cmd = cmd
	mp.dumpFilePath = dumpFilePath
	mp.isRunning = true
	return nil
}

// configureMitmdump creates the mitmdump configuration file from the existing mp.options and mp.scripts.
// The mitmdump config file format is yaml. Config file example:
//
// ---
// listen_port: 4040
// allowed_endpoints_yaml: /usr/local/share/tast/data_pushed/go.chromium.org/tast-tests/cros/local/bundles/cros/meta/data/endpoints.yml
// scripts:
// - /usr/local/share/tast/data_pushed/go.chromium.org/tast-tests/cros/local/bundles/cros/meta/data/allowed_endpoints.py
// - /usr/local/share/tast/data_pushed/go.chromium.org/tast-tests/cros/local/bundles/cros/meta/data/allowed_endpoints_yaml.py
func (mp *MitmProxy) configureMitmdump(ctx context.Context, path string) error {
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

// configureNetwork calls patchpanel to setup the network namespace for the local proxy.
func (mp *MitmProxy) configureNetwork(ctx context.Context, pidPath string) error {
	pc, err := patchpanel.New(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create patchpanel client")
	}

	pid, err := mp.getPidFromPath(ctx, pidPath)
	if err != nil {
		return errors.Wrap(err, "failed to get mitmproxy process pid")
	}

	if err != nil {
		return errors.Wrap(err, "failed to fetch the mitmproxy processes")
	}

	fd, resp, err := pc.ConnectNamespace(ctx, int32(pid), "", true)
	if err != nil {
		return err
	}
	mp.lifelineFD = fd

	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(resp.PeerIpv4Address))
	ip := net.IP(b)
	mp.host = ip.String()

	return nil
}

// getPidFromPath reads the pid of the proxy process from `pidPath`. The pid is required by
// patchpanel to setup an isolated network namespace for the proxy server with it's own network
// address.
func (mp *MitmProxy) getPidFromPath(ctx context.Context, pidPath string) (int, error) {
	var pid int
	// Wait for the proxy service to write the PID at `pidPath`.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		dat, err := ioutil.ReadFile(pidPath)
		if err != nil {
			return errors.Wrap(err, "failed to read proxy process pid")
		}
		pid, err = strconv.Atoi(strings.TrimSpace(string(dat)))
		if err != nil {
			return errors.Wrap(err, "failed to parse process pid")
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		return 0, err
	}

	return pid, nil
}

// configurePidAddon creates a mitmproxy addon script which writes the pid
// of the proxy process to a file and returns the path to the file.
func (mp *MitmProxy) configurePidAddon() (string, error) {
	// Create a temp dir where configuration and pid files can be saved.
	pidFile, err := mp.createTempFile("mitmpid")
	if err != nil {
		return "", errors.Wrap(err, "failed to create PID file")
	}

	pidScript, err := mp.createTempFile("mitma_add_on*.py")
	if err != nil {
		return "", errors.Wrap(err, "failed to create add on file")
	}

	script := fmt.Sprintf(`import os

def running():
	with open("%s","w+") as f:
		f.write(str(os.getpid()))
`, pidFile)
	err = os.WriteFile(pidScript, []byte(script), 775)

	if err != nil {
		return "", errors.Wrap(err, "failed to write add on file")
	}
	mp.scriptPaths = append(mp.scriptPaths, pidScript)
	return pidFile, nil
}

// createTempFile creates a temporary file in proxy server's config directory and returns the
// path.
func (mp *MitmProxy) createTempFile(name string) (string, error) {
	file, err := os.CreateTemp(mp.confDir, name)
	if err != nil {
		return "", errors.Wrap(err, "failed create temp file")
	}
	defer file.Close()
	if err := os.Chmod(file.Name(), 0755); err != nil {
		return "", errors.Wrapf(err, "failed to chmod %v", file.Name())
	}
	return file.Name(), nil
}

// verifyProxyStart verifies that the proxy starts with a root certificate successfully
// by checking the magic domain served locally by mitmproxy.
// However, this check can be bypassed via `HealthCheck` option in case the domain won't work with a user's allowlist or blocklist.
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

	// This would pass if the magic domain returns a 200 OK without the following error message in response.
	const respRootCANotInstalled = "traffic is not passing through mitmproxy"
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		resp, err := client.Get(testenv.MagicURL)
		if err != nil {
			return errors.Wrap(err, "failed to visit the magic domain")
		}
		if resp.StatusCode != http.StatusOK {
			return errors.Errorf("%s returns %v, want %v", testenv.MagicURL, resp.StatusCode, http.StatusOK)
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
	certFilePath := filepath.Join(mp.confDir, defaultCertFile)
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
	return fmt.Sprintf("%s:%d", mp.host, mp.port)
}

// Close closes proxy.
func (mp *MitmProxy) Close(ctx context.Context) error {
	if !mp.isRunning {
		testing.ContextLog(ctx, "mitmproxy: already closed, safe to ignore")
		return nil
	}

	var cleanupErrs []error

	// Terminate mitmproxy.
	// TODO(b/302244608): Figure out how to terminate all forked processes gracefully.
	if err := mp.cmd.Kill(); err != nil {
		cleanupErrs = append(cleanupErrs, errors.Wrap(err, "failed to terminate mitmproxy"))
	}

	// Closing the fd will signal to patchpanel that it needs to tear down the network namespace
	// for the local proxy server.
	if mp.lifelineFD != nil {
		mp.lifelineFD.Close()
		mp.lifelineFD = nil
	}

	// Compress dump file.
	if mp.compressDump {
		if _, err := os.Stat(mp.dumpFilePath); err == nil {
			targetTar := mp.dumpFilePath + ".tar.gz"
			if err := testexec.CommandContext(ctx, "tar", "-czf", targetTar, "-C", mp.outDir, mp.dumpFilePath, "--remove-files").Run(testexec.DumpLogOnError); err != nil {
				cleanupErrs = append(cleanupErrs, errors.Wrap(err, "failed to compress mitmproxy dump"))
			}
		}
	}

	if mp.removeCert {
		// Remove config files, such as certificates generated by Mitmproxy launch.
		if err := mp.removeCertDir(); err != nil {
			cleanupErrs = append(cleanupErrs, errors.Wrap(err, "failed to clean mitmproxy config"))
		}
	}

	mp.isRunning = false
	if err := errors.Join(cleanupErrs...); err != nil {
		testing.ContextLog(ctx, "mitmproxy: closed with errors: ", err)
		return err
	}
	testing.ContextLog(ctx, "mitmproxy: closed successfully")
	return nil
}

func (mp *MitmProxy) removeCertDir() error {
	if _, err := os.Stat(mp.confDir); err == nil {
		return os.RemoveAll(mp.confDir)
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

func killProcesses(ctx context.Context) error {
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
