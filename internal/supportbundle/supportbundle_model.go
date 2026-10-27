package supportbundle

import (
	"strings"
	"time"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/shared/api"
)

// manifestFormatVersion is raised, if the layout of the bundle changes incompatibly.
const manifestFormatVersion = 1

func validateRequest(req api.SupportBundlePost) error {
	_, ok := api.SupportBundleTypes[req.Type]
	if !ok {
		return domain.NewValidationErrf("Invalid support bundle, type %q is not one of cluster, project, instance or server", req.Type)
	}

	fields := []struct {
		name     string
		value    string
		required bool
	}{
		{name: "cluster", value: req.Cluster, required: req.Type != api.SupportBundleTypeServer},
		{name: "project", value: req.Project, required: req.Type == api.SupportBundleTypeProject || req.Type == api.SupportBundleTypeInstance},
		{name: "name", value: req.Name, required: req.Type == api.SupportBundleTypeInstance},
		{name: "server", value: req.Server, required: req.Type == api.SupportBundleTypeServer},
	}

	for _, field := range fields {
		if field.required && field.value == "" {
			return domain.NewValidationErrf("Invalid support bundle, %q is required for type %q", field.name, req.Type)
		}

		if !field.required && field.value != "" {
			return domain.NewValidationErrf("Invalid support bundle, %q is not supported for type %q", field.name, req.Type)
		}

		// The values are used as paths within the archive.
		if field.value == "." || field.value == ".." || strings.ContainsAny(field.value, `/\`) {
			return domain.NewValidationErrf("Invalid support bundle, %s %q is not a valid name", field.name, field.value)
		}
	}

	return nil
}

// scope holds the resources from the database, a support bundle is created for.
type scope struct {
	cluster        *provisioning.Cluster
	clusterServers provisioning.Servers
	server         *provisioning.Server

	// instanceLocation is the cluster member hosting the instance, if known upfront.
	instanceLocation string
}

type sectionStatus string

const (
	sectionStatusOK        sectionStatus = "ok"
	sectionStatusError     sectionStatus = "error"
	sectionStatusSkipped   sectionStatus = "skipped"
	sectionStatusTruncated sectionStatus = "truncated"
)

// manifest describes the content of a support bundle, it is its last entry.
type manifest struct {
	FormatVersion int                   `json:"format_version"`
	Version       string                `json:"operations_center_version"`
	CreatedAt     time.Time             `json:"created_at"`
	DurationMS    int64                 `json:"duration_ms"`
	Request       api.SupportBundlePost `json:"request"`
	Sections      []manifestSection     `json:"sections"`
	Notes         []string              `json:"notes,omitempty"`
}

type manifestSection struct {
	Path       string        `json:"path"`
	Status     sectionStatus `json:"status"`
	Error      string        `json:"error,omitempty"`
	DurationMS int64         `json:"duration_ms"`
}
