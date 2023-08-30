// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ime

// ChromeIMEPrefix is the prefix of IME chrome extension.
const ChromeIMEPrefix = "_comp_ime_jkghodnilhceideoidjikpgommlajknk"

// ChromiumIMEPrefix is the prefix of IME chromium extension.
const ChromiumIMEPrefix = "_comp_ime_fgoepimhcoialccpbmpnnblemnepkkao"

// Language represents the handwriting/voice language for an input method.
type Language string

// List of languages, names are defined based on ISO 639.
const (
	LanguageAr      Language = "Arabic"
	LanguageBg      Language = "Bulgarian"
	LanguageCa      Language = "Catalan"
	LanguageCs      Language = "Czech"
	LanguageDa      Language = "Danish"
	LanguageDe      Language = "German"
	LanguageEl      Language = "Greek"
	LanguageEn      Language = "English"
	LanguageEs      Language = "Spanish"
	LanguageFa      Language = "Persian"
	LanguageFi      Language = "Finnish"
	LanguageFr      Language = "French"
	LanguageGa      Language = "Irish"
	LanguageGu      Language = "Gujarati"
	LanguageHe      Language = "Hebrew"
	LanguageHi      Language = "Hindi"
	LanguageHr      Language = "Croatian"
	LanguageHu      Language = "Hungarian"
	LanguageIs      Language = "Icelandic"
	LanguageIt      Language = "Italian"
	LanguageJa      Language = "Japanese"
	LanguageKa      Language = "Georgian"
	LanguageKk      Language = "Kazakh"
	LanguageKm      Language = "Khmer"
	LanguageKn      Language = "Kannada"
	LanguageLv      Language = "Latvian"
	LanguageKo      Language = "Korean"
	LanguageMk      Language = "Macedonian"
	LanguageMl      Language = "Malayalam"
	LanguageMn      Language = "Mongolian"
	LanguageMr      Language = "Marathi"
	LanguageMt      Language = "Maltese"
	LanguageMy      Language = "Myanmar"
	LanguageNe      Language = "Nepali"
	LanguageNl      Language = "Dutch (Netherlands)"
	LanguageNo      Language = "Norwegian"
	LanguageOr      Language = "Odia"
	LanguagePa      Language = "Punjabi"
	LanguagePl      Language = "Polish"
	LanguagePt      Language = "Portuguese"
	LanguageRo      Language = "Romanian"
	LanguageRu      Language = "Russian"
	LanguageSa      Language = "Sanskrit"
	LanguageSi      Language = "Sinhala"
	LanguageSk      Language = "Slovak"
	LanguageSl      Language = "Slovenian"
	LanguageSr      Language = "Serbian"
	LanguageSv      Language = "Swedish"
	LanguageTa      Language = "Tamil"
	LanguageTe      Language = "Telugu"
	LanguageTh      Language = "Thai"
	LanguageTr      Language = "Turkish"
	LanguageUr      Language = "Urdu"
	LanguageVi      Language = "Vietnamese"
	LanguageYueHant Language = "Traditional Cantonese"
	LanguageZhHans  Language = "Simplified Chinese"
	LanguageZhHant  Language = "Traditional Chinese"
)
