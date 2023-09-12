package internal

import (
	"regexp"
	"strings"
)

// Remixed from https://github.com/python/cpython/blob/HEAD/Lib/shlex.py
// See: shlex.join & shlex.quote

// SHJoin returns a shell-escaped string from args.
func SHJoin(args []string) string {
	var b strings.Builder

	for i, arg := range args {
		if i != 0 {
			b.WriteByte(' ')
		}
		b.WriteString(SHQuote(arg))
	}

	return b.String()
}

var unsafePattern = regexp.MustCompile(`[^\w@%+=:,./-]`)

// SHQuote returns a shell-escaped version of s.
func SHQuote(s string) string {
	if s == "" {
		return "''"
	}

	if !unsafePattern.MatchString(s) {
		return s
	}

	// Use single quotes, and put single quotes into double quotes.
	// The string $'b is then quoted as '$'"'"'b'.
	return "'" + strings.ReplaceAll(s, `'`, `'"'"'`) + "'"
}
