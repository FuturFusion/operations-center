package structs_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/structs"
)

type TestStruct struct {
	Str    string
	Number int
	Slice  []string
	Map    map[string]int
}

func TestDeepCopy_Success(t *testing.T) {
	src := TestStruct{
		Str:    "foobar",
		Number: 10,
		Slice:  []string{"foo", "bar"},
		Map:    map[string]int{"value": 10},
	}

	var dst TestStruct

	err := structs.DeepCopy(src, &dst)
	require.NoError(t, err)
	require.Equal(t, src, dst)

	// Ensure deep copy (mutating src should not affect dst)
	src.Slice[0] = "changed"
	src.Map["value"] = 999

	require.NotEqual(t, src.Slice, dst.Slice)
	require.NotEqual(t, src.Map, dst.Map)
}

func TestDeepCopy_EncodeError(t *testing.T) {
	type Bad struct {
		C chan int
	}

	src := Bad{C: make(chan int)}
	var dst Bad

	err := structs.DeepCopy(src, &dst)
	require.Error(t, err)
}

func TestDeepCopy_DecodeTypeMismatch(t *testing.T) {
	src := TestStruct{Str: "foobar"}
	var dst struct {
		Foo string
	}

	err := structs.DeepCopy(src, &dst)
	require.Error(t, err)
}

func TestDeepCopy_DecodeNonPointer(t *testing.T) {
	src := TestStruct{Str: "foobar"}
	var dst TestStruct

	err := structs.DeepCopy(src, dst) // not a pointer
	require.Error(t, err)
}
