// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package nebraska provides start/stop functions for Nebraska.
package nebraska

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const localhost = "http://127.0.0.1"

// Nebraska struct hold Nebraska server runtime information.
type Nebraska struct {
	cleanup []func(ctx context.Context) error
	cmd     *testexec.Cmd

	Port int
	// Log file during runtime of Nebraska.
	LogFile string
	Root    string
}

// New starts the Nebraska server and returns the Nebraska struct on a
// successful bringup, otherwise an error is returned.
func New(ctx context.Context, options ...Option) (instance *Nebraska, err error) {
	config, err := NewConfig(options)
	instance = &Nebraska{
		cleanup: config.m.Cleanup,
		LogFile: filepath.Join(config.m.RuntimeRoot, config.m.LogFileName),
		Root:    config.m.RuntimeRoot,
	}

	// Schedule cleanup of any partial setup in case of error.
	ctxForCleanup := ctx
	ctx, _ = ctxutil.Shorten(ctx, 10*time.Second) // Save 10s for cleanup.
	defer func(ctxForCleanup context.Context) {
		if err != nil {
			instance.Close(ctxForCleanup)
		}
	}(ctxForCleanup)
	if err != nil {
		return nil, errors.Wrap(err, "failed to process options")
	}

	if err := startNebraska(ctx, instance, append(config.m.Args, argsLogFile, instance.LogFile)); err != nil {
		return nil, errors.Wrap(err, "failed to start Nebraska")
	}

	if err := config.m.ConfigureUpdateEngine(instance.Port); err != nil {
		return nil, errors.Wrap(err, "failed to configure update engine")
	}

	return instance, nil
}

func startNebraska(ctx context.Context, instance *Nebraska, args []string) error {
	testing.ContextLogf(ctx, "Starting Nebraska with args %v in root %s", args, instance.Root)
	cmd := testexec.CommandContext(ctx, "nebraska.py", args...)
	if err := cmd.Start(); err != nil {
		return errors.Wrap(err, "failed to run Nebraska")
	}

	port, err := waitForPort(ctx, instance.Root)
	if err != nil {
		return errors.Wrap(err, "could not get Nebraska port")
	}

	instance.cmd = cmd
	instance.Port = *port
	return nil
}

func waitForPort(ctx context.Context, root string) (*int, error) {
	portPath := filepath.Join(root, "port")

	// Try a few times to make sure Nebraska is up.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := os.Stat(portPath); err != nil {
			if os.IsNotExist(err) {
				return err
			}
			return testing.PollBreak(err)
		}
		return nil
	}, &testing.PollOptions{Timeout: time.Second * 5}); err != nil {
		return nil, errors.Wrap(err, "Nebraska did not start")
	}

	portStr, err := ioutil.ReadFile(portPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read the Nebraska's port file")
	}

	port, err := strconv.Atoi(string(portStr))
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse the Nebraska's port file")
	}

	return &port, nil
}

// Close stops Nebraska and dumps its log file.
func (n *Nebraska) Close(ctx context.Context) (err error) {
	testing.ContextLog(ctx, "Stopping Nebraska")
	// Log any errors as most users may be in the process of cleaning up and not check for them.
	defer func(ctx context.Context) {
		if err != nil {
			testing.ContextLog(ctx, "Error while closing Nebraska: ", err)
		}
	}(ctx)

	if n.cmd != nil {
		err = n.stopNebraska(ctx)
	}
	// Always dump logs after stopping.
	err = errors.Join(err, n.dumpLog(ctx))

	for _, cleanup := range n.cleanup {
		err = errors.Join(err, cleanup(ctx))
	}
	return err
}

func (n *Nebraska) stopNebraska(ctx context.Context) error {
	// Kill the Nebraska instance with SIGTERM so it has time to remove port/pid files
	// and cleanup properly.
	// If this fails, there is no need to kill the process, it will be killed on context timeout by a goroutine started in testexec.
	if err := n.cmd.Signal(unix.SIGTERM); err != nil {
		return errors.Wrap(err, "failed to send SIGTERM to Nebraska process")
	}
	// Wait for Nebraska to free resources.
	if err := n.cmd.Wait(); err != nil {
		// Attempt to kill the process
		return errors.Wrap(err, "failed to wait for Nebraska to stop with SIGTERM")
	}

	return nil
}

func (n *Nebraska) dumpLog(ctx context.Context) error {
	dir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return errors.New("failed to get testing out directory")
	}

	logs, err := os.ReadFile(n.LogFile)
	if err != nil {
		return errors.Wrap(err, "failed to read Nebraska log for dump")
	}

	time := time.Now().UTC().Format(time.RFC3339Nano)
	fileExt := filepath.Ext(n.LogFile)
	fileName := strings.TrimSuffix(filepath.Base(n.LogFile), fileExt)
	dumpedLog := fmt.Sprintf("%s-%s%s", fileName, time, fileExt)
	if err := os.WriteFile(filepath.Join(dir, dumpedLog), logs, 0644); err != nil {
		return errors.Wrap(err, "failed to write Nebraska log")
	}

	testing.ContextLog(ctx, "Saved Nebraska log to ", dumpedLog)

	return nil
}

const (
	// CriticalUpdate is passed to UpdateURL to indicate a critical update should be returned, i.e. it will contain "deadline=now".
	CriticalUpdate = true
	// NormalUpdate is passed to UpdateURL to make it return a normal update.
	NormalUpdate = false
)

// UpdateURL returns the URL to request updates from the Nebraska instance running on port.
func UpdateURL(port int, isCritical bool) string {
	critical := "False"
	if isCritical {
		critical = "True"
	}

	return fmt.Sprintf("%s:%d/update?critical_update=%s", localhost, port, critical)
}

