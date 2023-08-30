// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ime

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast/core/errors"
)

// TODO(b/192819861): Define new input method struct and migrate existing use of InputMethodCode.
// This page is a partial implementation of b/192819861.
// This structure might need to be refined.

// ID is the unique identifier of an input method
// from http://osscs/chromium/chromium/src/+/main:chrome/browser/resources/chromeos/input_method/google_xkb_manifest.json.
// Name is likely to change due to CrOS Cross-border improvement.

// InputMethod represents an input method.
type InputMethod struct {
	Name                string   // The displayed name of the IME in OS Settings.
	ID                  string   // The code / id of the IME, e.g. "xkb:us::eng".
	ShortLabel          string   // The short label displayed in VK language menu & System IME tray to represent the input method.
	HandwritingLanguage Language // The language for handwriting.
	VoiceLanguage       Language // The language for voice dictation.
}

// DefaultInputMethod is the default input method enabled for new users.
var DefaultInputMethod = EnglishUS

// EnglishUS represents the input method of English (US).
var EnglishUS = InputMethod{
	Name:                "English (US)",
	ID:                  "xkb:us::eng",
	ShortLabel:          "US",
	HandwritingLanguage: LanguageEn,
	VoiceLanguage:       LanguageEn,
}

// EnglishUSWithInternationalKeyboard represents the input method of English (US) with International keyboard.
var EnglishUSWithInternationalKeyboard = InputMethod{
	Name:                "English (US) with International keyboard",
	ID:                  "xkb:us:intl:eng",
	ShortLabel:          "INTL",
	HandwritingLanguage: LanguageEn,
	VoiceLanguage:       LanguageEn,
}

// EnglishUSWithExtendedKeyboard represents the input method of English (US) with Extended keyboard.
var EnglishUSWithExtendedKeyboard = InputMethod{
	Name:                "English (US) with Extended keyboard",
	ID:                  "xkb:us:altgr-intl:eng",
	ShortLabel:          "INTL",
	HandwritingLanguage: LanguageEn,
	VoiceLanguage:       LanguageEn,
}

// EnglishUSWithDvorakKeyboard represents the input method of English (US) with Dvorak keyboard.
var EnglishUSWithDvorakKeyboard = InputMethod{
	Name:                "English (US) with Dvorak keyboard",
	ID:                  "xkb:us:dvorak:eng",
	ShortLabel:          "DV",
	HandwritingLanguage: LanguageEn,
	VoiceLanguage:       LanguageEn,
}

// EnglishUSWithColemakKeyboard represents the input method of English (US) with Colemak keyboard.
var EnglishUSWithColemakKeyboard = InputMethod{
	Name:                "English (US) with Colemak keyboard",
	ID:                  "xkb:us:colemak:eng",
	ShortLabel:          "CO",
	HandwritingLanguage: LanguageEn,
	VoiceLanguage:       LanguageEn,
}

// EnglishUSWithWorkmanKeyboard represents the input method of English (US) with Workman keyboard.
var EnglishUSWithWorkmanKeyboard = InputMethod{
	Name:                "English (US) with Workman keyboard",
	ID:                  "xkb:us:workman:eng",
	ShortLabel:          "WM",
	HandwritingLanguage: LanguageEn,
	VoiceLanguage:       LanguageEn,
}

// EnglishUK represents the input method of English (UK).
var EnglishUK = InputMethod{
	Name:                "English (UK)",
	ID:                  "xkb:gb:extd:eng",
	ShortLabel:          "GB",
	HandwritingLanguage: LanguageEn,
	VoiceLanguage:       LanguageEn,
}

// EnglishCanada represents the input method of English (Canada).
var EnglishCanada = InputMethod{
	Name:                "English (Canada)",
	ID:                  "xkb:ca:eng:eng",
	ShortLabel:          "CA",
	HandwritingLanguage: LanguageEn,
	VoiceLanguage:       LanguageEn,
}

// EnglishSouthAfrica represents the input method of English (South Africa).
var EnglishSouthAfrica = InputMethod{
	Name:                "English (South Africa)",
	ID:                  "xkb:za:gb:eng",
	ShortLabel:          "ZA",
	HandwritingLanguage: LanguageEn,
	VoiceLanguage:       LanguageEn,
}

// EnglishPakistan represents the input method of English (Pakistan).
var EnglishPakistan = InputMethod{
	Name:                "English (Pakistan)",
	ID:                  "xkb:pk::eng",
	ShortLabel:          "PK",
	HandwritingLanguage: LanguageEn,
	VoiceLanguage:       LanguageEn,
}

