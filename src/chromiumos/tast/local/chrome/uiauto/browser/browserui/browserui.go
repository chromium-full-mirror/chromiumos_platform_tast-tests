// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package browserui contains the general browser UI finders and functions.
package browserui

import (
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
)

// AddressBarFinder represents the address bar node finder.
var AddressBarFinder = nodewith.HasClass("OmniboxViewViews").Role(role.TextField)
