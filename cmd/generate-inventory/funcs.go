package main

import (
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Capital capitalizes the given string ("foo" -> "Foo").
func Capital(s string) string {
	return cases.Title(language.English, cases.NoLower).String(s)
}

// Minuscule turns the first character to lower case ("Foo" -> "foo") or the whole word if it is all uppercase ("UUID" -> "uuid").
func Minuscule(s string) string {
	if strings.ToUpper(s) == s {
		return strings.ToLower(s)
	}

	return strings.ToLower(s[:1]) + s[1:]
}

var acronyms = map[string]struct{}{
	"acl": {},
}

// CamelCase converts to camel case ("foo_bar" -> "fooBar").
// If a segment (with the exception of the first one) is a known acronym,
// it is returned in all upper case.
func CamelCase(s string) string {
	return Minuscule(PascalCase(s))
}

// PascalCase converts to pascal case ("foo_bar" -> "FooBar").
// If a segment is a known acronym, it is returned in all upper case.
func PascalCase(s string) string {
	words := strings.Split(s, "_")
	for i := range words {
		_, ok := acronyms[strings.ToLower(words[i])]
		if ok {
			words[i] = strings.ToUpper(words[i])
			continue
		}

		// Plural?
		if strings.HasSuffix(words[i], "s") {
			w := strings.TrimSuffix(words[i], "s")
			_, ok := acronyms[strings.ToLower(w)]
			if ok {
				words[i] = strings.ToUpper(w) + "s"
				continue
			}
		}

		words[i] = Capital(words[i])
	}

	return strings.Join(words, "")
}