// EnglishIndia represents the input method of English (India).
var EnglishIndia = InputMethod{
	Name:                "English (India)",
	ID:                  "xkb:in::eng",
	ShortLabel:          "IN",
	HandwritingLanguage: LanguageEn,
	VoiceLanguage:       LanguageEn,
}

// AlphanumericWithJapaneseKeyboard represents the input method of Alphanumeric with Japanese keyboard.
var AlphanumericWithJapaneseKeyboard = InputMethod{
	Name:                "Alphanumeric with Japanese keyboard",
	ID:                  "xkb:jp::jpn",
	ShortLabel:          "JA",
	HandwritingLanguage: LanguageJa,
	VoiceLanguage:       LanguageJa,
}

// Arabic represents the input method of Arabic.
var Arabic = InputMethod{
	Name:                "Arabic",
	ID:                  "vkd_ar",
	ShortLabel:          "AR",
	HandwritingLanguage: LanguageAr,
	VoiceLanguage:       LanguageAr,
}

// Bulgarian represents the input method of Bulgarian.
var Bulgarian = InputMethod{
	Name:                "Bulgarian",
	ID:                  "xkb:bg::bul",
	ShortLabel:          "BG",
	HandwritingLanguage: LanguageBg,
	VoiceLanguage:       LanguageBg,
}

// Cantonese represents the input method of Chinese Cantonese.
var Cantonese = InputMethod{
	Name:                "Cantonese",
	ID:                  "yue-hant-t-i0-und",
	ShortLabel:          "粤",
	HandwritingLanguage: LanguageZhHant,
	VoiceLanguage:       LanguageYueHant,
}

// Catalan represents the input method of Catalan.
var Catalan = InputMethod{
	Name:                "Catalan",
	ID:                  "xkb:es:cat:cat",
	HandwritingLanguage: LanguageCa,
	VoiceLanguage:       LanguageCa,
}

// ChineseArray represents the input method of Chinese Array.
var ChineseArray = InputMethod{
	Name:                "Chinese Array",
	ID:                  "zh-hant-t-i0-array-1992",
	ShortLabel:          "行列",
	HandwritingLanguage: LanguageZhHant,
	VoiceLanguage:       LanguageZhHant,
}

// ChineseDayi represents the input method of Chinese Dayi.
var ChineseDayi = InputMethod{
	Name:                "Chinese Dayi",
	ID:                  "zh-hant-t-i0-dayi-1988",
	ShortLabel:          "大易",
	HandwritingLanguage: LanguageZhHant,
	VoiceLanguage:       LanguageZhHant,
}

// ChinesePinyin represents the input method of Chinese Pinyin.
var ChinesePinyin = InputMethod{
	Name:                "Chinese Pinyin",
	ID:                  "zh-t-i0-pinyin",
	ShortLabel:          "拼",
	HandwritingLanguage: LanguageZhHans,
	VoiceLanguage:       LanguageZhHans,
}

// ChineseTraditionalPinyin represents the input method of Chinese (Traditional) Pinyin.
var ChineseTraditionalPinyin = InputMethod{
	Name:                "Chinese Traditional Pinyin",
	ID:                  "zh-hant-t-i0-pinyin",
	ShortLabel:          "拼",
	HandwritingLanguage: LanguageZhHant,
	VoiceLanguage:       LanguageZhHant,
}

// ChineseQuick represents the input method of Chinese Quick.
var ChineseQuick = InputMethod{
	Name:                "Chinese Quick",
	ID:                  "zh-hant-t-i0-cangjie-1987-x-m0-simplified",
	ShortLabel:          "速",
	HandwritingLanguage: LanguageZhHant,
	VoiceLanguage:       LanguageZhHant,
}

// ChineseWubi represents the input method of Chinese Wubi.
var ChineseWubi = InputMethod{
	Name:                "Chinese Wubi",
	ID:                  "zh-t-i0-wubi-1986",
	ShortLabel:          "五",
	HandwritingLanguage: LanguageZhHans,
	VoiceLanguage:       LanguageZhHans,
}

// ChineseZhuyin represents the input method of Chinese Zhuyin.
var ChineseZhuyin = InputMethod{
	Name:                "Chinese Zhuyin",
	ID:                  "zh-hant-t-i0-und",
	ShortLabel:          "注",
	HandwritingLanguage: LanguageZhHant,
	VoiceLanguage:       LanguageZhHant,
}

