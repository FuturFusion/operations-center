//go:build ruleguard
// +build ruleguard

package ruleguard

import "github.com/quasilyte/go-ruleguard/dsl"

func noOneLineAssignAndTest(m dsl.Matcher) {
	m.Match(
		"if $*_ := $*_; $_ { $*_ }",
	).Report("no oneline assign and test")
}
