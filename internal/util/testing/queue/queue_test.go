package queue_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/queue"
)

func TestPop(t *testing.T) {
	tests := []struct {
		name  string
		items []queue.Item[int]

		wantFatal                bool
		assertErr                require.ErrorAssertionFunc
		want                     int
		wantRemainingQueueLength int
	}{
		{
			name:  "empty",
			items: []queue.Item[int]{},

			wantFatal: true,
			assertErr: require.NoError,
		},
		{
			name: "one item",
			items: []queue.Item[int]{
				{
					Value: 1,
					Err:   boom.Error,
				},
			},

			wantFatal: false,
			assertErr: boom.ErrorIs,
			want:      1,
		},
		{
			name: "multiple items",
			items: []queue.Item[int]{
				{
					Value: 1,
				},
				{
					Value: 2,
					Err:   boom.Error,
				},
			},

			wantFatal:                false,
			want:                     1,
			assertErr:                require.NoError,
			wantRemainingQueueLength: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tt := &mockT{}

			var item int
			var err error
			func() {
				defer func() {
					_ = recover()
				}()

				item, err = queue.Pop(tt, &tc.items)
			}()

			require.Equal(t, tc.wantFatal, tt.failed)
			tc.assertErr(t, err)
			require.Equal(t, tc.want, item)
			require.Len(t, tc.items, tc.wantRemainingQueueLength)
		})
	}
}

func TestPopRetainLast(t *testing.T) {
	tests := []struct {
		name  string
		items []queue.Item[int]

		wantFatal                bool
		assertErr                require.ErrorAssertionFunc
		want                     int
		wantRemainingQueueLength int
	}{
		{
			name:  "empty",
			items: []queue.Item[int]{},

			wantFatal: true,
			assertErr: require.NoError,
		},
		{
			name: "one item",
			items: []queue.Item[int]{
				{
					Value: 1,
					Err:   boom.Error,
				},
			},

			wantFatal:                false,
			assertErr:                boom.ErrorIs,
			want:                     1,
			wantRemainingQueueLength: 1,
		},
		{
			name: "multiple items",
			items: []queue.Item[int]{
				{
					Value: 1,
				},
				{
					Value: 2,
					Err:   boom.Error,
				},
			},

			wantFatal:                false,
			want:                     2,
			assertErr:                boom.ErrorIs,
			wantRemainingQueueLength: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tt := &mockT{}

			var item int
			var err error
			func() {
				defer func() {
					_ = recover()
				}()

				_, _ = queue.PopRetainLast(tt, &tc.items)
				item, err = queue.PopRetainLast(tt, &tc.items)
			}()

			require.Equal(t, tc.wantFatal, tt.failed)
			tc.assertErr(t, err)
			require.Equal(t, tc.want, item)
			require.Len(t, tc.items, tc.wantRemainingQueueLength)
		})
	}
}

type mockT struct {
	failed bool
}

func (m mockT) Helper() {}
func (m *mockT) Fatal(args ...any) {
	m.failed = true
	panic("fatal")
}