// ChineseCangjie represents the input method of Chinese Cangjie.
var ChineseCangjie = InputMethod{
	Name:                "Chinese Cangjie",
	ID:                  "zh-hant-t-i0-cangjie-1987",
	ShortLabel:          "倉",
	HandwritingLanguage: LanguageZhHant,
	VoiceLanguage:       LanguageZhHant,
}

// Croatian represents the input method of Croatian.
var Croatian = InputMethod{
	Name:                "Croatian",
	ID:                  "xkb:hr::scr",
	HandwritingLanguage: LanguageHr,
	VoiceLanguage:       LanguageHr,
}

// Czech represents the input method of Czech.
var Czech = InputMethod{
	Name:                "Czech",
	ID:                  "xkb:cz::cze",
	HandwritingLanguage: LanguageCs,
	VoiceLanguage:       LanguageCs,
}

// Danish represents the input method of Danish.
var Danish = InputMethod{
	Name:                "Danish",
	ID:                  "xkb:dk::dan",
	HandwritingLanguage: LanguageDa,
	VoiceLanguage:       LanguageDa,
}

// DutchNetherlands represents the input method of Dutch (Netherlands).
var DutchNetherlands = InputMethod{
	Name:                "Dutch (Netherlands)",
	ID:                  "xkb:us:intl:nld",
	HandwritingLanguage: LanguageNl,
	VoiceLanguage:       LanguageNl,
}

// Finnish represents the input method of Finnish.
var Finnish = InputMethod{
	Name:                "Finnish",
	ID:                  "xkb:fi::fin",
	HandwritingLanguage: LanguageFi,
	VoiceLanguage:       LanguageFi,
}

// FrenchFrance represents the input method of French (France).
var FrenchFrance = InputMethod{
	Name:                "French (France)",
	ID:                  "xkb:fr::fra",
	ShortLabel:          "FR",
	HandwritingLanguage: LanguageFr,
	VoiceLanguage:       LanguageFr,
}

// Georgian represents the input method of Georgian.
var Georgian = InputMethod{
	Name:                "Georgian",
	ID:                  "xkb:ge::geo",
	HandwritingLanguage: LanguageKa,
	VoiceLanguage:       LanguageKa,
}

// German represents the input method of German (Germany).
var German = InputMethod{
	Name:                "German",
	ID:                  "xkb:de::ger",
	HandwritingLanguage: LanguageDe,
	VoiceLanguage:       LanguageDe,
}

// GreekTransliteration represents the input method of Greek Transliteration.
var GreekTransliteration = InputMethod{
	Name:                "GreekTransliteration",
	ID:                  "el-t-i0-und",
	HandwritingLanguage: LanguageEl,
	VoiceLanguage:       LanguageEl,
}

// Gujarati represents the input method of Gujarati.
var Gujarati = InputMethod{
	Name:                "Gujarati",
	ID:                  "gu-t-i0-und",
	HandwritingLanguage: LanguageGu,
	VoiceLanguage:       LanguageGu,
}

// Hebrew represents the input method of Hebrew.
var Hebrew = InputMethod{
	Name:                "Hebrew",
	ID:                  "xkb:il::heb",
	HandwritingLanguage: LanguageHe,
	VoiceLanguage:       LanguageHe,
}

// Hindi represents the input method of Hindi.
var Hindi = InputMethod{
	Name:                "Hindi",
	ID:                  "hi-t-i0-und",
	HandwritingLanguage: LanguageHi,
	VoiceLanguage:       LanguageHi,
}

// Hungarian represents the input method of Hungarian.
var Hungarian = InputMethod{
	Name:                "Hungarian",
	ID:                  "xkb:hu::hun",
	HandwritingLanguage: LanguageHu,
	VoiceLanguage:       LanguageHu,
}

// Icelandic represents the input method of Icelandic.
var Icelandic = InputMethod{
	Name:                "Icelandic",
	ID:                  "xkb:is::ice",
	HandwritingLanguage: LanguageIs,
	VoiceLanguage:       LanguageIs,
}

// Irish represents the input method of Irish.
var Irish = InputMethod{
	Name:                "Irish",
	ID:                  "xkb:ie::ga",
	HandwritingLanguage: LanguageGa,
	VoiceLanguage:       LanguageGa,
}

// Italian represents the input method of Italian.
var Italian = InputMethod{
	Name:                "Italian",
	ID:                  "xkb:it::ita",
	HandwritingLanguage: LanguageIt,
	VoiceLanguage:       LanguageIt,
}

