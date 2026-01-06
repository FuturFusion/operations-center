package provisioning

import (
	"time"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api/images"

	"github.com/FuturFusion/operations-center/shared/api"
)

func WithRandomUUID(randomUUID func() (uuid.UUID, error)) TokenServiceOption {
	return func(s *tokenService) {
		s.randomUUID = randomUUID
	}
}

func (s *tokenService) AddImage(imageUUID uuid.UUID, tokenID uuid.UUID, imageType api.ImageType, architecture images.UpdateFileArchitecture, seedConfig TokenImageSeedConfigs, createdAt time.Time) {
	s.images[imageUUID] = imageRecord{
		TokenID:      tokenID,
		ImageType:    imageType,
		Architecture: architecture,
		SeedConfig:   seedConfig,
		CreatedAt:    createdAt,
	}
}

func (s *tokenService) GetImages() map[uuid.UUID]imageRecord {
	return s.images
}
