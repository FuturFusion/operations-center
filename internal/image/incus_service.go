package image

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"mime/multipart"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	incusapi "github.com/lxc/incus/v7/shared/api"
	"go.yaml.in/yaml/v4"

	"github.com/expr-lang/expr"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/archive/xz"
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

func (s *imageIncusService) AddVersion(ctx context.Context, mr *multipart.Reader) (_ string, err error) {
	var (
		part          *multipart.Part
		imageMetadata incusapi.ImageMetadata
		incusTarXZ    []byte
	)

	part, err = mr.NextPart()
	if err != nil {
		return "", fmt.Errorf("Add version failed to get first multipart item: %w", err)
	}

	switch {
	case part.FormName() == "request_json":
		imageMetadata, incusTarXZ, err = metadataFromRequestJSON(ctx, part)

	case part.FileName() == "incus.tar.xz":
		imageMetadata, incusTarXZ, err = metadataFromIncusTarXZ(ctx, part)

	default:
		return "", fmt.Errorf(`First part of the multipart request is required to be either "request_json" or the file "incus.tar.xz", got form-name %q, filename %q: %w`, part.FormName(), part.FileName(), domain.ErrOperationNotPermitted)
	}

	if err != nil {
		return "", fmt.Errorf(`Failed to read metadata: %w`, err)
	}

	os := imageMetadata.Properties["os"]
	release := imageMetadata.Properties["release"]
	architecture := fixArchitectureMapping(imageMetadata.Properties["architecture"])
	variant := imageMetadata.Properties["variant"]
	versionIdentifier := imageMetadata.Properties["serial"]
	description := imageMetadata.Properties["description"]

	if os == "" || architecture == "" || versionIdentifier == "" {
		return "", domain.NewValidationErrf(`Incomplete metadata, not all required properties set os=%q, architecture=%q, version=%q`, os, architecture, versionIdentifier)
	}

	err = ValidateIncusImageVersion(versionIdentifier)
	if err != nil {
		return "", fmt.Errorf("Invalid incus image version: %w", err)
	}

	if release == "" {
		release = "current"
	}

	if variant == "" {
		variant = "default"
	}

	name := strings.Join([]string{os, release, architecture, variant}, ":")

	img, err := s.repo.GetByName(ctx, name)
	if err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			return "", fmt.Errorf("Failed to get incus image %q: %w", name, err)
		}

		img = &IncusImage{
			Name:            name,
			Aliases:         []string{fmt.Sprintf("%s/%s/%s", os, release, variant)},
			OperatingSystem: os,
			Release:         release,
			Architecture:    architecture,
			Variant:         variant,
			Description:     description,
			Versions:        make(map[string]api.IncusImageVersion, 1),
		}

		_, err = s.repo.Create(ctx, *img)
		if err != nil {
			return "", fmt.Errorf("Failed to create incus image %q: %w", name, err)
		}
	}

	_, ok := img.Versions[versionIdentifier]
	if ok {
		return "", fmt.Errorf("Version %q already exists for incus image %q: %w", versionIdentifier, name, domain.ErrOperationNotPermitted)
	}

	incusImageVersion := api.IncusImageVersion{
		Items: map[string]api.IncusImageVersionItem{},
	}

	hash256 := sha256.New()
	incusTarXZReader := file.NewTeeReadCloser(io.NopCloser(bytes.NewReader(incusTarXZ)), hash256)

	commit, cancel, size, putErr := s.filesRepo.Put(ctx, img, versionIdentifier, "incus.tar.xz", incusTarXZReader)
	defer func() { //nolint:revive // if any of the file put operations fail, we would want to cancel all of them, so having defer in the loop is what we want.
		cancelErr := cancel()
		if cancelErr != nil {
			err = errors.Join(err, cancelErr)
		}
	}()
	if putErr != nil {
		return "", fmt.Errorf(`Failed to put file "incus.tar.xz" for image %q, version %q: %w`, name, versionIdentifier, putErr)
	}

	err = commit()
	if err != nil {
		return "", fmt.Errorf("Add version failed to complete file put for %q of image %q, version %q: %w", part.FileName(), name, versionIdentifier, err)
	}

	incusImageVersion.Items["incus.tar.xz"] = api.IncusImageVersionItem{
		FileType:   "incus.tar.xz",
		Size:       size,
		HashSha256: hex.EncodeToString(hash256.Sum(nil)),
		Path:       filepath.Join("images", img.Path(), versionIdentifier, "incus.tar.xz"),
	}

	for {
		part, err = mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return "", fmt.Errorf("Add version failed to get multipart item: %w", err)
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
			return "", fmt.Errorf("Failed to put file %q for image %q, version %q: %w", part.FileName(), name, versionIdentifier, putErr)
		}

		err = commit()
		if err != nil {
			return "", fmt.Errorf("Add version failed to complete file put for %q of image %q, version %q: %w", part.FileName(), name, versionIdentifier, err)
		}

		ftype := part.FileName()
		switch part.FileName() {
		case "root.squashfs":
			ftype = "squashfs"

		case "disk.qcow2":
			ftype = "disk-kvm.img"
		}

		incusImageVersion.Items[part.FileName()] = api.IncusImageVersionItem{
			FileType:   ftype,
			Size:       size,
			HashSha256: hex.EncodeToString(hash256.Sum(nil)),
			Path:       filepath.Join("images", img.Path(), versionIdentifier, part.FileName()),
		}
	}

	err = s.calculateCombinedHashes(ctx, img, versionIdentifier, &incusImageVersion)
	if err != nil {
		return "", fmt.Errorf("Failed to calculate combined hashes for %q, version %q: %w", name, versionIdentifier, err)
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
		return "", err
	}

	return name, nil
}

