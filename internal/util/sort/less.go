package sort

import (
	"github.com/fvbommel/sortorder"
)

func NaturalLess(str1 string, str2 string) bool {
	if str1 == "" {
		return false
	}

	if str2 == "" {
		return true
	}

	return sortorder.NaturalLess(str1, str2)
}

func StringLess(str1 string, str2 string) bool {
	return str1 < str2
}
