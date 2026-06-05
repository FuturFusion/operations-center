package image

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"
	"slices"
	"strings"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/file"
	"github.com/FuturFusion/operations-center/shared/api"
)

type imageIncusService struct {
	repo      ImageIncusRepo
	filesRepo ImageIncusFileRepo
}

var _ ImageIncusService = &imageIncusService{}

func New(repo ImageIncusRepo, filesRepo ImageIncusFileRepo) *imageIncusService {
	service := &imageIncusService{
		repo:      repo,
		filesRepo: filesRepo,
	}

	return service
}

func (s *imageIncusService) AddVersion(ctx context.Context, name string, versionIdentifier string, mr *multipart.Reader) (err error) {
	err = ValidateIncusImageName(name)
	if err != nil {
		return err
	}

	if versionIdentifier == "" {
		return domain.NewValidationErrf("Incus image version can not be empty")
	}

	nameParts := strings.Split(name, ":")

	img, err := s.repo.GetByName(ctx, name)
	if err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("Failed to get incus image %q: %w", name, err)
		}

		img = &IncusImage{
			Name:            name,
			OperatingSystem: nameParts[0],
			Release:         nameParts[1],
			Architecture:    nameParts[2],
			Variant:         nameParts[3],
			Description:     fmt.Sprintf("%s %s (%s) (%s)", nameParts[0], nameParts[1], nameParts[3], nameParts[2]),
			Versions:        make(map[string]api.IncusImageVersion, 1),
		}

		_, err = s.repo.Create(ctx, *img)
		if err != nil {
			return fmt.Errorf("Failed to create incus image %q: %w", name, err)
		}
	}

	_, ok := img.Versions[versionIdentifier]
	if ok {
		return fmt.Errorf("Version %q already exists for incus image %q: %w", versionIdentifier, name, domain.ErrOperationNotPermitted)
	}

	incusImageVersion := api.IncusImageVersion{
		Items: map[string]api.IncusImageVersionItem{},
	}
	for {
		var part *multipart.Part

		part, err = mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return fmt.Errorf("Add version failed to get multipart item: %w", err)
		}

		hash256 := sha256.New()
		partReader := file.NewTeeReadCloser(part, hash256)

		commit, cancel, size, putErr := s.filesRepo.Put(ctx, img, versionIdentifier, part.FileName(), partReader)
		defer func() { //nolint:revive // if any of the file put operations fail, we would want to cancel all of them, so having defer in the loop is what we want.
			cancelErr := cancel()
			if cancelErr != nil {
				err = errors.Join(err, cancelErr)
			}
		}()
		if putErr != nil {
			return fmt.Errorf("Failed to put file %q for image %q, version %q: %w", part.FileName(), name, versionIdentifier, putErr)
		}

		err = commit()
		if err != nil {
			return fmt.Errorf("Add version failed to complete file put for %q of image %q, version %q: %w", part.FileName(), name, versionIdentifier, err)
		}

		ftype := part.FileName()
		var combinedType string
		switch part.FileName() {
		case "root.squashfs":
			ftype = "squashfs"

		case "disk.qcow2":
			ftype = "disk-kvm.img"

		case "incus_combined.tar.gz":
			ftype = "incus_combined.tar.gz"

			combinedType, err = s.detectCombinedImageType(ctx, img, versionIdentifier, part.FileName())
		}

		incusImageVersion.Items[part.FileName()] = api.IncusImageVersionItem{
			FileType:     ftype,
			CombinedType: combinedType,
			Size:         size,
			HashSha256:   hex.EncodeToString(hash256.Sum(nil)),
			Path:         filepath.Join("images", img.Path(), versionIdentifier, part.FileName()),
		}
	}

	err = s.calculateCombinedHashes(ctx, img, versionIdentifier, &incusImageVersion)
	if err != nil {
		return fmt.Errorf("Failed to calculate combined hashes for %q, version %q: %w", name, versionIdentifier, err)
	}

	err = transaction.Do(ctx, func(ctx context.Context) error {
		img, err = s.repo.GetByName(ctx, name)
		if err != nil {
			return fmt.Errorf("Failed to get version %q for incus image %q: %w", versionIdentifier, name, err)
		}

		if img.Versions == nil {
			img.Versions = map[string]api.IncusImageVersion{}
		}

		img.Versions[versionIdentifier] = incusImageVersion

		err = s.repo.Update(ctx, *img)
		if err != nil {
			return fmt.Errorf("Failed to update incus image %q: %w", name, err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

func (s *imageIncusService) detectCombinedImageType(ctx context.Context, img *IncusImage, versionIdentifier string, filename string) (imageType string, err error) {
	rc, _, err := s.filesRepo.Get(ctx, img, versionIdentifier, filename)
	if err != nil {
		return "", err
	}

	defer func() {
		closeErr := rc.Close()
		err = errors.Join(err, closeErr)
	}()

	gzReader, err := gzip.NewReader(rc)
	if err != nil {
		return "", err
	}

	defer func() {
		closeErr := gzReader.Close()
		err = errors.Join(err, closeErr)
	}()

	tarReader := tar.NewReader(gzReader)

	for {
		hdr, err := tarReader.Next()
		if err != nil {
			if err == io.EOF {
				return "", fmt.Errorf("Invalid incus_combined.tar.gzip archive, unable to determine type of combined image")
			}

			return "", err
		}

		if hdr.Name == "rootfs" || hdr.Name == "./rootfs" {
			return "container", nil
		}

		if hdr.Name == "rootfs.img" || hdr.Name == "./rootfs.img" {
			return "virtual-machine", nil
		}
	}
}

func (s *imageIncusService) calculateCombinedHashes(ctx context.Context, img *IncusImage, versionIdentifier string, incusImageVersion *api.IncusImageVersion) error {
	incusTarXZ, ok := incusImageVersion.Items["incus.tar.xz"]
	if !ok {
		_, ok := incusImageVersion.Items["incus_combined.tar.gz"]
		if ok && len(incusImageVersion.Items) == 1 {
			return nil
		}

		return fmt.Errorf(`Incus image version does not have metadata file "incus.tar.xz": %w`, domain.ErrOperationNotPermitted)
	}

	for fileName := range incusImageVersion.Items {
		if !slices.Contains([]string{"root.tar.xz", "root.squashfs", "disk.qcow2"}, fileName) {
			continue
		}

		hash256 := sha256.New()

		r, _, err := s.filesRepo.Get(ctx, img, versionIdentifier, "incus.tar.xz")
		if err != nil {
			return fmt.Errorf("Failed to get file %q from incus image %q, version %q: %w", "incus.tar.xz", img.Name, versionIdentifier, err)
		}

		_, err = file.SafeCopy(hash256, r)
		if err != nil {
			return fmt.Errorf("Failed to read file %q from incus image %q, version %q: %w", fileName, img.Name, versionIdentifier, err)
		}

		r, _, err = s.filesRepo.Get(ctx, img, versionIdentifier, fileName)
		if err != nil {
			return fmt.Errorf("Failed to get file %q from incus image %q, version %q: %w", fileName, img.Name, versionIdentifier, err)
		}

		_, err = file.SafeCopy(hash256, r)
		if err != nil {
			return fmt.Errorf("Failed to read file %q from incus image %q, version %q: %w", fileName, img.Name, versionIdentifier, err)
		}

		switch fileName {
		case "root.tar.xz":
			incusTarXZ.CombinedSha256RootXz = hex.EncodeToString(hash256.Sum(nil))

		case "root.squashfs":
			incusTarXZ.CombinedSha256SquashFs = hex.EncodeToString(hash256.Sum(nil))

		case "disk.qcow2":
			incusTarXZ.CombinedSha256DiskKvmImg = hex.EncodeToString(hash256.Sum(nil))
		}
	}

	incusImageVersion.Items["incus.tar.xz"] = incusTarXZ

	return nil
}

func (s *imageIncusService) GetAll(ctx context.Context) (IncusImages, error) {
	imgs, err := s.repo.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("Failed to get incus images: %w", err)
	}

	return imgs, nil
}

func (s *imageIncusService) GetAllNames(ctx context.Context) ([]string, error) {
	names, err := s.repo.GetAllNames(ctx)
	if err != nil {
		return nil, fmt.Errorf("Failed to get incus image names: %w", err)
	}

	return names, nil
}

func (s *imageIncusService) GetByName(ctx context.Context, name string) (*IncusImage, error) {
	err := ValidateIncusImageName(name)
	if err != nil {
		return nil, err
	}

	img, err := s.repo.GetByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("Failed to get incus image %q: %w", name, err)
	}

	return img, nil
}

func (s *imageIncusService) DeleteByName(ctx context.Context, name string) error {
	err := ValidateIncusImageName(name)
	if err != nil {
		return err
	}

	err = transaction.Do(ctx, func(ctx context.Context) error {
		img, err := s.repo.GetByName(ctx, name)
		if err != nil {
			return fmt.Errorf("Failed to get incus image %q: %w", name, err)
		}

		err = s.repo.DeleteByName(ctx, name)
		if err != nil {
			return fmt.Errorf("Failed to delete incus image %q: %w", name, err)
		}

		err = s.filesRepo.Delete(ctx, img)
		if err != nil {
			return fmt.Errorf("Failed to delete incus image %q: %w", name, err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

func (s *imageIncusService) DeleteVersionByName(ctx context.Context, name string, versionIdentifier string) error {
	err := ValidateIncusImageName(name)
	if err != nil {
		return err
	}

	if versionIdentifier == "" {
		return domain.NewValidationErrf("Incus image version cannot be empty")
	}

	err = transaction.Do(ctx, func(ctx context.Context) error {
		img, err := s.repo.GetByName(ctx, name)
		if err != nil {
			return fmt.Errorf("Failed to get incus image %q: %w", name, err)
		}

		_, ok := img.Versions[versionIdentifier]
		if !ok {
			return fmt.Errorf("Failed to delete version %q from incus image %q: %w", versionIdentifier, name, domain.ErrNotFound)
		}

		delete(img.Versions, versionIdentifier)

		err = s.repo.Update(ctx, *img)
		if err != nil {
			return fmt.Errorf("Failed to delete version %q from incus image %q: %w", versionIdentifier, name, err)
		}

		err = s.filesRepo.DeleteVersion(ctx, img, versionIdentifier)
		if err != nil {
			return fmt.Errorf("Failed to delete version %q from incus image %q: %w", versionIdentifier, name, err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

func (s *imageIncusService) GetVersionFileByName(ctx context.Context, name string, version string, filename string) (_ io.ReadCloser, size int64, _ error) {
	err := ValidateIncusImageName(name)
	if err != nil {
		return nil, 0, err
	}

	if version == "" {
		return nil, 0, domain.NewValidationErrf("Incus image version cannot be empty")
	}

	if filename == "" {
		return nil, 0, domain.NewValidationErrf("Filename cannot be empty")
	}

	img, err := s.repo.GetByName(ctx, name)
	if err != nil {
		return nil, 0, fmt.Errorf("Failed to get incus image %q: %w", name, err)
	}

	rc, size, err := s.filesRepo.Get(ctx, img, version, filename)
	if err != nil {
		return nil, 0, fmt.Errorf("Failed get file %q for image %q and version %q: %w", filename, name, version, err)
	}

	return rc, size, nil
}