// SetFakedMetadata writes faked metadata to Nebraska's runtime directory and points the server towards it.
// This allows for update requests with responses without actually having to provide any payload.
func (n *Nebraska) SetFakedMetadata(ctx context.Context) error {
	const metadataFileName = "update_payload.json"

	fakePayload := struct {
		Appid             string  `json:"appid"`
		IsDelta           bool    `json:"is_delta"`
		MetadataSignature *string `json:"metadata_signature"`
		MetadataSize      int     `json:"metadata_size"`
		Sha256Hex         string  `json:"sha256_hex"`
		Size              int     `json:"size"`
		TargetVersion     string  `json:"target_version"`
		Version           int     `json:"version"`
	}{
		Appid:             "{NONESENSE}",
		IsDelta:           false,
		MetadataSignature: nil,
		MetadataSize:      74204,
		Sha256Hex:         "/gu3P+FBIioa5ILaBNKysJ/uWQiIId/mNEn9qCT/0/k=",
		Size:              1401355452,
		TargetVersion:     "99999.0.0",
		Version:           2,
	}
	metadataFileContent, err := json.Marshal(fakePayload)
	if err != nil {
		return errors.Wrap(err, "failed to serialize update payload")
	}

	if err := os.WriteFile(filepath.Join(n.Root, metadataFileName), []byte(metadataFileContent), 0644); err != nil {
		return errors.Wrap(err, "failed to write metadata file to temp directory")
	}

	if err := n.SetIgnoreAppID(ctx, true); err != nil {
		return errors.Wrap(err, "failed to set ignore app id")
	}

	if err := n.SetUpdateMetadata(ctx, n.Root); err != nil {
		return errors.Wrapf(err, "failed to set update metadata folder to %q", n.Root)
	}

	return nil
}

// SetUpdateMetadata configures Nebraska to load update metadata from the given folder.
func (n *Nebraska) SetUpdateMetadata(ctx context.Context, folder string) error {
	return configureNebraska(ctx, n.Port, "update_metadata", fmt.Sprintf("%q", folder))
}

// SetUpdatePayloadsAddress configures Nebraska to fetch update payload from the given url.
func (n *Nebraska) SetUpdatePayloadsAddress(ctx context.Context, url string) error {
	return configureNebraska(ctx, n.Port, "update_payloads_address", fmt.Sprintf("%q", url))
}

// SetIsRollback configures Nebraska to serve the next update as rollback or not.
func (n *Nebraska) SetIsRollback(ctx context.Context, isRollback bool) error {
	return configureNebraska(ctx, n.Port, "is_rollback", fmt.Sprint(isRollback))
}

// SetRollbackPreventionImage configures Nebraska to send firmware and kernel version of the served image.
// Pass a two-value array of the form [kernel, firmware], e.g. ["1.2", "3.1"].
func (n *Nebraska) SetRollbackPreventionImage(ctx context.Context, firmwareAndKernelVersion [2]string) error {
	return configureNebraska(ctx, n.Port, "rollback_prevention_image",
		fmt.Sprintf("[%q, %q]", firmwareAndKernelVersion[0], firmwareAndKernelVersion[1]))
}

// SetIgnoreAppID configures Nebraska to skip comparison of payload metadata and device's app id.
func (n *Nebraska) SetIgnoreAppID(ctx context.Context, ignoreAppID bool) error {
	return configureNebraska(ctx, n.Port, "ignore_appid", fmt.Sprint(ignoreAppID))
}

// SetInvalidateLastUpdate configures Nebraska to return an invalidation of installed update.
func (n *Nebraska) SetInvalidateLastUpdate(ctx context.Context, value bool) error {
	return configureNebraska(ctx, n.Port, "invalidate_last_update", fmt.Sprint(value))
}

// SetCriticalUpdate configures Nebraska to serve the next update as critical update or not.
func (n *Nebraska) SetCriticalUpdate(ctx context.Context, value bool) error {
	return configureNebraska(ctx, n.Port, "critical_update", fmt.Sprint(value))
}

// SetEolDate sets the end of life date Nebraska sends on update requests.
func (n *Nebraska) SetEolDate(ctx context.Context, eolDate time.Time) error {
	return configureNebraska(ctx, n.Port, "eol_date", fmt.Sprint(toDaysSinceUnixEpoch(eolDate)))
}

// SetExtendedDate sets the extended date for extended auto updates that Nebraska sends on update requests.
func (n *Nebraska) SetExtendedDate(ctx context.Context, extendedDate time.Time) error {
	return configureNebraska(ctx, n.Port, "extended_date", fmt.Sprint(toDaysSinceUnixEpoch(extendedDate)))
}

// SetExtendedOptInRequired sets the extended opt in required boolean Nebraska sends on update requests.
func (n *Nebraska) SetExtendedOptInRequired(ctx context.Context, value bool) error {
	return configureNebraska(ctx, n.Port, "extended_opt_in_required", fmt.Sprint(value))
}

func configureNebraska(ctx context.Context, port int, key, value string) error {
	command := fmt.Sprintf("curl -X POST -d '{%q: %v}' %s", key, value, configURL(port))

	testing.ContextLog(ctx, command)
	if err := testexec.CommandContext(ctx, "sh", "-c", command).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to configure Nebraska")
	}
	return nil
}

func configURL(port int) string {
	return fmt.Sprintf("%s:%d/update_config", localhost, port)
}

func toDaysSinceUnixEpoch(date time.Time) int64 {
	// Omaha sends days since unix epoch instead of seconds. Convert by dividing by seconds * minutes * hours.
	return date.Unix() / (60 * 60 * 24)
}
