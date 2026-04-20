package main

import (
	"regexp"
	"strings"
)

// fnmatch implements Python-compatible fnmatch where * matches any character
// including path separators. Go's filepath.Match treats * as non-separator only.
func fnmatch(pattern, name string) bool {
	re := fnmatchToRegexp(pattern)
	return re.MatchString(name)
}

func fnmatchToRegexp(pattern string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteByte('.')
		case '.', '+', '^', '$', '(', ')', '{', '}', '|', '\\':
			b.WriteByte('\\')
			b.WriteByte(pattern[i])
		case '[':
			j := i + 1
			for j < len(pattern) && pattern[j] != ']' {
				j++
			}
			if j < len(pattern) {
				b.WriteString(pattern[i : j+1])
				i = j
			} else {
				b.WriteString(regexp.QuoteMeta(string(pattern[i])))
			}
		default:
			b.WriteByte(pattern[i])
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return regexp.MustCompile(regexp.QuoteMeta(pattern))
	}
	return re
}
