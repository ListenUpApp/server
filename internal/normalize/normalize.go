// Package normalize provides utilities for normalizing and sanitizing data.
package normalize

import (
	"strings"

	"github.com/listenupapp/listenup-server/internal/genre"
)

// iso639_2to1 maps ISO 639-2 (3-letter) codes to ISO 639-1 (2-letter) codes.
//
//nolint:gochecknoglobals // Static lookup table for language normalization
var iso639_2to1 = map[string]string{
	"eng": "en", "spa": "es", "fra": "fr", "deu": "de", "ita": "it",
	"por": "pt", "nld": "nl", "rus": "ru", "jpn": "ja", "zho": "zh",
	"kor": "ko", "ara": "ar", "hin": "hi", "pol": "pl", "swe": "sv",
	"nor": "no", "dan": "da", "fin": "fi", "tur": "tr", "ell": "el",
	"heb": "he", "ces": "cs", "hun": "hu", "ron": "ro", "tha": "th",
	"vie": "vi", "ind": "id", "msa": "ms", "ukr": "uk", "cat": "ca",
	"hrv": "hr", "slk": "sk", "bul": "bg", "lit": "lt", "lav": "lv",
	"est": "et", "slv": "sl", "srp": "sr", "fas": "fa", "ben": "bn",
	"tam": "ta", "tel": "te", "mar": "mr", "guj": "gu", "kan": "kn",
	"mal": "ml", "pan": "pa", "urd": "ur", "nep": "ne", "sin": "si",
	"mya": "my", "khm": "km", "lao": "lo", "amh": "am", "swa": "sw",
	"afr": "af", "zul": "zu", "xho": "xh", "hau": "ha", "yor": "yo",
	"ibo": "ig", "cym": "cy", "gle": "ga", "gla": "gd", "eus": "eu",
	"glg": "gl", "isl": "is", "mkd": "mk", "bos": "bs", "sqi": "sq",
	"hye": "hy", "kat": "ka", "kaz": "kk", "uzb": "uz", "azj": "az",
	"mon": "mn", "tgl": "tl", "fil": "tl", "jav": "jv", "sun": "su",
	// Alternative ISO 639-2/B codes (bibliographic)
	"ger": "de", "fre": "fr", "dut": "nl", "chi": "zh", "cze": "cs",
	"gre": "el", "per": "fa", "rum": "ro", "slo": "sk", "alb": "sq",
	"arm": "hy", "baq": "eu", "bur": "my", "geo": "ka", "ice": "is",
	"mac": "mk", "may": "ms", "tib": "bo", "wel": "cy",
}

// languageNameToCode maps common language names to ISO 639-1 codes.
//
//nolint:gochecknoglobals // Static lookup table for language normalization
var languageNameToCode = map[string]string{
	"english": "en", "spanish": "es", "french": "fr", "german": "de",
	"italian": "it", "portuguese": "pt", "dutch": "nl", "russian": "ru",
	"japanese": "ja", "chinese": "zh", "korean": "ko", "arabic": "ar",
	"hindi": "hi", "polish": "pl", "swedish": "sv", "norwegian": "no",
	"danish": "da", "finnish": "fi", "turkish": "tr", "greek": "el",
	"hebrew": "he", "czech": "cs", "hungarian": "hu", "romanian": "ro",
	"thai": "th", "vietnamese": "vi", "indonesian": "id", "malay": "ms",
	"ukrainian": "uk", "catalan": "ca", "croatian": "hr", "slovak": "sk",
	"bulgarian": "bg", "lithuanian": "lt", "latvian": "lv", "estonian": "et",
	"slovenian": "sl", "serbian": "sr", "persian": "fa", "farsi": "fa",
	"bengali": "bn", "tamil": "ta", "telugu": "te", "marathi": "mr",
	"gujarati": "gu", "kannada": "kn", "malayalam": "ml", "punjabi": "pa",
	"urdu": "ur", "nepali": "ne", "sinhala": "si", "burmese": "my",
	"khmer": "km", "lao": "lo", "amharic": "am", "swahili": "sw",
	"afrikaans": "af", "zulu": "zu", "xhosa": "xh", "hausa": "ha",
	"yoruba": "yo", "igbo": "ig", "welsh": "cy", "irish": "ga",
	"scottish gaelic": "gd", "basque": "eu", "galician": "gl", "icelandic": "is",
	"macedonian": "mk", "bosnian": "bs", "albanian": "sq", "armenian": "hy",
	"georgian": "ka", "kazakh": "kk", "uzbek": "uz", "azerbaijani": "az",
	"mongolian": "mn", "tagalog": "tl", "filipino": "tl", "javanese": "jv",
	"sundanese": "su", "mandarin": "zh", "cantonese": "zh", "tibetan": "bo",
}

