// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package citrix

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/uidetection"
)

const (
	// ScriptelAppName is the name of the Scriptel application.
	ScriptelAppName AppName = "Scrip"

	scriptelTransparentIcon = "citrix/scriptel_transparent.png"
	scriptelClearIcon       = "citrix/scriptel_clear.png"
)

// ScriptelSignaturePad implements the SignaturePad interface for Scriptel.
type ScriptelSignaturePad struct {
	ud       *uidetection.Context
	kb       *input.KeyboardEventWriter
	tconn    *chrome.TestConn
	dataPath func(string) string
}

// NewScriptelSignaturePad creates a new ScriptelSignaturePad instance.
func NewScriptelSignaturePad(ud *uidetection.Context, kb *input.KeyboardEventWriter, tconn *chrome.TestConn, dataPath func(string) string) *ScriptelSignaturePad {
	return &ScriptelSignaturePad{ud: ud, kb: kb, tconn: tconn, dataPath: dataPath}
}

// GetCanvasBounds returns the Scriptel canvas bounds.
func (s *ScriptelSignaturePad) GetCanvasBounds(ctx context.Context) (coords.Rect, error) {
	scriptelClearCanvas := uidetection.CustomIcon(s.dataPath(scriptelClearIcon))
	return getSignaturePadCanvasBounds(ctx, s.ud, s.tconn, s.dataPath, scriptelClearCanvas)
}

// StartSignature starts Scriptel signature.
func (s *ScriptelSignaturePad) StartSignature() action.Action {
	ud := s.ud
	scriptelTransparent := uidetection.CustomIcon(s.dataPath(scriptelTransparentIcon))
	closeTouchSigningText := uidetection.TextBlockFromSentence("Close Touch Signing").First()
	skipThisVersionText := uidetection.TextBlockFromSentence("Skip this version").First()
	connectedText := uidetection.TextBlockFromSentence("Connected to ScripTouch").First()
	return uiauto.NamedCombine("start Scriptel signature",
		uiauto.IfSuccessThen(ud.Exists(skipThisVersionText),
			ud.LeftClickUntil(skipThisVersionText, ud.Gone(skipThisVersionText))),
		uiauto.IfSuccessThen(ud.Exists(closeTouchSigningText),
			ud.LeftClickUntil(closeTouchSigningText, ud.Gone(closeTouchSigningText))),
		uiauto.IfSuccessThen(ud.Exists(scriptelTransparent),
			ud.LeftClickUntil(scriptelTransparent, ud.Gone(scriptelTransparent))),
		ud.WaitUntilExists(connectedText),
	)
}

// SaveSignature saves Scriptel signature.
func (s *ScriptelSignaturePad) SaveSignature(fileName string) uiauto.Action {
	ud := s.ud
	acceptText := uidetection.Word("Accept").First()
	recentItemsText := uidetection.TextBlockFromSentence("Recent Items").First()
	desktopText := uidetection.Word("Desktop").Below(recentItemsText).First()
	saveText := uidetection.Word("Save").RightOf(desktopText).First()
	return uiauto.NamedCombine("save signature",
		ud.LeftClickUntil(acceptText, ud.Exists(desktopText)),
		s.kb.TypeAction(fileName),
		ud.LeftClick(desktopText),
		ud.LeftClickUntil(saveText, ud.Gone(saveText)),
	)
}

// ClearSignature clears Scriptel signature.
func (s *ScriptelSignaturePad) ClearSignature() uiauto.Action {
	ud := s.ud
	scriptelClearCanvas := uidetection.CustomIcon(s.dataPath(scriptelClearIcon))
	clearText := uidetection.Word("Clear").First()
	return uiauto.NamedAction("clear signature",
		ud.LeftClickUntil(clearText, ud.Exists(scriptelClearCanvas)),
	)
}

// LoadSignature loads Scriptel signature.
func (s *ScriptelSignaturePad) LoadSignature(fileName string) uiauto.Action {
	fileNameText := uidetection.Word(fileName).First()
	return uiauto.NamedAction("load signature",
		s.ud.DoubleClick(fileNameText),
	)
}

var _ SignaturePad = (*ScriptelSignaturePad)(nil)
