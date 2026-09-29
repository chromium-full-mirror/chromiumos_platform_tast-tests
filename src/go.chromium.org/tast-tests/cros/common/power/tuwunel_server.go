// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
)

const (
	// TuwunelServerDefaultPort is the default port to forward the request from DUT to Tuwunel server.
	TuwunelServerDefaultPort = 8008
	// tuwunelMaxCPU is the maximum number of CPU cores supported by the Tuwunel server,
	// as it tracks available cores using a 128-bit bitmask.
	tuwunelMaxCPU         = 128
	tuwunelConfigTemplate = `
	[global]
	server_name = "powertest.localdomain"
	database_path = "%s"
	address = "0.0.0.0"
	port = %d
	allow_registration = true
	yes_i_am_very_very_sure_i_want_an_open_registration_server_prone_to_abuse = true
	db_pool_workers = 2
	db_pool_max_workers = 4
	rocksdb_parallelism_threads = 1
	db_cache_capacity_mb = 16.0
`
	signalKilledMessage = "signal: killed"
)

// logBuffer is a buffer for storing logs that supports dumping its contents.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// Write writes data d into the bytes buffer.
func (b *logBuffer) Write(d []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(d)
}

func (b *logBuffer) reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
}

// dump writes the contents of the buffer to w.
// This operation drains the buffer, making it ideal for log rotation between test cases.
func (b *logBuffer) dump(w io.Writer) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, err := b.buf.WriteTo(w)
	return err
}

func (b *logBuffer) len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Len()
}

// TuwunelServer is a wrapper for the Tuwunel server.
type TuwunelServer struct {
	cmd         *testexec.Cmd
	listener    net.Listener
	baseDir     string
	databaseDir string
	configFile  string
	binaryFile  string
	port        int
	buf         logBuffer
}

// NewTuwunelServer returns a new TuwunelServer object.
func NewTuwunelServer() *TuwunelServer {
	return &TuwunelServer{}
}

// Initiate initiates the binary and the config of the Tuwunel server.
func (t *TuwunelServer) Initiate(ctx context.Context) (retErr error) {
	defer func() {
		if retErr != nil {
			t.CleanUp()
		}
	}()

	// Find an available port and keep the listener alive to reserve the port for the server.
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return errors.Wrap(err, "failed to find an available port")
	}
	t.listener = listener
	t.port = listener.Addr().(*net.TCPAddr).Port

	baseDir := filepath.Join(os.TempDir(), fmt.Sprintf("tuwunel_%d", t.port))
	if err := os.RemoveAll(baseDir); err != nil {
		return errors.Wrap(err, "failed to cleanup directory before initiation")
	}
	dataBaseDir := filepath.Join(baseDir, "database")
	if err := os.MkdirAll(dataBaseDir, 0777); err != nil {
		return errors.Wrap(err, "failed to create Tuwunel database directory")
	}
	t.baseDir = baseDir
	t.databaseDir = dataBaseDir

	binaryFile := filepath.Join(t.baseDir, "tuwunel")
	if err := downloadTuwunelBinaryTo(ctx, binaryFile); err != nil {
		return errors.Wrap(err, "failed to download Tuwunel binary")
	}
	t.binaryFile = binaryFile

	configFile := filepath.Join(t.baseDir, "config.toml")
	tuwunelConfig := fmt.Sprintf(tuwunelConfigTemplate, t.databaseDir, t.port)
	if err := os.WriteFile(configFile, []byte(tuwunelConfig), 0777); err != nil {
		return errors.Wrap(err, "failed to write Tuwunel config")
	}
	t.configFile = configFile
	return nil
}

// CleanUp cleans up the Tuwunel server.
func (t *TuwunelServer) CleanUp() error {
	var errs []error
	if t.baseDir != "" {
		if _, err := os.Stat(t.baseDir); !os.IsNotExist(err) {
			if err := os.RemoveAll(t.baseDir); err != nil {
				errs = append(errs, errors.Wrap(err, "failed to remove Tuwunel directory"))
			}
		}
		t.baseDir = ""
		t.databaseDir = ""
	}
	if t.listener != nil {
		if err := t.listener.Close(); err != nil {
			errs = append(errs, errors.Wrap(err, "failed to close the listener"))
		}
		t.listener = nil
	}
	t.buf.reset()
	return errors.Join(errs...)
}

