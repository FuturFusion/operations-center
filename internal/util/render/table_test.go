package render_test

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/render"
	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
)

func TestTable(t *testing.T) {
	t.Parallel()

	header := []string{"NAME", "STATUS"}
	data := [][]string{{"one", "ready"}, {"two", "pending"}}
	raw := []map[string]string{{"name": "one"}}

	tests := []struct {
		name   string
		format string

		assertErr require.ErrorAssertionFunc
		want      string
	}{
		{
			name:   "table",
			format: render.TableFormatTable,

			assertErr: require.NoError,
			want: "+------+---------+\n" +
				"| NAME | STATUS  |\n" +
				"+------+---------+\n" +
				"| one  | ready   |\n" +
				"+------+---------+\n" +
				"| two  | pending |\n" +
				"+------+---------+\n",
		},
		{
			name:   "table without header",
			format: render.TableFormatTable + "," + render.TableOptionNoHeader,

			assertErr: require.NoError,
			want: "+-----+---------+\n" +
				"| one | ready   |\n" +
				"+-----+---------+\n" +
				"| two | pending |\n" +
				"+-----+---------+\n",
		},
		{
			name:   "compact",
			format: render.TableFormatCompact,

			assertErr: require.NoError,
			want: " NAME  STATUS  \n" +
				" one   ready   \n" +
				" two   pending \n",
		},
		{
			name:   "compact without header",
			format: render.TableFormatCompact + "," + render.TableOptionNoHeader,

			assertErr: require.NoError,
			want: " one  ready   \n" +
				" two  pending \n",
		},
		{
			name:   "csv without header by default",
			format: render.TableFormatCSV,

			assertErr: require.NoError,
			want:      "one,ready\ntwo,pending\n",
		},
		{
			name:   "csv with header",
			format: render.TableFormatCSV + "," + render.TableOptionHeader,

			assertErr: require.NoError,
			want:      "NAME,STATUS\none,ready\ntwo,pending\n",
		},
		{
			name:   "json renders raw",
			format: render.TableFormatJSON,

			assertErr: require.NoError,
			want:      "[{\"name\":\"one\"}]\n",
		},
		{
			name:   "yaml renders raw",
			format: render.TableFormatYAML,

			assertErr: require.NoError,
			want:      "- name: one\n",
		},
		{
			name:   "error - header and noheader",
			format: render.TableFormatCSV + "," + render.TableOptionHeader + "," + render.TableOptionNoHeader,

			assertErr: errassert.Contains(`Format options "header" and "noheader" are mutually exclusive`),
			want:      "",
		},
		{
			name:   "error - invalid format",
			format: "xml",

			assertErr: errassert.Contains(`Invalid format "xml"`),
			want:      "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			var buf strings.Builder

			// Run test
			err := render.Table(&buf, tc.format, header, data, raw)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.want, buf.String(), "Unexpected rendered output")
		})
	}
}

func TestTable_nilWriter(t *testing.T) {
	t.Parallel()

	// Setup
	var w io.Writer

	// Run test
	err := render.Table(w, render.TableFormatTable, []string{"NAME"}, [][]string{{"one"}}, nil)

	// Assert
	require.ErrorContains(t, err, "Unable to render to nil writer")
}
