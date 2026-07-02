package terraform

import (
	"reflect"
	"strings"
)

// hclStringReplacer escapes a string for use in a HCL quoted string.
var hclStringReplacer = strings.NewReplacer(
	`\`, `\\`,
	`"`, `\"`,
	"\n", `\n`,
	"\r", `\r`,
	"\t", `\t`,
	"${", "$${",
	"%{", "%%{",
)

func tfString(s string) string {
	return hclStringReplacer.Replace(s)
}

func maxKeyLength(m any) int {
	v := reflect.ValueOf(m)

	if v.Kind() != reflect.Map {
		return 0
	}

	if v.Type().Key().Kind() != reflect.String {
		return 0
	}

	maxLen := 0
	for _, key := range v.MapKeys() {
		maxLen = max(maxLen, len(key.String()))
	}

	return maxLen
}