// validISO639_1 contains all valid ISO 639-1 codes for validation.
//
//nolint:gochecknoglobals // Static lookup table for language normalization
var validISO639_1 = map[string]bool{
	"aa": true, "ab": true, "ae": true, "af": true, "ak": true, "am": true,
	"an": true, "ar": true, "as": true, "av": true, "ay": true, "az": true,
	"ba": true, "be": true, "bg": true, "bh": true, "bi": true, "bm": true,
	"bn": true, "bo": true, "br": true, "bs": true, "ca": true, "ce": true,
	"ch": true, "co": true, "cr": true, "cs": true, "cu": true, "cv": true,
	"cy": true, "da": true, "de": true, "dv": true, "dz": true, "ee": true,
	"el": true, "en": true, "eo": true, "es": true, "et": true, "eu": true,
	"fa": true, "ff": true, "fi": true, "fj": true, "fo": true, "fr": true,
	"fy": true, "ga": true, "gd": true, "gl": true, "gn": true, "gu": true,
	"gv": true, "ha": true, "he": true, "hi": true, "ho": true, "hr": true,
	"ht": true, "hu": true, "hy": true, "hz": true, "ia": true, "id": true,
	"ie": true, "ig": true, "ii": true, "ik": true, "io": true, "is": true,
	"it": true, "iu": true, "ja": true, "jv": true, "ka": true, "kg": true,
	"ki": true, "kj": true, "kk": true, "kl": true, "km": true, "kn": true,
	"ko": true, "kr": true, "ks": true, "ku": true, "kv": true, "kw": true,
	"ky": true, "la": true, "lb": true, "lg": true, "li": true, "ln": true,
	"lo": true, "lt": true, "lu": true, "lv": true, "mg": true, "mh": true,
	"mi": true, "mk": true, "ml": true, "mn": true, "mr": true, "ms": true,
	"mt": true, "my": true, "na": true, "nb": true, "nd": true, "ne": true,
	"ng": true, "nl": true, "nn": true, "no": true, "nr": true, "nv": true,
	"ny": true, "oc": true, "oj": true, "om": true, "or": true, "os": true,
	"pa": true, "pi": true, "pl": true, "ps": true, "pt": true, "qu": true,
	"rm": true, "rn": true, "ro": true, "ru": true, "rw": true, "sa": true,
	"sc": true, "sd": true, "se": true, "sg": true, "si": true, "sk": true,
	"sl": true, "sm": true, "sn": true, "so": true, "sq": true, "sr": true,
	"ss": true, "st": true, "su": true, "sv": true, "sw": true, "ta": true,
	"te": true, "tg": true, "th": true, "ti": true, "tk": true, "tl": true,
	"tn": true, "to": true, "tr": true, "ts": true, "tt": true, "tw": true,
	"ty": true, "ug": true, "uk": true, "ur": true, "uz": true, "ve": true,
	"vi": true, "vo": true, "wa": true, "wo": true, "xh": true, "yi": true,
	"yo": true, "za": true, "zh": true, "zu": true,
}

