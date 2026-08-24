package processing

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

const lowerHex = "0123456789abcdef"

// sanitizeDiagnosticMessage preserves printable Unicode while rendering text that could alter log structure or
// terminal presentation as visible Go-style escape sequences.
func sanitizeDiagnosticMessage(message string) string {
	if utf8.ValidString(message) {
		safe := true
		for _, value := range message {
			if !strconv.IsGraphic(value) {
				safe = false
				break
			}
		}
		if safe {
			return message
		}
	}

	var sanitized strings.Builder
	sanitized.Grow(len(message))
	for len(message) > 0 {
		value, size := utf8.DecodeRuneInString(message)
		if value == utf8.RuneError && size == 1 {
			sanitized.WriteString(`\x`)
			sanitized.WriteByte(lowerHex[message[0]>>4])
			sanitized.WriteByte(lowerHex[message[0]&0x0f])
			message = message[1:]
			continue
		}
		if strconv.IsGraphic(value) {
			sanitized.WriteString(message[:size])
		} else {
			quoted := strconv.QuoteRuneToGraphic(value)
			sanitized.WriteString(quoted[1 : len(quoted)-1])
		}
		message = message[size:]
	}
	return sanitized.String()
}