// Japanese represents the input method of Japanese.
var Japanese = InputMethod{
	Name:                "Japanese",
	ID:                  "nacl_mozc_jp",
	ShortLabel:          "あ",
	HandwritingLanguage: LanguageJa,
	VoiceLanguage:       LanguageJa,
}

// JapaneseWithUSKeyboard represents the input method of Japanese with US keyboard.
var JapaneseWithUSKeyboard = InputMethod{
	Name:                "Japanese with US keyboard",
	ID:                  "nacl_mozc_us",
	ShortLabel:          "あ",
	HandwritingLanguage: LanguageJa,
	VoiceLanguage:       LanguageJa,
}

// Kannada represents the input method of Kannada.
var Kannada = InputMethod{
	Name:                "Kannada",
	ID:                  "kn-t-i0-und",
	HandwritingLanguage: LanguageKn,
	VoiceLanguage:       LanguageKn,
}

// Kazakh represents the input method of Kazakh.
var Kazakh = InputMethod{
	Name:                "Kazakh",
	ID:                  "xkb:kz::kaz",
	HandwritingLanguage: LanguageKk,
	VoiceLanguage:       LanguageKk,
}

// Khmer represents the input method of Khmer.
var Khmer = InputMethod{
	Name:                "Khmer",
	ID:                  "vkd_km",
	ShortLabel:          "KM",
	HandwritingLanguage: LanguageKm,
	VoiceLanguage:       LanguageKm,
}

// Korean represents the input method of Korean.
var Korean = InputMethod{
	Name:                "Korean",
	ID:                  "ko-t-i0-und",
	ShortLabel:          "한",
	HandwritingLanguage: LanguageKo,
	VoiceLanguage:       LanguageKo,
}

// Latvian represents the input method of Latvian.
var Latvian = InputMethod{
	Name:                "Latvian",
	ID:                  "xkb:lv:apostrophe:lav",
	HandwritingLanguage: LanguageLv,
	VoiceLanguage:       LanguageLv,
}

// Macedonian represents the input method of Macedonian.
var Macedonian = InputMethod{
	Name:                "Macedonian",
	ID:                  "xkb:mk::mkd",
	HandwritingLanguage: LanguageMk,
	VoiceLanguage:       LanguageMk,
}

// Malayalam represents the input method of Malayalam.
var Malayalam = InputMethod{
	Name:                "Malayalam",
	ID:                  "ml-t-i0-und",
	HandwritingLanguage: LanguageMl,
	VoiceLanguage:       LanguageMl,
}

// Maltese represents the input method of Maltese.
var Maltese = InputMethod{
	Name:                "Maltese",
	ID:                  "xkb:mt::mlt",
	HandwritingLanguage: LanguageMt,
	VoiceLanguage:       LanguageMt,
}

// Marathi represents the input method of Marathi.
var Marathi = InputMethod{
	Name:                "Marathi",
	ID:                  "mr-t-i0-und",
	HandwritingLanguage: LanguageMr,
	VoiceLanguage:       LanguageMr,
}

// Mongolian represents the input method of Mongolian.
var Mongolian = InputMethod{
	Name:                "Mongolian",
	ID:                  "xkb:mn::mon",
	HandwritingLanguage: LanguageMn,
	VoiceLanguage:       LanguageMn,
}

// Myanmar represents the input method of Myanmar.
var Myanmar = InputMethod{
	Name:                "Myanmar",
	ID:                  "vkd_my",
	ShortLabel:          "MY",
	HandwritingLanguage: LanguageMy,
	VoiceLanguage:       LanguageMy,
}

// NepaliTransliteration represents the input method of Nepali transliteration.
var NepaliTransliteration = InputMethod{
	Name:                "NepaliTransliteration",
	ID:                  "ne-t-i0-und",
	HandwritingLanguage: LanguageNe,
	VoiceLanguage:       LanguageNe,
}

// Norwegian represents the input method of Norwegian.
var Norwegian = InputMethod{
	Name:                "Norwegian",
	ID:                  "xkb:no::nob",
	HandwritingLanguage: LanguageNo,
	VoiceLanguage:       LanguageNo,
}

// Odia represents the input method of Odia.
var Odia = InputMethod{
	Name:                "Odia",
	ID:                  "or-t-i0-und",
	HandwritingLanguage: LanguageOr,
	VoiceLanguage:       LanguageOr,
}

// PersianTransliteration represents the input method of Persian transliteration.
var PersianTransliteration = InputMethod{
	Name:                "PersianTransliteration",
	ID:                  "fa-t-i0-und",
	HandwritingLanguage: LanguageFa,
	VoiceLanguage:       LanguageFa,
}

