// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kioskmode

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast/core/testing"
)

// SimpleWebApp describes a minimally configurable web app.
// Used to serve the app in an HTTP server, and to create policy.DeviceLocalAccounts for Kiosk
// tests.
type SimpleWebApp struct {
	Name        string
	Description string
	IconColor   color.Color
	HTMLBody    string
}

type cleanupFunc func()

const (
	// DefaultKioskWebAppTitle is the name shown under the apps button on the login screen.
	DefaultKioskWebAppTitle = "Kiosk Test PWA Title"
	// DefaultKioskWebAppHeading is a heading shown on the web app.
	DefaultKioskWebAppHeading = "Test PWA for Kiosk"
)

// DefaultWebApp returns a default SimpleWebApp.
func DefaultWebApp() SimpleWebApp {
	return SimpleWebApp{
		Name:        DefaultKioskWebAppTitle,
		Description: "Kiosk Test PWA description",
		IconColor:   color.Black,
		HTMLBody:    "<h1>" + DefaultKioskWebAppHeading + "</h1>",
	}
}

// NewServerAndAccount returns a server for this app and a policy.DeviceLocalAccounts containing one
// single account configured for it.
func (app SimpleWebApp) NewServerAndAccount(ctx context.Context) (string, policy.DeviceLocalAccounts, cleanupFunc) {
	url, cleanup := app.NewServer(ctx)
	account := app.NewDeviceLocalAccountInfo(url, WebKioskAccountID)
	accounts := policy.DeviceLocalAccounts{Val: []policy.DeviceLocalAccountInfo{account}}
	return url, accounts, cleanup
}

// NewServer starts and returns an HTTP server for this app.
func (app SimpleWebApp) NewServer(ctx context.Context) (string, cleanupFunc) {
	httpServer := httptest.NewServer(app.httpHandler())
	testing.ContextLog(ctx, "Serving test PWA at "+httpServer.URL)
	return httpServer.URL, func() { httpServer.Close() }
}

// NewDeviceLocalAccountInfo creates a DeviceLocalAccountInfo for this app.
//
// Meant to be used with the URL of an HTTP server created by NewServer.
//
// By default FakeDMS requires accountID to be an email@managedchrome.com. See:
// https://crsrc.org/c/components/policy/test_support/request_handler_for_policy.cc;l=236. In
// production an AccountID can be any string, e.g. URLs in Kiosk web apps.
func (app SimpleWebApp) NewDeviceLocalAccountInfo(url, accountID string) policy.DeviceLocalAccountInfo {
	iconURL := url + "/icon.png"
	accountType := policy.AccountTypeWebKioskApp
	return policy.DeviceLocalAccountInfo{
		AccountID:   &accountID,
		AccountType: &accountType,
		WebKioskAppInfo: &policy.WebKioskAppInfo{
			Url:     &url,
			Title:   &app.Name,
			IconUrl: &iconURL,
		},
	}
}

func (app SimpleWebApp) httpHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
		} else if strings.Contains(r.URL.Path, "manifest.webmanifest") {
			serveString(w, "application/manifest+json", app.renderManifest())
		} else if strings.Contains(r.URL.Path, "icon.png") {
			serveIcon(w, app.IconColor)
		} else {
			serveString(w, "text/html", app.renderHTML())
		}
	}
}

func (app SimpleWebApp) renderManifest() string {
	manifestName := app.Name
	if manifestName == "" {
		manifestName = "Simple Web App"
	}
	manifestDescription := app.Description
	if manifestDescription == "" {
		manifestDescription = "Simple web app used in Tast tests"
	}
	const manifestTemplate = `
{
  "name":"%s",
  "description": "%s",
  "display": "standalone",
  "icons":[{"sizes":"144x144","src":"/icon.png","type":"image/png"}],
  "id":"/",
  "scope":"/",
  "short_name":"Kiosk Test",
  "start_url":"/start",
  "theme_color":"#000000"
}
`
	return fmt.Sprintf(manifestTemplate, manifestName, manifestDescription)
}

func (app SimpleWebApp) renderHTML() string {
	body := app.HTMLBody
	if !strings.HasPrefix(strings.TrimSpace(body), "<body") {
		body = "<body>" + body + "</body>"
	}
	const htmlTemplate = `
<!DOCTYPE html>
<html>
    <head>
      <title id="title">%s</title>
      <link rel="manifest" href="manifest.webmanifest">
      <link rel="icon" type="image/png" href="icon.png">
    </head>
    %s
</html>
`
	return fmt.Sprintf(htmlTemplate, app.Name, body)
}

func serveString(w http.ResponseWriter, contentType, content string) {
	w.Header().Add("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(binary.Size(content)))
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, content)
}

func serveIcon(w http.ResponseWriter, iconColor color.Color) {
	var pngData bytes.Buffer
	pngWriter := bufio.NewWriter(&pngData)
	icon := image.NewRGBA(image.Rectangle{Min: image.Point{}, Max: image.Point{X: 144, Y: 144}})
	draw.Draw(icon, icon.Bounds(), &image.Uniform{C: iconColor}, image.Point{}, draw.Src)
	if err := png.Encode(pngWriter, icon); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-Type", "image/png")
	w.Header().Add("Content-Disposition", "attachment; filename=icon.png")
	w.Header().Set("Content-Length", strconv.Itoa(pngData.Len()))
	w.WriteHeader(http.StatusOK)
	w.Write(pngData.Bytes())
}
