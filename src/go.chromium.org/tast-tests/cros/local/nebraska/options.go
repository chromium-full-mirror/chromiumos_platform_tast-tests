// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package nebraska

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"

	"go.chromium.org/tast/core/errors"
)

const (
	argsUpdateMetadata         = "--update-metadata"
	argsInstallMetadata        = "--install-metadata"
	argsUpdatePayloadsAddress  = "--update-payloads-address"
	argsInstallPayloadsAddress = "--install-payloads-address"
	argsPort                   = "--port"
	argsRuntimeRoot            = "--runtime-root"
	argsLogFile                = "--log-file"
)

const tmpPathPattern = "nebraska-"

const logFileName = "nebraska.log"

const statefulLSBRelease = "/mnt/stateful_partition/etc/lsb-release"

// runInTmpDir creates a temporary directory to run Nebraska in.
// This option is always added and not configurable from the outside.
func runInTmpDir() Option {
	return func(config *MutableConfig) error {
		root, err := ioutil.TempDir("", tmpPathPattern)
		if err != nil {
			return errors.Wrap(err, "failed to create temporary runtime root for Nebraska")
		}

		// Schedule cleanup of temp directory.
		config.Cleanup = append(config.Cleanup,
			func(ctx context.Context) error {
				return os.RemoveAll(root)
			})

		if err := runtimeRoot(root)(config); err != nil {
			return errors.Wrap(err, "failed to place logs the temporary directory")
		}

		return nil
	}

}

// runtimeRoot configures the directory Nebraska runs in. Always run Nebraska in a temp directory.
func runtimeRoot(path string) Option {
	return func(config *MutableConfig) error {
		config.Args = append(config.Args, argsRuntimeRoot, path)
		config.RuntimeRoot = path
		return nil
	}
}

// LogName renames Nebraska's log file. This can be helpful when you run Nebraska multiple times.
func LogName(name string) Option {
	return func(config *MutableConfig) error {
		config.LogFileName = name
		return nil
	}
}

// Port sets the port Nebraska is supposed to run on. Default is a random free port.
func Port(port int) Option {
	return func(config *MutableConfig) error {
		config.Args = append(config.Args, argsPort, strconv.Itoa(port))
		return nil
	}
}

// UpdateMetadata configures Nebraska to load update metadata from the given folder.
func UpdateMetadata(folder string) Option {
	return func(config *MutableConfig) error {
		config.Args = append(config.Args, argsUpdateMetadata, folder)
		return nil
	}
}

// UpdatePayloads configures Nebraska to expect update payloads at the given URL.
func UpdatePayloads(url string) Option {
	return func(config *MutableConfig) error {
		config.Args = append(config.Args, argsUpdatePayloadsAddress, url)
		return nil
	}
}

// InstallMetadata configures Nebraska to load install metadata at the given folder.
func InstallMetadata(folder string) Option {
	return func(config *MutableConfig) error {
		config.Args = append(config.Args, argsInstallMetadata, folder)
		return nil
	}
}

// InstallPayloads configures Nebraska to expect install payloads at the given URL.
func InstallPayloads(url string) Option {
	return func(config *MutableConfig) error {
		config.Args = append(config.Args, argsInstallPayloadsAddress, url)
		return nil
	}
}

// ConfigureUpdateEngine sets up update_engine service to ask this Nebraska instance for updates by modifying stateful LSB release file.
// This will result in stateful LSB release being cleared when nebraska stops.
func ConfigureUpdateEngine() Option {
	return func(config *MutableConfig) error {
		// Delay setup until after nebraska is up and we know the port it runs on.
		config.ConfigureUpdateEngine = func(port int) error {
			if err := ioutil.WriteFile(statefulLSBRelease, []byte(fmt.Sprintf("CHROMEOS_AUSERVER=%s", UpdateURL(port))), 0666); err != nil {
				return errors.Wrap(err, "failed to configure LSB release")
			}
			// TODO restart update engine?
			return nil
		}

		config.Cleanup = append(config.Cleanup,
			func(ctx context.Context) error {
				return os.Remove(statefulLSBRelease)
			})

		return nil
	}
}
