package ptr_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/ptr"
)

func TestClone_string(t *testing.T) {
	original := new("string")
	clone, err := ptr.Clone(original)
	require.NoError(t, err)

	require.Equal(t, original, clone)

	*original = "new string"

	require.NotEqual(t, original, clone)
}

func TestClone_struct(t *testing.T) {
	type somestruct struct {
		Int    int
		String string
		Nested struct {
			Int int
			Ptr *int
		}

		Ptr *int
	}

	original := new(somestruct{
		Int:    1,
		String: "str",
		Nested: struct {
			Int int
			Ptr *int
		}{
			Int: 2,
			Ptr: new(20),
		},
		Ptr: new(10),
	})
	clone, err := ptr.Clone(original)
	require.NoError(t, err)

	require.Equal(t, original, clone)

	original.Ptr = new(50)

	require.NotEqual(t, original, clone)
}

func TestClone_error_marshal(t *testing.T) {
	original := new(func() {})
	_, err := ptr.Clone(original)
	require.Error(t, err)
}

func TestClone_error_unmarshal(t *testing.T) {
	original := new(errors.New("err"))
	_, err := ptr.Clone(original)
	require.Error(t, err)
}
