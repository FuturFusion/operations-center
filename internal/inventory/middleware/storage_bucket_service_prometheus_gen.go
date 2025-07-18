// Code generated by mockery. DO NOT EDIT.
// template: github.com/FuturFusion/operations-center/internal/metrics/prometheus.gotmpl

package middleware

import (
	"context"
	"time"

	"github.com/FuturFusion/operations-center/internal/inventory"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// StorageBucketServiceWithPrometheus implements inventory.StorageBucketService interface with all methods wrapped
// with Prometheus metrics.
type StorageBucketServiceWithPrometheus struct {
	base         inventory.StorageBucketService
	instanceName string
}

var storageBucketServiceDurationSummaryVec = promauto.NewSummaryVec(
	prometheus.SummaryOpts{
		Name:       "storage_bucket_service_duration_seconds",
		Help:       "storageBucketService runtime duration and result",
		MaxAge:     time.Minute,
		Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
	},
	[]string{"instance_name", "method", "result"},
)

// NewStorageBucketServiceWithPrometheus returns an instance of the inventory.StorageBucketService decorated with prometheus summary metric.
func NewStorageBucketServiceWithPrometheus(base inventory.StorageBucketService, instanceName string) StorageBucketServiceWithPrometheus {
	return StorageBucketServiceWithPrometheus{
		base:         base,
		instanceName: instanceName,
	}
}

// GetAllUUIDsWithFilter implements inventory.StorageBucketService.
func (_d StorageBucketServiceWithPrometheus) GetAllUUIDsWithFilter(ctx context.Context, filter inventory.StorageBucketFilter) (uUIDs []uuid.UUID, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		storageBucketServiceDurationSummaryVec.WithLabelValues(_d.instanceName, "GetAllUUIDsWithFilter", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetAllUUIDsWithFilter(ctx, filter)
}

// GetAllWithFilter implements inventory.StorageBucketService.
func (_d StorageBucketServiceWithPrometheus) GetAllWithFilter(ctx context.Context, filter inventory.StorageBucketFilter) (storageBuckets inventory.StorageBuckets, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		storageBucketServiceDurationSummaryVec.WithLabelValues(_d.instanceName, "GetAllWithFilter", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetAllWithFilter(ctx, filter)
}

// GetByUUID implements inventory.StorageBucketService.
func (_d StorageBucketServiceWithPrometheus) GetByUUID(ctx context.Context, id uuid.UUID) (storageBucket inventory.StorageBucket, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		storageBucketServiceDurationSummaryVec.WithLabelValues(_d.instanceName, "GetByUUID", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetByUUID(ctx, id)
}

// ResyncByUUID implements inventory.StorageBucketService.
func (_d StorageBucketServiceWithPrometheus) ResyncByUUID(ctx context.Context, id uuid.UUID) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		storageBucketServiceDurationSummaryVec.WithLabelValues(_d.instanceName, "ResyncByUUID", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.ResyncByUUID(ctx, id)
}

// SyncCluster implements inventory.StorageBucketService.
func (_d StorageBucketServiceWithPrometheus) SyncCluster(ctx context.Context, cluster string) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		storageBucketServiceDurationSummaryVec.WithLabelValues(_d.instanceName, "SyncCluster", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.SyncCluster(ctx, cluster)
}