// Polish represents the input method of Polish.
var Polish = InputMethod{
	Name:                "Polish",
	ID:                  "xkb:pl::pol",
	HandwritingLanguage: LanguagePl,
	VoiceLanguage:       LanguagePl,
}

// Portuguese represents the input method of Portuguese.
var Portuguese = InputMethod{
	Name:                "Portuguese",
	ID:                  "xkb:pt::por",
	HandwritingLanguage: LanguagePt,
	VoiceLanguage:       LanguagePt,
}

// Punjabi represents the input method of Punjabi.
var Punjabi = InputMethod{
	Name:                "Punjabi",
	ID:                  "pa-t-i0-und",
	HandwritingLanguage: LanguagePa,
	VoiceLanguage:       LanguagePa,
}

// Romanian represents the input method of Romanian.
var Romanian = InputMethod{
	Name:                "Romanian",
	ID:                  "xkb:ro::rum",
	HandwritingLanguage: LanguageRo,
	VoiceLanguage:       LanguageRo,
}

// Russian represents the input method of Russian.
var Russian = InputMethod{
	Name:                "Russian",
	ID:                  "xkb:ru::rus",
	HandwritingLanguage: LanguageRu,
	VoiceLanguage:       LanguageRu,
}

// Sanskrit represents the input method of Sanskrit.
var Sanskrit = InputMethod{
	Name:                "Sanskrit",
	ID:                  "sa-t-i0-und",
	HandwritingLanguage: LanguageSa,
	VoiceLanguage:       LanguageSa,
}

// Serbian represents the input method of Serbian.
var Serbian = InputMethod{
	Name:                "Serbian",
	ID:                  "xkb:rs::srp",
	HandwritingLanguage: LanguageSr,
	VoiceLanguage:       LanguageSr,
}

// Sinhala represents the input method of Sinhala.
var Sinhala = InputMethod{
	Name:                "Sinhala",
	ID:                  "vkd_si",
	ShortLabel:          "SI",
	HandwritingLanguage: LanguageSi,
	VoiceLanguage:       LanguageSi,
}

// Slovak represents the input method of Slovak.
var Slovak = InputMethod{
	Name:                "Slovak",
	ID:                  "xkb:sk::slo",
	HandwritingLanguage: LanguageSk,
	VoiceLanguage:       LanguageSk,
}

// Slovenian represents the input method of Slovenian.
var Slovenian = InputMethod{
	Name:                "Slovenian",
	ID:                  "xkb:si::slv",
	HandwritingLanguage: LanguageSl,
	VoiceLanguage:       LanguageSl,
}

// SpanishSpain represents the input method of Spanish (Spain).
var SpanishSpain = InputMethod{
	Name:                "Spanish (Spain)",
	ID:                  "xkb:es::spa",
	ShortLabel:          "ES",
	HandwritingLanguage: LanguageEs,
	VoiceLanguage:       LanguageEs,
}

// Swedish represents the input method of Swedish.
var Swedish = InputMethod{
	Name:                "Swedish",
	ID:                  "xkb:se::swe",
	ShortLabel:          "SE",
	HandwritingLanguage: LanguageSv,
	VoiceLanguage:       LanguageSv,
}

// Tamil represents the input method of Tamil.
var Tamil = InputMethod{
	Name:                "Tamil",
	ID:                  "ta-t-i0-und",
	HandwritingLanguage: LanguageTa,
	VoiceLanguage:       LanguageTa,
}

// Telugu represents the input method of Telugu.
var Telugu = InputMethod{
	Name:                "Telugu",
	ID:                  "te-t-i0-und",
	HandwritingLanguage: LanguageTe,
	VoiceLanguage:       LanguageTe,
}

// ThaiTis represents the input method of Thai (TIS).
var ThaiTis = InputMethod{
	Name:                "ThaiTis",
	ID:                  "vkd_th_tis",
	HandwritingLanguage: LanguageTh,
	VoiceLanguage:       LanguageTh,
}

// Turkish represents the input method of Turkish.
var Turkish = InputMethod{
	Name:                "Turkish",
	ID:                  "xkb:tr:f:tur",
	HandwritingLanguage: LanguageTr,
	VoiceLanguage:       LanguageTr,
}

// Urdu represents the input method of Urdu.
var Urdu = InputMethod{
	Name:                "Urdu",
	ID:                  "ur-t-i0-und",
	HandwritingLanguage: LanguageUr,
	VoiceLanguage:       LanguageUr,
}

