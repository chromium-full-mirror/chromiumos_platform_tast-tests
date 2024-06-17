// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package utils

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"

	"go.chromium.org/tast/core/errors"
)

// FetchFromURL fetches content from a specific URL.
func FetchFromURL(ctx context.Context, url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", errors.Wrapf(err, "failed to send request %q", url)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", errors.Errorf("failed with status %v", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(err, "failed to read response body")
	}
	return string(body), nil
}

// CompileHTMLDataURL returns a base64 encoded HTML data url string.
func CompileHTMLDataURL(context []byte) string {
	// url.QueryEscape() is not the use case because it can't escape some characters.
	return "data:text/html;base64," + base64.StdEncoding.EncodeToString(context)
}
