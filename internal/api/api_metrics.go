package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/security/authz"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/metrics"
	"github.com/FuturFusion/operations-center/internal/util/response"
)

func registerMetricsHandler(router Router, authorizer *authz.Authorizer, gatherer prometheus.Gatherer) {
	// The metrics of the handler itself belong to the runtime component.
	handler := promhttp.InstrumentMetricHandler(
		metrics.RuntimeRegisterer(),
		promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{
			ErrorLog:                            metricsErrorLog{ctx: logger.ContextWithComponent(context.Background(), componentAPI)},
			ErrorHandling:                       promhttp.ContinueOnError,
			Registry:                            metrics.RuntimeRegisterer(),
			EnableOpenMetrics:                   true,
			EnableOpenMetricsTextCreatedSamples: true,
		}),
	)

	router.HandleFunc("GET /{$}", response.With(metricsGet(handler), assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanViewMetrics), assertMetricsEnabled))
}

func assertMetricsEnabled(next response.HandlerFunc) response.HandlerFunc {
	return func(r *http.Request) response.Response {
		if !config.GetLocalSettings().MetricsEnabled {
			return response.SmartError(domain.NewErrorf(domain.ErrOperationNotPermitted, "", "The metrics endpoint is disabled").
				WithHintf(`Enable it with the "metrics_enabled" system setting.`))
		}

		return next(r)
	}
}

// swagger:operation GET /1.0/metrics metrics metrics_get
//
//	Get the metrics
//
//	Gets the metrics of Operations Center in the OpenMetrics or the Prometheus
//	text format. The format depends on the Accept header of the request.
//
//	Returns a bad request error if the "metrics_enabled" system setting is
//	false.
//
//	---
//	produces:
//	  - application/openmetrics-text
//	  - text/plain
//	responses:
//	  "200":
//	    description: Metrics
//	    schema:
//	      type: file
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func metricsGet(handler http.Handler) response.HandlerFunc {
	return func(r *http.Request) response.Response {
		return response.ManualResponse(func(w http.ResponseWriter) error {
			handler.ServeHTTP(w, r)

			return nil
		})
	}
}

// metricsErrorLog logs the errors of the metrics handler. Without it, these
// errors are dropped, because the handler continues on error.
type metricsErrorLog struct {
	ctx context.Context
}

func (m metricsErrorLog) Println(v ...any) {
	slog.ErrorContext(m.ctx, "Failed to gather metrics", slog.String("error", strings.TrimSpace(fmt.Sprintln(v...))))
}
