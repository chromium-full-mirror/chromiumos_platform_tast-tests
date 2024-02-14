// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testenv

import (
	"io"
	"os"
	"regexp"
	"strings"

	"go.chromium.org/tast/core/errors"
)

// replaceFile moves a old (hosts) file to the existing path.
// Note that os.Rename can't be used if two paths are on a different filesystem.
// For example, it may fail if the old file under /tmp is renamed to the path at /etc/hosts.
func replaceFile(src, dst string) error {
	fsrc, err := os.Open(src)
	if err != nil {
		return errors.Wrapf(err, "failed to open the source file: %v", src)
	}
	defer os.Remove(src)
	defer fsrc.Close()
	fdst, err := os.Create(dst)
	if err != nil {
		return errors.Wrapf(err, "failed to open the destination file: %v", dst)
	}
	defer fdst.Close()
	_, err = io.Copy(fdst, fsrc)
	return err
}

// isValidHostname checks that hostname is RFC 1123 compatible.
// For example, "foo.bar.baz" is valid in that it starts and ends with a letter without using any disallowed character.
func isValidHostname(hostname string) bool {
	re := regexp.MustCompile(`^(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])\.)+[a-zA-Z]{2,}$`)
	return re.MatchString(hostname)
}

// isValidAlias checks that alias is suffixed with the valid env name.
func isValidAlias(alias string) bool {
	if alias == "" {
		return false
	}
	for _, e := range ValidEnvs {
		if strings.HasSuffix(alias, "-"+e) {
			return true
		}
	}
	return false
}

// isDUT checks if it runs in a DUT or not.
// TODO(b/310046249): Reliable way to detect it. It would be useful to gate code that should or should not be run in a DUT.
func isDUT() bool {
	lsb, err := os.ReadFile("/etc/lsb-release")
	return err == nil && strings.Contains(string(lsb), "CHROMEOS_RELEASE_BOARD")
}
