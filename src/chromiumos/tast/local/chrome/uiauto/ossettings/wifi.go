// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ossettings

import (
	"context"

	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
)

// LaunchAtWiFi launch OS settings and navigate to WiFi settings page.
func LaunchAtWiFi(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome) (*OSSettings, error) {
	condition := uiauto.New(tconn).Exists(nodewith.Name("Wi-Fi subpage back button"))
	return LaunchAtPageURL(ctx, tconn, cr, "networks?type=WiFi", condition)
}
