// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package extension

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"

	"go.chromium.org/tast/core/errors"
)

const chromeUser = "chronos" // Chrome Unix username

// ChownContentsToChrome recursively changes the ownership of the directory
// contents to the uid and gid of the Chrome's browser process.
func ChownContentsToChrome(dir string) error {
	return chownContents(dir, chromeUser)
}

// chownContents recursively chowns dir's contents to username's uid and gid.
func chownContents(dir, username string) error {
	var u *user.User
	var err error
	if u, err = user.Lookup(username); err != nil {
		return err
	}

	var uid, gid int64
	if uid, err = strconv.ParseInt(u.Uid, 10, 32); err != nil {
		return errors.Wrapf(err, "failed to parse uid %q", u.Uid)
	}
	if gid, err = strconv.ParseInt(u.Gid, 10, 32); err != nil {
		return errors.Wrapf(err, "failed to parse gid %q", u.Gid)
	}

	return filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Chown(p, int(uid), int(gid))
	})
}

// BackgroundPageURL returns the URL to the background page for the extension
// with the supplied ID.
func BackgroundPageURL(id string) string {
	return "chrome-extension://" + id + "/_generated_background_page.html"
}
