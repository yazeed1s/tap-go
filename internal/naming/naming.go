// Package naming converts Tap API identifiers into Go identifiers. It is
// shared by the spec builder and the SDK generator so both agree on names.
package naming

import (
	"strings"
	"unicode"
)

var initialisms = map[string]string{
	"api":   "API",
	"bin":   "BIN",
	"cvc":   "CVC",
	"cvv":   "CVV",
	"html":  "HTML",
	"http":  "HTTP",
	"https": "HTTPS",
	"iban":  "IBAN",
	"id":    "ID",
	"ids":   "IDs",
	"ip":    "IP",
	"json":  "JSON",
	"otp":   "OTP",
	"pdf":   "PDF",
	"sdk":   "SDK",
	"sms":   "SMS",
	"uri":   "URI",
	"url":   "URL",
	"urls":  "URLs",
	"uuid":  "UUID",
	"vat":   "VAT",
}

// Words splits raw into lower-case words on separators and camelCase
// boundaries: "card_threeDSecure" -> [card three d secure].
func Words(raw string) []string {
	var words []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			words = append(words, strings.ToLower(string(cur)))
			cur = cur[:0]
		}
	}
	runes := []rune(raw)
	for i, r := range runes {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			flush()
			continue
		}
		if i > 0 && len(cur) > 0 && unicode.IsUpper(r) {
			prev := runes[i-1]
			nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if unicode.IsLower(prev) || unicode.IsDigit(prev) ||
				(unicode.IsUpper(prev) && nextLower) {
				flush()
			}
		}
		cur = append(cur, r)
	}
	flush()
	return words
}

// Export returns the exported Go identifier for raw, e.g.
// "card_threeDSecure" -> "CardThreeDSecure", "customer_id" -> "CustomerID".
func Export(raw string) string {
	var out strings.Builder
	for _, word := range Words(raw) {
		if repl, ok := initialisms[word]; ok {
			out.WriteString(repl)
			continue
		}
		out.WriteString(strings.ToUpper(word[:1]))
		out.WriteString(word[1:])
	}
	name := out.String()
	if name == "" {
		return "Value"
	}
	if name[0] >= '0' && name[0] <= '9' {
		return "N" + name
	}
	return name
}

// Lower returns the unexported form of an exported identifier, e.g.
// "CustomerID" -> "customerID", "ID" -> "id", "URLList" -> "urlList".
func Lower(name string) string {
	if name == "" {
		return "value"
	}
	runes := []rune(name)
	i := 0
	for i < len(runes) && unicode.IsUpper(runes[i]) {
		i++
	}
	switch {
	case i == len(runes):
		return strings.ToLower(name)
	case i > 1:
		i--
	}
	if i == 0 {
		i = 1
	}
	return strings.ToLower(string(runes[:i])) + string(runes[i:])
}

// Singular trims a simple English plural: "charges" -> "charge",
// "entities" -> "entity". Words it does not recognize are returned as is.
func Singular(word string) string {
	switch {
	case strings.HasSuffix(word, "ies") && len(word) > 4:
		return word[:len(word)-3] + "y"
	case strings.HasSuffix(word, "sses"):
		return word[:len(word)-2]
	case strings.HasSuffix(word, "ss"), strings.HasSuffix(word, "us"):
		return word
	case strings.HasSuffix(word, "s") && len(word) > 3:
		return word[:len(word)-1]
	}
	return word
}