// VietnameseTelex represents the input method of Telex.
var VietnameseTelex = InputMethod{
	Name:                "Vietnamese Telex",
	ID:                  "vkd_vi_telex",
	HandwritingLanguage: LanguageVi,
	VoiceLanguage:       LanguageVi,
}

// VietnameseVNI represents the input method of VNI.
var VietnameseVNI = InputMethod{
	Name:                "Vietnamese VNI",
	ID:                  "vkd_vi_vni",
	HandwritingLanguage: LanguageVi,
	VoiceLanguage:       LanguageVi,
}

// inputMethods represents in-use (available) IMEs in ChromeOS.
// Any IMEs displayed in OS settings can be added to this list.
var inputMethods = []InputMethod{
	EnglishIndia,
	EnglishPakistan,
	EnglishUS,
	EnglishUSWithInternationalKeyboard,
	EnglishUSWithExtendedKeyboard,
	EnglishUSWithDvorakKeyboard,
	EnglishUSWithColemakKeyboard,
	EnglishUSWithWorkmanKeyboard,
	EnglishUK,
	EnglishSouthAfrica,
	SpanishSpain,
	Swedish,
	AlphanumericWithJapaneseKeyboard,
	EnglishCanada,
	Japanese,
	FrenchFrance,
	JapaneseWithUSKeyboard,
	Cantonese,
	ChineseArray,
	ChineseCangjie,
	ChineseDayi,
	ChinesePinyin,
	ChineseQuick,
	ChineseTraditionalPinyin,
	ChineseWubi,
	ChineseZhuyin,
	DutchNetherlands,
	Korean,
	Arabic,
	Bulgarian,
	Croatian,
	Czech,
	Danish,
	Finnish,
	Georgian,
	German,
	GreekTransliteration,
	Gujarati,
	Hebrew,
	Hindi,
	Hungarian,
	Icelandic,
	Irish,
	Italian,
	Kannada,
	Kazakh,
	Khmer,
	Latvian,
	Macedonian,
	Malayalam,
	Maltese,
	Marathi,
	Myanmar,
	Mongolian,
	NepaliTransliteration,
	Norwegian,
	Odia,
	PersianTransliteration,
	Polish,
	Portuguese,
	Punjabi,
	Romanian,
	Russian,
	Sanskrit,
	Serbian,
	Sinhala,
	Slovak,
	Slovenian,
	Tamil,
	Telugu,
	ThaiTis,
	Turkish,
	Urdu,
	VietnameseTelex,
	VietnameseVNI,
}

// ErrInputNotDefined indicates that the input method has not been defined.
var ErrInputNotDefined = errors.New("IME code has not been defined")

// ActiveInputMethod returns the active input method via Chrome API.
func ActiveInputMethod(ctx context.Context, tconn *chrome.TestConn) (*InputMethod, error) {
	fullyQualifiedIMEID, err := CurrentInputMethod(ctx, tconn)
	if err != nil {
		return nil, err
	}
	return FindInputMethodByFullyQualifiedIMEID(ctx, tconn, fullyQualifiedIMEID)
}

// FindInputMethodByName finds the input method by displayed name.
func FindInputMethodByName(name string) (*InputMethod, error) {
	for _, im := range inputMethods {
		if im.Name == name {
			return &im, nil
		}
	}
	return nil, errors.Errorf("failed to find input method by name %q", name)
}

// FindInputMethodByID finds the input method by ime id.
func FindInputMethodByID(id string) (*InputMethod, error) {
	for _, im := range inputMethods {
		if im.ID == id {
			return &im, nil
		}
	}
	return nil, errors.Errorf("failed to find input method by IME id %q", id)
}

// FindInputMethodByFullyQualifiedIMEID finds the input method by fully qualified IME ID,
// e.g. _comp_ime_jkghodnilhceideoidjikpgommlajknkxkb:us::eng.
func FindInputMethodByFullyQualifiedIMEID(ctx context.Context, tconn *chrome.TestConn, fullyQualifiedIMEID string) (*InputMethod, error) {
	imePrefix, err := Prefix(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get IME prefix")
	}
	for _, im := range inputMethods {
		if imePrefix+im.ID == fullyQualifiedIMEID {
			return &im, nil
		}
	}
	return nil, errors.Wrapf(ErrInputNotDefined, "failed to find input method by IME Code %q", fullyQualifiedIMEID)
}