// ISO 639-1 code to display name mapping.
//
//nolint:gochecknoglobals // Static lookup table
var codeToLanguageName = map[string]string{
	"en": "English", "es": "Spanish", "fr": "French", "de": "German",
	"it": "Italian", "pt": "Portuguese", "nl": "Dutch", "ru": "Russian",
	"ja": "Japanese", "zh": "Chinese", "ko": "Korean", "ar": "Arabic",
	"hi": "Hindi", "pl": "Polish", "sv": "Swedish", "no": "Norwegian",
	"da": "Danish", "fi": "Finnish", "tr": "Turkish", "el": "Greek",
	"he": "Hebrew", "cs": "Czech", "hu": "Hungarian", "ro": "Romanian",
	"th": "Thai", "vi": "Vietnamese", "id": "Indonesian", "ms": "Malay",
	"uk": "Ukrainian", "ca": "Catalan", "hr": "Croatian", "sk": "Slovak",
	"bg": "Bulgarian", "lt": "Lithuanian", "lv": "Latvian", "et": "Estonian",
	"sl": "Slovenian", "sr": "Serbian", "fa": "Persian", "bn": "Bengali",
	"ta": "Tamil", "te": "Telugu", "mr": "Marathi", "gu": "Gujarati",
	"kn": "Kannada", "ml": "Malayalam", "pa": "Punjabi", "ur": "Urdu",
	"ne": "Nepali", "si": "Sinhala", "my": "Burmese", "km": "Khmer",
	"lo": "Lao", "am": "Amharic", "sw": "Swahili", "af": "Afrikaans",
	"zu": "Zulu", "xh": "Xhosa", "ha": "Hausa", "yo": "Yoruba",
	"ig": "Igbo", "cy": "Welsh", "ga": "Irish", "gd": "Scottish Gaelic",
	"eu": "Basque", "gl": "Galician", "is": "Icelandic", "mk": "Macedonian",
	"bs": "Bosnian", "sq": "Albanian", "hy": "Armenian", "ka": "Georgian",
	"kk": "Kazakh", "uz": "Uzbek", "az": "Azerbaijani", "mn": "Mongolian",
	"tl": "Tagalog", "jv": "Javanese", "su": "Sundanese", "bo": "Tibetan",
}

// LanguageCode converts various language representations to ISO 639-1 codes.
// It handles:
//   - ISO 639-1 codes: "en" -> "en"
//   - ISO 639-2 codes: "eng" -> "en"
//   - Locale codes: "en-US", "en_GB" -> "en"
//   - Language names: "English", "ENGLISH" -> "en"
//
// Returns empty string for unrecognized values.
func LanguageCode(raw string) string {
	if raw == "" {
		return ""
	}

	// Sanitize and normalize case.
	s := strings.ToLower(strings.TrimSpace(sanitizeString(raw)))
	if s == "" {
		return ""
	}

	// Handle locale codes (e.g., "en-US", "en_GB").
	// Split on common separators and use the first part.
	if idx := strings.IndexAny(s, "-_"); idx > 0 {
		s = s[:idx]
	}

	// Check if already valid ISO 639-1.
	if len(s) == 2 && validISO639_1[s] {
		return s
	}

	// Try ISO 639-2 (3-letter) to ISO 639-1 mapping.
	if len(s) == 3 {
		if code, ok := iso639_2to1[s]; ok {
			return code
		}
	}

	// Try language name mapping.
	if code, ok := languageNameToCode[s]; ok {
		return code
	}

	// Unrecognized - return empty.
	return ""
}

// Language converts various language representations to display names.
// "en" -> "English", "german" -> "German", "deu" -> "German"
// Returns empty string for unrecognized values.
func Language(raw string) string {
	// First convert to ISO code
	code := LanguageCode(raw)
	if code == "" {
		return ""
	}

	// Then look up display name
	if name, ok := codeToLanguageName[code]; ok {
		return name
	}

	return ""
}

// sanitizeString removes null bytes from strings, which can cause
// issues in databases and JSON parsing. Some audio metadata parsers include
// null terminators in strings.
func sanitizeString(s string) string {
	return strings.Map(func(r rune) rune {
		if r == 0 { // null byte
			return -1 // drop it
		}
		return r
	}, s)
}

// GenreSlugs normalizes a raw genre string to canonical slugs.
// Delegates to genre.NormalizeToSlugs for consistency.
func GenreSlugs(raw string) []string {
	return genre.NormalizeToSlugs(raw)
}
