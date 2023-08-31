package main

import (
	"regexp"
	"strings"
)

// Remixed from https://github.com/python/cpython/blob/HEAD/Lib/shlex.py
// See: shlex.join & shlex.quote

// commandJoin returns a shell-escaped string from args.
func commandJoin(args []string) string {
	var b strings.Builder

	for i, arg := range args {
		if i != 0 {
			b.WriteByte(' ')
		}
		b.WriteString(commandQuote(arg))
	}

	return b.String()
}

var findUnsafe = regexp.MustCompile(`[^\w@%+=:,./-]`).MatchString

// commandQuote returns a shell-escaped version of s.
func commandQuote(s string) string {
	if s == "" {
		return "''"
	}

	if !findUnsafe(s) {
		return s
	}

	// Use single quotes, and put single quotes into double quotes.
	// The string $'b is then quoted as '$'"'"'b'.
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