// FullyQualifiedIMEID returns the fully qualified IME id constructed by IMEPrefix + IME ID.
// In Chrome, the fully qualified IME id is Chrome IME prefix + id: e.g. _comp_ime_jkghodnilhceideoidjikpgommlajknkxkb:us::eng
// In Chromium, the fully qualified IME id is Chromium IME prefix + id: e.g. _comp_ime_fgoepimhcoialccpbmpnnblemnepkkaoxkb:us::eng
func (im InputMethod) FullyQualifiedIMEID(ctx context.Context, tconn *chrome.TestConn) (string, error) {
	imePrefix, err := Prefix(ctx, tconn)
	if err != nil {
		return "", errors.Wrap(err, "failed to get IME prefix")
	}
	return imePrefix + im.ID, nil
}

// Equal compares two input methods by id and returns true if they equal.
func (im InputMethod) Equal(imb InputMethod) bool {
	return im.ID == imb.ID
}

// String returns the key representative string content of the input method.
func (im InputMethod) String() string {
	return fmt.Sprintf("ID: %s; Name: %s", im.ID, im.Name)
}

// Install installs the input method via Chrome API.
// It does nothing if the IME is already installed.
func (im InputMethod) Install(tconn *chrome.TestConn) action.Action {
	f := func(ctx context.Context, fullyQualifiedIMEID string) error {
		fullyQualifiedIMEs, err := InstalledInputMethods(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to get installed input methods")
		}

		for _, installedIME := range fullyQualifiedIMEs {
			if installedIME.ID == fullyQualifiedIMEID {
				return nil
			}
		}
		return AddInputMethod(ctx, tconn, fullyQualifiedIMEID)
	}
	return im.actionWithFullyQualifiedID(tconn, f)
}

// WaitUntilInstalled waits for the input method to be installed.
func (im InputMethod) WaitUntilInstalled(tconn *chrome.TestConn) action.Action {
	f := func(ctx context.Context, fullyQualifiedIMEID string) error {
		return WaitForInputMethodInstalled(ctx, tconn, fullyQualifiedIMEID, 20*time.Second)
	}
	return im.actionWithFullyQualifiedID(tconn, f)
}

// WaitUntilRemoved waits for the input method to be removed.
func (im InputMethod) WaitUntilRemoved(tconn *chrome.TestConn) action.Action {
	f := func(ctx context.Context, fullyQualifiedIMEID string) error {
		return WaitForInputMethodRemoved(ctx, tconn, fullyQualifiedIMEID, 20*time.Second)
	}
	return im.actionWithFullyQualifiedID(tconn, f)
}

// Activate sets the input method to use via Chrome API.
// It does nothing if the IME is already in use.
func (im InputMethod) Activate(tconn *chrome.TestConn) action.Action {
	return uiauto.NamedCombine(fmt.Sprintf("activating input method %q", im.Name),
		im.SetCurrentInputMethod(tconn),
		im.WaitUntilActivated(tconn),
	)
}

// SetCurrentInputMethod sets the current input method to use via Chrome API.
// Note: The IME might still need sometime to warm up after this.
// It's recommended to use im.Activate for input functional testing.
func (im InputMethod) SetCurrentInputMethod(tconn *chrome.TestConn) action.Action {
	f := func(ctx context.Context, fullyQualifiedIMEID string) error {
		activeIME, err := ActiveInputMethod(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to get active input method")
		}
		if activeIME.Equal(im) {
			return nil
		}

		return tconn.Call(ctx, nil, `chrome.inputMethodPrivate.setCurrentInputMethod`, fullyQualifiedIMEID)
	}
	return im.actionWithFullyQualifiedID(tconn, f)
}

// A list of input methods that can be detected in testing readiness via Chromium API
// chrome.autotestPrivate.isInputMethodReadyForTesting.
var imesDetectedByAPI = []InputMethod{
	ChineseZhuyin,
	ChineseArray,
	ChineseCangjie,
	ChineseDayi,
	ChineseQuick,
	ChineseTraditionalPinyin,
	ChineseWubi,
}

// WaitUntilActivated waits until the certain input method to be activated.
func (im InputMethod) WaitUntilActivated(tconn *chrome.TestConn) action.Action {
	// Use 12s as warming up time by default.
	imWarmingUpTime := 12 * time.Second

	// Some IMEs are known to take longer time on low-end VM/HW.
	switch im {
	case Swedish, FrenchFrance, SpanishSpain, Korean, Cantonese, ChinesePinyin:
		imWarmingUpTime = 17 * time.Second
	}

	f := func(ctx context.Context, fullyQualifiedIMEID string) error {
		// TODO(b/195374149) re-enable using readiness API once it is available.
		return WaitForInputMethodActivatedWithSleep(ctx, tconn, fullyQualifiedIMEID, imWarmingUpTime)
	}
	return im.actionWithFullyQualifiedID(tconn, f)
}

