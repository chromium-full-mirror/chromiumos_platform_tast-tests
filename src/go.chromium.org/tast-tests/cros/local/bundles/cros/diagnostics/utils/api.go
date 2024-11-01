// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package utils

import (
	"context"
	// Used to embed api_wrapper.js in string variable `systemDataProviderJs`.
	_ "embed"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/errors"
)

// systemDataProviderJs is a stringified JS file that exposes the SystemDataProvider mojo
// API.
//
//go:embed api_wrapper.js
var systemDataProviderJs string

// MojoAPI is a struct that encapsulates a SystemDataProvider mojo remote.
type MojoAPI struct {
	conn       *chrome.Conn
	mojoRemote *chrome.JSObject
}

type systemInfo struct {
	BoardName           string `json:"boardName"`
	MarketingName       string `json:"marketingName"`
	CPUModelName        string `json:"cpuModelName"`
	TotalMemoryKib      uint32 `json:"totalMemoryKib"`
	CPUThreadsCount     uint16 `json:"cpuThreadsCount"`
	CPUMaxClockSpeedKhz uint32 `json:"cpuMaxClockSpeedKhz"`
}

// SystemDataProviderMojoAPI returns a MojoAPI object that is connected to a SystemDataProvider
// mojo remote instance on success, or an error.
func SystemDataProviderMojoAPI(ctx context.Context, cr *chrome.Chrome) (*MojoAPI, error) {
	conn, err := cr.NewConnForTarget(ctx, chrome.MatchTargetURL(appURL))
	if err != nil {
		return nil, errors.Wrap(err, "failed to match the diagnostics chrome connection")
	}

	var mojoRemote chrome.JSObject
	if err := conn.Call(ctx, &mojoRemote, systemDataProviderJs); err != nil {
		return nil, errors.Wrap(err, "failed to set up the SystemDataProvider mojo API")
	}

	return &MojoAPI{conn, &mojoRemote}, nil
}

// RunFetchSystemInfo calls into the injected SystemDataProvider mojo API.
func (m *MojoAPI) RunFetchSystemInfo(ctx context.Context) error {
	jsWrap := "function() { return this.fetchSystemInfo() }"
	var result systemInfo
	if err := m.mojoRemote.Call(ctx, &result, jsWrap); err != nil {
		return errors.Wrap(err, "failed to run fetchSystemInfo")
	}

	if result.BoardName == "" || result.MarketingName == "" ||
		result.CPUModelName == "" {
		return errors.New("failed to get valid system info")
	}

	return nil
}

// Release frees the resources help by the internal MojoAPI components.
func (m *MojoAPI) Release(ctx context.Context) error {
	if err := m.conn.Close(); err != nil {
		return errors.Wrap(err, "failed to close connection")
	}

	if err := m.mojoRemote.Release(ctx); err != nil {
		return errors.Wrap(err, "failed to release mojo remote")
	}

	return nil
}
