package image

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"mime/multipart"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/expr-lang/expr"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/expropts"
	"github.com/FuturFusion/operations-center/internal/util/file"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
	"github.com/FuturFusion/operations-center/shared/api"
)

type imageIncusService struct {
	repo          ImageIncusRepo
	filesRepo     ImageIncusFileRepo
	simplestreams SimplestreamsPort
}

var _ ImageIncusService = &imageIncusService{}

func NewIncusImage(repo ImageIncusRepo, filesRepo ImageIncusFileRepo, simplestreams SimplestreamsPort) *imageIncusService {
	service := &imageIncusService{
		repo:          repo,
		filesRepo:     filesRepo,
		simplestreams: simplestreams,
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
	os, release, architecture, variant := nameParts[0], nameParts[1], nameParts[2], nameParts[3]

	img, err := s.repo.GetByName(ctx, name)
	if err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("Failed to get incus image %q: %w", name, err)
		}

		img = &IncusImage{
			Name:            name,
			Aliases:         []string{fmt.Sprintf("%s/%s/%s/%s", os, release, variant, architecture)},
			OperatingSystem: os,
			Release:         release,
			Architecture:    architecture,
			Variant:         variant,
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

func (s *imageIncusService) DeleteBySource(ctx context.Context, sourceName string) error {
	panic("// FIXME: not implemented")
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

func (s *imageIncusService) Update(ctx context.Context, incusImage IncusImage) error {
	err := incusImage.Validate()
	if err != nil {
		return fmt.Errorf("Failed to validate incus image %q for update: %w", incusImage.Name, err)
	}

	err = s.repo.Update(ctx, incusImage)
	if err != nil {
		return fmt.Errorf("Failed to update incus image %q: %w", incusImage.Name, err)
	}

	return nil
}

func (s *imageIncusService) ValidateFilterExpression(ctx context.Context, filterExpression string) error {
	if filterExpression != "" {
		_, err := expr.Compile(
			filterExpression,
			expr.Env(ExprIncusImageVersionFile{}),
			expr.AsBool(),
			expr.Patch(expropts.UnderlyingBaseTypePatcher{}),
		)
		if err != nil {
			return domain.NewValidationErrf(`Invalid config, failed to compile filter expression: %v`, err)
		}
	}

	return nil
}

// RefreshFromSource refreshes the images from a given Incus image source.
//
// This operations is performed in the following steps:
//
//   - Get latest list of images from the provided source.
//   - Get all existing images for this source from the DB.
//   - Merge the two sets such that only images are downloaded, which pass the filter
//     and are not yet present. Existing images might be updated (e.g. newer versions,
//     missing files) in this process.
//     Obsolete or supernumerous images, versions and files are removed.
//   - Remove the images, version or files, which are marked for removal.
//   - Download the images, version or files, that are part of the resulting state
//     and not yet present on the system.
func (s *imageIncusService) RefreshFromSource(ctx context.Context, source ImageSource) error {
	originImages, err := s.simplestreams.GetImageList(ctx, source)
	if err != nil {
		return fmt.Errorf("Failed to refresh images from source %q: %w", source.Name, err)
	}

	originImages, err = filterImageVersionFilesByFilterExpression(originImages, source.FilterExpression)
	if err != nil {
		return fmt.Errorf("Failed to filter images from source %q: %w", source.Name, err)
	}

	dbImages, err := s.repo.GetAllWithFilter(ctx, IncusImageFilter{
		Source: ptr.To(source.Name),
	})
	if err != nil {
		return fmt.Errorf("Failed to get images from DB for source %q: %w", source.Name, err)
	}

	imagesToDeleteFromDB := determineImagesToDeleteFromDB(originImages, dbImages)
	for _, image := range imagesToDeleteFromDB {
		err = s.repo.DeleteByName(ctx, image.Name)
		if err != nil {
			return fmt.Errorf("Failed to remove obsolete image %q from DB: %w", image.Name, err)
		}
	}

	originImageVersionFileLookup := getImageVersionFileLookup(originImages)
	dbImageVersionFileLookup := getImageVersionFileLookup(dbImages)

	// Delete supernoumerous files.
	var errs []error
	for _, image := range dbImages {
		for versionIdentifier, version := range image.Versions {
			for filename := range version.Items {
				if originImageVersionFileLookup[imageVersionFileIdentifier(image, versionIdentifier, filename)] {
					continue
				}

				err := s.filesRepo.DeleteVersionFile(ctx, &image, versionIdentifier, filename)
				if err != nil {
					errs = append(errs, err)
					continue
				}
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("Failed to remove supernoumerous images from source %q: %w", source.Name, errors.Join(errs...))
	}

	// Iterate images, check if files exist, download if not.
syncImages:
	for _, image := range originImages {
		// Make sure, we do have enough space left in the files repository before downloading the files.
		err = s.isSpaceAvailable(ctx, image)
		if err != nil {
			errs = append(errs, err)
			break syncImages
		}

		for versionIdentifier, version := range image.Versions {
			for filename, fileItem := range version.Items {
				if dbImageVersionFileLookup[imageVersionFileIdentifier(image, versionIdentifier, filename)] {
					continue
				}

				downloadItem := downloadItem{
					source:            source,
					image:             &image,
					versionIdentifier: versionIdentifier,
					filename:          filename,
					file:              fileItem,
				}
				err = s.downloadFile(ctx, downloadItem)
				if err != nil {
					errs = append(errs, err)
					continue
				}
			}

			if len(errs) >= 10 {
				break syncImages
			}
		}

		err := s.repo.Upsert(ctx, image)
		if err != nil {
			errs = append([]error{err}, errs...)
			return fmt.Errorf("Failed to update image %q in DB: %w", image.Name, errors.Join(errs...))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("Failed to sync from source %q: %w", source.Name, errors.Join(errs...))
	}

	return nil
}

func filterImageVersionFilesByFilterExpression(images IncusImages, filterExpression string) (IncusImages, error) {
	// No filter set defaults to filter everything to prevent users from accidentally downloading
	// huge number of files unnecessarily. If a user wants do download everything on purpose,
	// the filter expression can be set to "true".
	if filterExpression == "" {
		return IncusImages{}, nil
	}

	filterExpressionProgram, err := expr.Compile(
		filterExpression,
		expr.Env(ExprIncusImageVersionFile{}),
		expr.AsBool(),
		expr.Patch(expropts.UnderlyingBaseTypePatcher{}),
	)
	if err != nil {
		return nil, err
	}

	n := 0
	for i, image := range images {
		for versionIdentifier, version := range image.Versions {
			for filename, fileItem := range version.Items {
				exprIncusImageVersionFile := ExprIncusImageVersionFile{
					Name:            image.Name,
					OperatingSystem: image.OperatingSystem,
					Release:         image.Release,
					Architecture:    image.Architecture,
					Variant:         image.Variant,
					Version:         versionIdentifier,
					Filename:        filename,
					FileType:        fileItem.FileType,
					Size:            fileItem.Size,
				}

				result, err := expr.Run(filterExpressionProgram, exprIncusImageVersionFile)
				if err != nil {
					return nil, err
				}

				if !result.(bool) {
					delete(images[i].Versions[versionIdentifier].Items, filename)
				}
			}

			if len(images[i].Versions[versionIdentifier].Items) == 0 {
				delete(images[i].Versions, versionIdentifier)
			}
		}

		if len(images[i].Versions) == 0 {
			continue
		}

		images[n] = images[i]
		n++
	}

	return images[:n], nil
}

func determineImagesToDeleteFromDB(originImages IncusImages, dbImages IncusImages) IncusImages {
	originImageNames := make(map[string]bool, len(originImages))
	for _, image := range originImages {
		originImageNames[image.Name] = true
	}

	imagesToDeleteFromDB := make(IncusImages, 0, len(dbImages))
	for _, image := range dbImages {
		if originImageNames[image.Name] {
			continue
		}

		imagesToDeleteFromDB = append(imagesToDeleteFromDB, image)
	}

	return imagesToDeleteFromDB
}

func getImageVersionFileLookup(images IncusImages) map[string]bool {
	lookup := map[string]bool{}
	for _, image := range images {
		for versionIdentifier, version := range image.Versions {
			for filename := range version.Items {
				lookup[imageVersionFileIdentifier(image, versionIdentifier, filename)] = true
			}
		}
	}

	return lookup
}

func imageVersionFileIdentifier(image IncusImage, versionIdentifier string, filename string) string {
	return path.Join(image.Path(), versionIdentifier, filename)
}

func (s *imageIncusService) isSpaceAvailable(ctx context.Context, downloadImage IncusImage) error {
	var requiredSpaceTotal int64
	for _, version := range downloadImage.Versions {
		for _, fileItem := range version.Items {
			requiredSpaceTotal += fileItem.Size
		}
	}

	ui, err := s.filesRepo.UsageInformation(ctx)
	if err != nil {
		return fmt.Errorf("Failed to get usage information: %w", err)
	}

	if ui.TotalSpaceBytes < 1 {
		return fmt.Errorf("Files repository reported an invalid total space: %d", ui.TotalSpaceBytes)
	}

	if (float64(ui.AvailableSpaceBytes)-float64(requiredSpaceTotal))/float64(ui.TotalSpaceBytes) < 0.1 {
		return fmt.Errorf("Not enough space available in files repository, require: %d, available: %d, required headroom after download: 10%%", requiredSpaceTotal, ui.AvailableSpaceBytes)
	}

	return nil
}

type downloadItem struct {
	source            ImageSource
	image             *IncusImage
	versionIdentifier string
	filename          string
	file              api.IncusImageVersionItem
}

func (s *imageIncusService) downloadFile(ctx context.Context, item downloadItem) error {
	stream, err := s.simplestreams.GetFile(ctx, item.source, item.file.Path)
	if err != nil {
		return fmt.Errorf(`Failed to fetch image %q, version %q, file %q from source %q: %w`, item.image.Name, item.versionIdentifier, item.filename, item.source.Name, err)
	}

	teeStream := stream
	var h hash.Hash

	if item.file.HashSha256 != "" {
		h = sha256.New()
		teeStream = file.NewTeeReadCloser(stream, h)
	}

	commit, cancel, _, err := s.filesRepo.Put(ctx, item.image, item.versionIdentifier, item.filename, teeStream)
	defer func() {
		cancelErr := cancel()
		if cancelErr != nil {
			err = errors.Join(err, cancelErr)
		}
	}()
	if err != nil {
		return fmt.Errorf(`Failed to save stream for image %q, version %q, file %q from source %q: %w`, item.image.Name, item.versionIdentifier, item.filename, item.source.Name, err)
	}

	if item.file.HashSha256 != "" {
		checksum := hex.EncodeToString(h.Sum(nil))
		if item.file.HashSha256 != checksum {
			return fmt.Errorf("Image file sha256 mismatch for image %q, version %q, file %q from source %q: manifest %s, actual: %s", item.image.Name, item.versionIdentifier, item.filename, item.source.Name, item.file.HashSha256, checksum)
		}
	}

	return commit()
}