// InstallAndActivate installs the input method and set it to active via Chrome API.
func (im InputMethod) InstallAndActivate(tconn *chrome.TestConn) action.Action {
	return uiauto.Combine(fmt.Sprintf("install and activate input method: %q", im),
		im.Install(tconn),
		im.Activate(tconn),
	)
}

// InstallAndActivateUserAction returns an user action installing the input method
// and set it to active via Chrome API. It can be used to collect DUT environment information
// and measure the perforamce of installing a certain input method.
func (im InputMethod) InstallAndActivateUserAction(uc *useractions.UserContext) action.Action {
	return uiauto.UserAction(
		"Add and activate input method via Chromium API",
		im.InstallAndActivate(uc.TestAPIConn()),
		uc, &useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeFeature:     useractions.FeatureIMEManagement,
				useractions.AttributeInputMethod: im.Name,
			},
		},
	)
}

// Remove uninstalls the input method via Chrome API.
func (im InputMethod) Remove(tconn *chrome.TestConn) action.Action {
	f := func(ctx context.Context, fullyQualifiedIMEID string) error {
		return RemoveInputMethod(ctx, tconn, fullyQualifiedIMEID)
	}
	return im.actionWithFullyQualifiedID(tconn, f)
}

func (im InputMethod) actionWithFullyQualifiedID(tconn *chrome.TestConn, f func(ctx context.Context, fullyQualifiedIMEID string) error) action.Action {
	return func(ctx context.Context) error {
		fullyQualifiedIMEID, err := im.FullyQualifiedIMEID(ctx, tconn)
		if err != nil {
			return errors.Wrapf(err, "failed to get fully qualified IME ID of %q", im)
		}
		return f(ctx, fullyQualifiedIMEID)
	}
}

// SetSettings changes the IME setting via chrome api.
// `chrome.inputMethodPrivate.setSettings(
//
//	"xkb:us::eng", { "physicalKeyboardAutoCorrectionLevel": 1})`,
//
// Note: Settings change won't take effect until the next input session.
// e.g. focus on a text field, or change input method.
// Live setting change is not supported because it never happens in a real user environment.
func (im InputMethod) SetSettings(tconn *chrome.TestConn, settings map[string]interface{}) action.Action {
	return func(ctx context.Context) error {
		settingJSON, err := json.Marshal(settings)
		if err != nil {
			return errors.Wrapf(err, "failed to read settings: %+v", settings)
		}

		var settingsAPICall = fmt.Sprintf(
			`chrome.inputMethodPrivate.setSettings(
					 %q, %s)`,
			im.ID, settingJSON)

		return tconn.Eval(ctx, settingsAPICall, nil)
	}
}

// ResetSettings empties IME settings to reset.
func (im InputMethod) ResetSettings(tconn *chrome.TestConn) action.Action {
	return im.SetSettings(tconn, map[string]interface{}{})
}

// SetPKAutoCorrection whether enables or disables the physical keyboard auto correction.
func (im InputMethod) SetPKAutoCorrection(tconn *chrome.TestConn, acLevel AutoCorrectionLevel) action.Action {
	settings := map[string]interface{}{"physicalKeyboardAutoCorrectionLevel": acLevel}
	return im.SetSettings(tconn, settings)
}

// SetVKAutoCorrection whether enables or disables the physical keyboard auto correction.
func (im InputMethod) SetVKAutoCorrection(tconn *chrome.TestConn, acLevel AutoCorrectionLevel) action.Action {
	settings := map[string]interface{}{"virtualKeyboardAutoCorrectionLevel": acLevel}
	return im.SetSettings(tconn, settings)
}

// SetVKEnableCapitalization whether enables or disables auto capitalization.
func (im InputMethod) SetVKEnableCapitalization(tconn *chrome.TestConn, isEnabled bool) action.Action {
	settings := map[string]interface{}{"virtualKeyboardEnableCapitalization": isEnabled}
	return im.SetSettings(tconn, settings)
}

// InputMethods returns in-use (available) IMEs in ChromeOS.
func InputMethods() []InputMethod {
	return inputMethods
}

// AutoCorrectionLevel describes the auto correction level of an input method.
type AutoCorrectionLevel int

// Available auto correction levels.
const (
	AutoCorrectionOff AutoCorrectionLevel = iota
	AutoCorrectionModest
	AutoCorrectionProgressive
)
