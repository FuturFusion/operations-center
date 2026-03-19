package api

import (
	"github.com/lxc/incus-os/incus-osd/api"
	"github.com/lxc/incus-os/incus-osd/api/seed"
)

type (
	SeedApplications = seed.Applications
	SeedApplication  = seed.Application

	SeedIncus = seed.Incus

	SeedInstall = seed.Install

	SeedMigrationManager = seed.MigrationManager

	SeedNetwork = seed.Network

	SeedOperationsCenter = seed.OperationsCenter

	SeedUpdate       = seed.Update
	SeedUpdateConfig = api.SystemUpdateConfig
)
