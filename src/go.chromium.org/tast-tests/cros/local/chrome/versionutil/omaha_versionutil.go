// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package versionutil provides utilities for the chrome and lacros versions
package versionutil

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"

	"go.chromium.org/tast-tests/cros/common/chrome/version"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// listReleasesResponse is imported from google.installer.versionhistory.v1.ListReleasesResponse
type listReleasesResponse struct {
	Releases      []Release `json:"releases"`
	NextPageToken string    `json:"nextPageToken"`
}

// Release is imported from google.installer.versionhistory.v1.Release
type Release struct {
	Name    string `json:"name"`
	Serving struct {
		StartTime string `json:"startTime"`
	} `json:"serving"`
	Fraction      int    `json:"fraction"`
	Version       string `json:"version"`
	FractionGroup string `json:"fractionGroup"`
}

// HTTPGetVersionHistory calls VersionHistory API to get the latest live versions of all channels for the given `platform`,
// returns a sorted list of `Release` by versions in ascending order.
// It guarantees a non-empty result on success.
func HTTPGetVersionHistory(ctx context.Context, platform string) ([]Release, error) {
	// See https://developer.chrome.com/docs/versionhistory/reference/ for the usage of the API.
	apiURL := fmt.Sprintf("https://versionhistory.googleapis.com/v1/chrome/platforms/%s/channels/all/versions/all/releases"+
		"?filter=endtime=none"+
		"&order_by=version%%20asc", platform) // in ascending order of version
	testing.ContextLog(ctx, "Calling VersionHistory: ", apiURL)
	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to call VersionHistory API")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("failed to call VersionHistory API, status: %s", resp.Status)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "error reading data")
	}

	releasesResp := listReleasesResponse{}
	if err := json.Unmarshal(body, &releasesResp); err != nil {
		return nil, errors.Wrapf(err, "failed to parse VersionHistory API response: %v", string(body))
	}
	if len(releasesResp.Releases) == 0 {
		return nil, errors.New("failed to find any releases in VersionHistory API")
	}
	return releasesResp.Releases, nil
}

// OmahaLacrosVersions returns a map of lacros channels to the latest live versions from Omaha that are compatible with the given Ash version.
func OmahaLacrosVersions(ctx context.Context, ashVersion version.Version, lacrosPlatform string) (map[string]version.Version, error) {
	// Call VersionHistory API to get the latest live versions of all channels.
	releases, err := HTTPGetVersionHistory(ctx, lacrosPlatform)
	if err != nil {
		return nil, errors.Wrap(err, "failed to call VersionHistory API")
	}

	// Parse the latest versions for each channel, and save the ones only compatible with Ash.
	channelToVersionMap := make(map[string]version.Version)
	var channelRE = regexp.MustCompile(fmt.Sprintf(`chrome/platforms/%s/channels/(\w+)`, lacrosPlatform))
	for _, release := range releases {
		match := channelRE.FindStringSubmatch(release.Name)
		if match == nil {
			return nil, errors.New("failed to find channel in release")
		}
		channel := match[1]

		switch channel {
		case "canary", "dev", "beta", "stable":
			lacrosVersion := version.Parse(release.Version)
			if err := version.IsSkewValid(lacrosVersion, ashVersion); err == nil {
				// Keep the only latest version per channel.
				// If the multiple versions exist it is okay to overwrite the version here since HTTPGetVersionHistory returns a sorted list of `Release` by versions in ascending order.
				channelToVersionMap[channel] = lacrosVersion
			} else {
				testing.ContextLogf(ctx, "Skipping incompatible channel: %v, reason: %v", channel, err)
			}
		default:
			testing.ContextLog(ctx, "Skipping unexpected channel: ", channel)
		}
	}
	return channelToVersionMap, nil
}
