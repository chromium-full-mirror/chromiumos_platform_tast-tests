// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package util is to provide some common function or struct for launcher search.
package util

import (
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
)

// ImageSearchLocalPictureName is the image file name for image search.
const ImageSearchLocalPictureName = "search_local_image.png"

// ImageSearchRetryTimes is the times for image search if search failed.
const ImageSearchRetryTimes = 5

// ImageSearchTestParam is a image search related struct.
type ImageSearchTestParam struct {
	Name           string
	TabletMode     bool
	Query          []string
	ExpectedResult string
	UseIca         bool
	UseOcr         bool
}

// ImagesSelectionItem is the node of Image item on search category menu.
var ImagesSelectionItem *nodewith.Finder = nodewith.Name("Images").ClassName("CheckBoxMenuItemView").Role(role.MenuItemCheckBox)

// ImagesCategoryResultNode is the node of image category in search result category.
var ImagesCategoryResultNode *nodewith.Finder = nodewith.Role(role.ListBox).NameContaining("Images , search result category")

// ImageNode is node of image shown in the search result in launcher.
var ImageNode *nodewith.Finder = nodewith.Role(role.ListBoxOption).HasClass("SearchResultImageView").NameContaining("search_local_image")