func metadataFromRequestJSON(ctx context.Context, part *multipart.Part) (_ incusapi.ImageMetadata, incusTarXZ []byte, err error) {
	var requestMetadata api.IncusImagePost

	err = json.NewDecoder(part).Decode(&requestMetadata)
	if err != nil {
		return incusapi.ImageMetadata{}, nil, fmt.Errorf(`Failed to decode metadata from "request_json": %w`, err)
	}

	imageMetadata := incusapi.ImageMetadata{
		Architecture: requestMetadata.Architecture,
		CreationDate: time.Now().Unix(),
		Properties: map[string]string{
			"os":           requestMetadata.OperatingSystem,
			"release":      requestMetadata.Release,
			"architecture": requestMetadata.Architecture,
			"variant":      requestMetadata.Variant,
			"serial":       requestMetadata.Version,
			"description":  fmt.Sprintf("%s %s (%s) (%s)", requestMetadata.OperatingSystem, requestMetadata.Release, requestMetadata.Variant, requestMetadata.Architecture),
		},
	}

	buf := &bytes.Buffer{}
	metadataBody, err := yaml.Marshal(imageMetadata)
	if err != nil {
		return incusapi.ImageMetadata{}, nil, fmt.Errorf(`Failed to yaml encode "metadata.yaml": %w`, err)
	}

	incusTarXZWriter, err := xz.NewWriter(ctx, buf)
	if err != nil {
		return incusapi.ImageMetadata{}, nil, fmt.Errorf("Failed to create XZ writer: %w", err)
	}

	defer func() {
		err = errors.Join(err, incusTarXZWriter.Close())
	}()

	tarWriter := tar.NewWriter(incusTarXZWriter)
	err = tarWriter.WriteHeader(&tar.Header{
		Name: "metadata.yaml",
		Size: int64(len(metadataBody)),
		Mode: 0o600,
	})
	if err != nil {
		return incusapi.ImageMetadata{}, nil, fmt.Errorf(`Failed to write header for "metadata.yaml" to incus.tar.xz: %w`, err)
	}

	defer func() {
		err = errors.Join(err, tarWriter.Close())
	}()

	_, err = tarWriter.Write(metadataBody)
	if err != nil {
		return incusapi.ImageMetadata{}, nil, fmt.Errorf(`Failed to write "metadata.yaml" to incus.tar.xz: %w`, err)
	}

	return imageMetadata, buf.Bytes(), nil
}

func metadataFromIncusTarXZ(ctx context.Context, part *multipart.Part) (_ incusapi.ImageMetadata, incusTarXZ []byte, err error) {
	const incusTarXZSizeLimit = 64 * 1024
	r := io.LimitReader(part, incusTarXZSizeLimit)
	buf := &bytes.Buffer{}
	r = io.TeeReader(r, buf)

	xzReader, err := xz.NewReader(ctx, r)
	if err != nil {
		return incusapi.ImageMetadata{}, nil, fmt.Errorf(`Failed to create xz reader for "incus.tar.xz": %w`, err)
	}

	tr := tar.NewReader(xzReader)

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return incusapi.ImageMetadata{}, nil, fmt.Errorf(`Failed to find metadata.yaml in incus.tar.xz: %w`, domain.ErrConstraintViolation)
		}

		if err != nil {
			return incusapi.ImageMetadata{}, nil, fmt.Errorf(`Failed to read "incus.tar.xz": %w`, err)
		}

		if hdr.Name != "metadata.yaml" {
			continue
		}

		var imageMetadata incusapi.ImageMetadata
		err = yaml.NewDecoder(tr).Decode(&imageMetadata)
		if err != nil {
			return incusapi.ImageMetadata{}, nil, fmt.Errorf(`Failed to decode "metadata.yaml": %w`, err)
		}

		return imageMetadata, buf.Bytes(), nil
	}
}

func (s *imageIncusService) calculateCombinedHashes(ctx context.Context, img *IncusImage, versionIdentifier string, incusImageVersion *api.IncusImageVersion) error {
	incusTarXZ := incusImageVersion.Items["incus.tar.xz"]

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
	err := ValidateIncusImageName(sourceName)
	if err != nil {
		return err
	}

	err = transaction.Do(ctx, func(ctx context.Context) error {
		images, err := s.repo.GetAllWithFilter(ctx, IncusImageFilter{
			Source: ptr.To(sourceName),
		})
		if err != nil {
			return fmt.Errorf("Failed to get incus images for source %q: %w", sourceName, err)
		}

		for _, img := range images {
			err := s.filesRepo.Delete(ctx, &img)
			if err != nil {
				return fmt.Errorf("Failed to remove files for image %q from source %q: %w", img.Name, sourceName, err)
			}

			err = s.repo.DeleteByName(ctx, img.Name)
			if err != nil {
				return fmt.Errorf("Failed to delete image %q (source %q): %w", img.Name, sourceName, err)
			}
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
func (s *imageIncusService) RefreshFromSource(ctx context.Context, source IncusImageSource) error {
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
	source            IncusImageSource
	image             *IncusImage
	versionIdentifier string
	filename          string
	file              api.IncusImageVersionItem
}

func (s *imageIncusService) downloadFile(ctx context.Context, item downloadItem) (err error) {
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
			return fmt.Errorf("Image file sha256 mismatch for image %q, version %q, file %q from source %q: manifest: %s, actual: %s", item.image.Name, item.versionIdentifier, item.filename, item.source.Name, item.file.HashSha256, checksum)
		}
	}

	return commit()
}
