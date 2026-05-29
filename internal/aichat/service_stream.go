package aichat

import (
	"strings"
)

type assistantMessageExtractor struct {
	keyFound      bool
	colonFound    bool
	inString      bool
	escape        bool
	unicodeMode   bool
	unicodeDigits []rune

	window  []rune
	decoded strings.Builder
	emitted int
	done    bool
}

func newAssistantMessageExtractor() *assistantMessageExtractor {
	return &assistantMessageExtractor{
		window: make([]rune, 0, 64),
	}
}

func (e *assistantMessageExtractor) Feed(rawDelta string) string {
	if e.done || rawDelta == "" {
		return ""
	}
	for _, r := range rawDelta {
		if e.done {
			break
		}
		if !e.keyFound {
			e.window = append(e.window, r)
			if len(e.window) > 80 {
				e.window = e.window[len(e.window)-80:]
			}
			windowStr := string(e.window)
			if strings.Contains(windowStr, "\"assistantMessage\"") || strings.Contains(windowStr, "\"assistant_message\"") {
				e.keyFound = true
			}
			continue
		}

		if !e.colonFound {
			if r == ':' {
				e.colonFound = true
			}
			continue
		}

		if !e.inString {
			if r == '"' {
				e.inString = true
			}
			continue
		}

		// inside JSON string
		if e.unicodeMode {
			if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
				e.unicodeDigits = append(e.unicodeDigits, r)
				if len(e.unicodeDigits) == 4 {
					var value rune
					for _, d := range e.unicodeDigits {
						value <<= 4
						switch {
						case d >= '0' && d <= '9':
							value += d - '0'
						case d >= 'a' && d <= 'f':
							value += d - 'a' + 10
						case d >= 'A' && d <= 'F':
							value += d - 'A' + 10
						}
					}
					e.decoded.WriteRune(value)
					e.unicodeDigits = e.unicodeDigits[:0]
					e.unicodeMode = false
				}
				continue
			}
			// invalid unicode escape; reset state
			e.unicodeDigits = e.unicodeDigits[:0]
			e.unicodeMode = false
			e.escape = false
		}

		if e.escape {
			switch r {
			case '"', '\\', '/':
				e.decoded.WriteRune(r)
			case 'b':
				e.decoded.WriteByte('\b')
			case 'f':
				e.decoded.WriteByte('\f')
			case 'n':
				e.decoded.WriteByte('\n')
			case 'r':
				e.decoded.WriteByte('\r')
			case 't':
				e.decoded.WriteByte('\t')
			case 'u':
				e.unicodeMode = true
				if e.unicodeDigits == nil {
					e.unicodeDigits = make([]rune, 0, 4)
				} else {
					e.unicodeDigits = e.unicodeDigits[:0]
				}
			default:
				e.decoded.WriteRune(r)
			}
			e.escape = false
			continue
		}

		switch r {
		case '\\':
			e.escape = true
		case '"':
			e.done = true
			e.inString = false
		default:
			e.decoded.WriteRune(r)
		}
	}

	decoded := e.decoded.String()
	if len(decoded) <= e.emitted {
		return ""
	}
	delta := decoded[e.emitted:]
	e.emitted = len(decoded)
	return delta
}
