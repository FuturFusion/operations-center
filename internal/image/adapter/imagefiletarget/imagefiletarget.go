// Package imagefiletarget lets other cluster members manage the image files they store.
package imagefiletarget

import (
	"context"
	"path/filepath"

	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/cluster/memberfiles"
	"github.com/FuturFusion/operations-center/internal/image"
)

type imageFileTarget struct {
	targets memberfiles.Targets
}

var _ image.ImageFileTargetPort = imageFileTarget{}

// New returns access to the members storing image files, found through targets.
func New(targets memberfiles.Targets) imageFileTarget {
	return imageFileTarget{targets: targets}
}

func (i imageFileTarget) LocalMemberID(ctx context.Context) (int64, error) {
	return i.targets.LocalID()
}

func (i imageFileTarget) SyncImage(ctx context.Context, source image.IncusImageSource, img image.IncusImage) error {
	return i.targets.Do(ctx, img.MemberID, func(ctx context.Context, c client.OperationsCenterClient) error {
		return c.SyncMemberImage(ctx, source.Name, img.Name, img)
	})
}

func (i imageFileTarget) DeleteFiles(ctx context.Context, img image.IncusImage) error {
	return i.targets.Do(ctx, img.MemberID, func(ctx context.Context, c client.OperationsCenterClient) error {
		return c.DeleteMemberFile(ctx, memberfiles.StoreImages, filepath.ToSlash(img.FilePath()))
	})
}