// Start starts the Tuwunel server.
func (t *TuwunelServer) Start(ctx context.Context) error {
	if t.binaryFile == "" || t.configFile == "" {
		return errors.New("the server is not initiated")
	}

	// Close the listener to free up the reserved port for the server to use.
	if t.listener != nil {
		if err := t.listener.Close(); err != nil {
			return errors.Wrap(err, "failed to close the listener")
		}
		t.listener = nil
	}
	// The Tuwunel server spawns threads based on the CPU cores it can access.
	// Limit its CPU affinity to a single core to avoid exceeding container thread limits.
	coreID := os.Getpid() % min(runtime.NumCPU(), tuwunelMaxCPU)
	t.cmd = testexec.CommandContext(ctx, "taskset", "-c", strconv.Itoa(coreID), t.binaryFile, "-c", t.configFile)
	// By default, jemalloc spawns background threads and creates an arena per CPU core.
	// Disable background threads and per-CPU arenas to reduce thread and memory usage.
	t.cmd.Env = append(os.Environ(),
		"MALLOC_CONF=background_thread:false,narenas:1,percpu_arena:disabled")
	t.cmd.Stdout = &t.buf
	t.cmd.Stderr = &t.buf
	if err := t.cmd.Start(); err != nil {
		return errors.Wrap(err, "failed to start Tuwunel server")
	}
	return nil
}

// Stop stops the Tuwunel server.
func (t *TuwunelServer) Stop() error {
	if t.cmd == nil {
		return errors.New("the server is not running")
	}
	if err := t.cmd.Kill(); err != nil {
		return errors.Wrap(err, "failed to kill Tuwunel server")
	}
	if err := t.cmd.Wait(); err != nil {
		// Ignore error of "signal: killed" as it's the expected behavior.
		if !strings.Contains(err.Error(), signalKilledMessage) {
			return errors.Wrap(err, "failed to wait for Tuwunel server to terminate")
		}
	}
	t.cmd = nil
	return nil
}

// Reset resets the Tuwunel server.
func (t *TuwunelServer) Reset(ctx context.Context) error {
	if err := t.Stop(); err != nil {
		return errors.Wrap(err, "failed to stop Tuwunel server")
	}
	t.buf.reset()
	if err := os.RemoveAll(t.databaseDir); err != nil {
		return errors.Wrap(err, "failed to clear Tuwunel database")
	}
	if err := os.MkdirAll(t.databaseDir, 0777); err != nil {
		return errors.Wrap(err, "failed to create Tuwunel database")
	}
	return t.Start(ctx)
}

// DumpLogs copies the collected server logs to w.
func (t *TuwunelServer) DumpLogs(w io.Writer) error {
	return t.buf.dump(w)
}

// LogLen returns the number of unread bytes in the log buffer.
func (t *TuwunelServer) LogLen() int {
	return t.buf.len()
}

// Port returns the port that is used by the Tuwunel server.
func (t *TuwunelServer) Port() int {
	return t.port
}

// downloadTuwunelBinaryTo downloads and decompresses the Tuwunel binary to the given path.
func downloadTuwunelBinaryTo(ctx context.Context, binaryFile string) error {
	tuwunelZSTURL, err := getTuwunelZSTURL(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get Tuwunel ZST URL")
	}
	resp, err := http.Get(tuwunelZSTURL)
	if err != nil {
		return errors.Wrap(err, "failed to fetch Tuwunel ZST binary")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("failed to fetch Tuwunel ZST binary: status code %d", resp.StatusCode)
	}

	decompressCommand := testexec.CommandContext(ctx, "zstd", "-d", "-o", binaryFile)
	decompressCommand.Stdin = resp.Body
	if err := decompressCommand.Run(); err != nil {
		return errors.Wrap(err, "failed to decompress Tuwunel ZST binary")
	}
	if err := os.Chmod(binaryFile, 0777); err != nil {
		return errors.Wrap(err, "failed to chmod Tuwunel binary")
	}
	return nil
}

// getTuwunelZSTURL returns the URL of the Tuwunel ZST binary based on the host architecture.
func getTuwunelZSTURL(ctx context.Context) (string, error) {
	archBytes, err := testexec.CommandContext(ctx, "uname", "-m").Output()
	if err != nil {
		return "", errors.Wrap(err, "failed to get the host arch")
	}
	arch := strings.TrimSpace(string(archBytes))

	var tuwunelArch string
	switch arch {
	case "x86_64":
		tuwunelArch = "x86_64-v3-linux-gnu"
	case "aarch64":
		tuwunelArch = "aarch64-v8-linux-gnu"
	default:
		return "", errors.Errorf("unsupported architecture: %s", arch)
	}

	tuwunelZSTURL := "https://storage.googleapis.com/chromiumos-test-assets-public/tast/cros/power/tuwunel/v1.5.0-release-all-" + tuwunelArch + "-tuwunel.zst"
	return tuwunelZSTURL, nil
}
