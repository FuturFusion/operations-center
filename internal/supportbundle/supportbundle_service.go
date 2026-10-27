package supportbundle

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	incusapi "github.com/lxc/incus/v7/shared/api"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/version"
	"github.com/FuturFusion/operations-center/shared/api"
)

const (
	bundleTimeout      = 10 * time.Minute
	probeTimeout       = 10 * time.Second
	existsTimeout      = 10 * time.Second
	sectionTimeout     = 30 * time.Second
	journalTimeout     = 60 * time.Second
	incusLogsTimeout   = 3 * time.Minute
	journalEntries     = 5000
	instanceLogLimit   = 16 << 20
	incusLogsSizeLimit = 512 << 20
)

type supportBundleService struct {
	clusterSvc ProvisioningClusterService
	serverSvc  ProvisioningServerService
	client     SupportBundleClientPort
	now        func() time.Time
}

var _ SupportBundleService = &supportBundleService{}

func New(clusterSvc ProvisioningClusterService, serverSvc ProvisioningServerService, client SupportBundleClientPort) *supportBundleService {
	return &supportBundleService{
		clusterSvc: clusterSvc,
		serverSvc:  serverSvc,
		client:     client,
		now:        time.Now,
	}
}

func (s *supportBundleService) Create(ctx context.Context, req api.SupportBundlePost) (string, io.ReadCloser, error) {
	err := validateRequest(req)
	if err != nil {
		return "", nil, err
	}

	// Resolve upfront, errors can not be reported once the archive is streamed.
	sc, err := s.resolve(ctx, req)
	if err != nil {
		return "", nil, err
	}

	createdAt := s.now().UTC()
	subject := strings.Join(slices.DeleteFunc([]string{req.Cluster, req.Project, req.Name, req.Server}, func(s string) bool { return s == "" }), "_")
	name := fmt.Sprintf("support-bundle-%s-%s-%s", req.Type, subject, createdAt.Format("20060102T150405Z"))

	pr, pw := io.Pipe()
	done := make(chan struct{})

	go func() {
		defer close(done)

		ctx, cancel := context.WithTimeout(ctx, bundleTimeout)
		defer cancel()

		b := newBundle(pw, name, manifest{
			FormatVersion: manifestFormatVersion,
			Version:       version.Version,
			CreatedAt:     createdAt,
			Request:       req,
		})

		c := &collector{
			client: s.client,
			b:      b,
			req:    req,
			scope:  sc,
		}

		c.collect(ctx)

		_ = pw.CloseWithError(b.close())
	}()

	return name + ".zip", &bundleReader{PipeReader: pr, done: done}, nil
}

// bundleReader streams a support bundle and waits for its producer on Close.
type bundleReader struct {
	*io.PipeReader

	done chan struct{}
}

func (b *bundleReader) Close() error {
	err := b.PipeReader.Close()
	<-b.done

	return err
}

func (s *supportBundleService) resolve(ctx context.Context, req api.SupportBundlePost) (scope, error) {
	sc := scope{}

	clusterName := req.Cluster
	if req.Type == api.SupportBundleTypeServer {
		server, err := s.serverSvc.GetByName(ctx, req.Server)
		if err != nil {
			return scope{}, fmt.Errorf("Failed to get server %q: %w", req.Server, err)
		}

		sc.server = server
		if server.Cluster == nil {
			return sc, nil
		}

		clusterName = *server.Cluster
	}

	cluster, err := s.clusterSvc.GetByName(ctx, clusterName)
	if err != nil {
		return scope{}, fmt.Errorf("Failed to get cluster %q: %w", clusterName, err)
	}

	sc.cluster = cluster

	servers, err := s.serverSvc.GetAllWithFilter(ctx, provisioning.ServerFilter{Cluster: &clusterName})
	if err != nil {
		return scope{}, fmt.Errorf("Failed to get servers of cluster %q: %w", clusterName, err)
	}

	slices.SortFunc(servers, func(a, b provisioning.Server) int { return strings.Compare(a.Name, b.Name) })
	sc.clusterServers = servers

	if req.Type == api.SupportBundleTypeProject || req.Type == api.SupportBundleTypeInstance {
		err = s.checkExists(ctx, req, &sc)
		if err != nil {
			return scope{}, err
		}
	}

	return sc, nil
}

// checkExists reports a project or instance, which does not exist, a cluster, which can not be reached, is tolerated.
func (s *supportBundleService) checkExists(ctx context.Context, req api.SupportBundlePost, sc *scope) error {
	ctx, cancel := context.WithTimeout(ctx, existsTimeout)
	defer cancel()

	for _, server := range sc.clusterServers {
		client, err := s.client.IncusClient(ctx, server)
		if err != nil {
			continue
		}

		if req.Type == api.SupportBundleTypeProject {
			_, _, err = client.GetProject(req.Project)
		} else {
			var instance *incusapi.Instance
			instance, _, err = client.UseProject(req.Project).GetInstance(req.Name)
			if err == nil {
				sc.instanceLocation = instance.Location
			}
		}

		if err == nil {
			return nil
		}

		if incusapi.StatusErrorCheck(err, http.StatusNotFound) {
			if req.Type == api.SupportBundleTypeProject {
				return domain.NewErrorf(domain.ErrNotFound, "", "Project %q not found in cluster %q", req.Project, req.Cluster)
			}

			return domain.NewErrorf(domain.ErrNotFound, "", "Instance %q not found in project %q of cluster %q", req.Name, req.Project, req.Cluster)
		}

		if ctx.Err() != nil {
			return nil
		}
	}

	return nil
}

// probe pings the servers in parallel and returns the error for each unreachable one.
func probe(ctx context.Context, client SupportBundleClientPort, servers provisioning.Servers) map[string]error {
	var mu sync.Mutex
	var wg sync.WaitGroup
	unreachable := map[string]error{}

	for _, server := range servers {
		wg.Go(func() {
			ctx, cancel := context.WithTimeout(ctx, probeTimeout)
			defer cancel()

			err := client.Ping(ctx, server)
			if err != nil {
				mu.Lock()
				unreachable[server.Name] = err
				mu.Unlock()
			}
		})
	}

	wg.Wait()

	return unreachable
}
