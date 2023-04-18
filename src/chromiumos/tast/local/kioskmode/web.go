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

	"chromiumos/tast/common/policy"
	"chromiumos/tast/testing"
)

const (
	// WebKioskAccountID identifier of the default WebKioskApp.
	WebKioskAccountID = "arbitrary_id_web_kiosk_1@managedchrome.com"
	// WebKioskTitle title shown on the splash screen of the kiosk app.
	WebKioskTitle = "Web Kiosk Placeholder Title"
	// WebKioskHeading heading shown on the main page of the kiosk web app.
	WebKioskHeading = "Test PWA for Web Kiosk"
)

// WebKioskAppAccountInfo creates a DeviceLocalAccountInfo for a mock WebKioskApp.
func WebKioskAppAccountInfo(url, accountID string) policy.DeviceLocalAccountInfo {
	iconURL := url + "/icon.png"
	accountType := policy.AccountTypeWebKioskApp
	webKioskTitle := WebKioskTitle
	return policy.DeviceLocalAccountInfo{
		AccountID:   &accountID,
		AccountType: &accountType,
		WebKioskAppInfo: &policy.WebKioskAppInfo{
			Url:     &url,
			Title:   &webKioskTitle,
			IconUrl: &iconURL,
		}}
}

// NewWebKioskAppServer creates a local http server for a mock WebKioskApp.
func NewWebKioskAppServer(ctx context.Context) *httptest.Server {
	httpServer := httptest.NewServer(http.HandlerFunc(webKioskServerHandler))
	testing.ContextLog(ctx, "Serving test PWA at "+httpServer.URL)
	return httpServer
}

// webKioskServerHandler handles http requests sent to test web Kiosk server.
func webKioskServerHandler(w http.ResponseWriter, r *http.Request) {
	const (
		contentHTMLFormat = `
<!DOCTYPE html>
<html>
    <head>
      <title id="title">Kiosk Test PWA page</title>
      <link rel="manifest" href="manifest.webmanifest">
      <link rel="icon" type="image/png" href="icon.png">
    </head>
    <body>
        <h1>%s</h1>
        <p>Path: %s</p>
    </body>
</html>
`
		manifestJs = `
{
  "description": "Kiosk Test PWA description",
  "display": "standalone",
  "icons":[{"sizes":"144x144","src":"/icon.png","type":"image/png"}],
  "id":"/",
  "name":"Web Kiosk Test PWA",
  "scope":"/",
  "short_name":"Kiosk Test",
  "start_url":"/start",
  "theme_color":"#000000"
}
`
	)
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Serve PWA manifest JSON.
	if strings.Contains(r.URL.Path, "manifest.webmanifest") {
		w.Header().Add("Content-Type", "application/manifest+json")
		w.Header().Set("Content-Length", strconv.Itoa(binary.Size(manifestJs)))
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, manifestJs)
		return
	}

	// Serve a blank PNG image as app icon.
	if strings.Contains(r.URL.Path, "icon.png") {
		var pngData bytes.Buffer
		pngWriter := bufio.NewWriter(&pngData)
		icon := image.NewRGBA(image.Rectangle{Min: image.Point{}, Max: image.Point{X: 144, Y: 144}})
		draw.Draw(icon, icon.Bounds(), &image.Uniform{C: color.Black}, image.Point{}, draw.Src)
		if err := png.Encode(pngWriter, icon); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Add("Content-Type", "image/png")
		w.Header().Add("Content-Disposition", "attachment; filename=icon.png")
		w.Header().Set("Content-Length", strconv.Itoa(pngData.Len()))
		w.WriteHeader(http.StatusOK)
		w.Write(pngData.Bytes())
		return
	}

	// Serve a html with path in body for all other paths.
	contentHTML := fmt.Sprintf(contentHTMLFormat, WebKioskHeading, r.URL.Path)
	w.Header().Add("Content-Type", "text/html")
	w.Header().Set("Content-Length", strconv.Itoa(binary.Size(contentHTML)))
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, contentHTML)
	return
}
