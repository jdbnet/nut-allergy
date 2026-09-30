// Package version is set with -ldflags at build time.
package version

import (
	"strconv"
	"strings"
)

// Version is the release name, without a leading v. Development builds stay "dev".
var Version = "dev"

// Commit is the git commit the binary was built from.
var Commit = ""

// Repo is the GitHub repository as owner/name. Empty skips the update check.
var Repo = "jdbnet/nut-allergy"

// Newer reports whether latest is a higher release than current.
// A development build is older than any numbered release.
func Newer(latest, current string) bool {
	l, lok := parts(latest)
	c, cok := parts(current)
	if !lok {
		return false
	}
	if !cok {
		return true
	}
	n := len(l)
	if len(c) > n {
		n = len(c)
	}
	for i := 0; i < n; i++ {
		lv, cv := 0, 0
		if i < len(l) {
			lv = l[i]
		}
		if i < len(c) {
			cv = c[i]
		}
		if lv != cv {
			return lv > cv
		}
	}
	return false
}

func parts(v string) ([]int, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if v == "" || v == "dev" {
		return nil, false
	}
	raw := strings.Split(v, ".")
	out := make([]int, 0, len(raw))
	for _, p := range raw {
		n, ok := leadingInt(p)
		if !ok {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

func leadingInt(s string) (int, bool) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(s[:i])
	return n, err == nil
}
