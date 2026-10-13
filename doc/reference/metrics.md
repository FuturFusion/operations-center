# Metrics

Operations Center exposes metrics about itself on the REST API. A monitoring
system can collect these metrics to observe the health of Operations Center.

## Endpoint

Operations Center serves the metrics at `GET /1.0/metrics` on the regular REST
API listener.

The endpoint supports several formats. The `Accept` header of the request selects
the format:

| `Accept` header                                                                | Format                                                                      |
| :---                                                                           | :---                                                                        |
| `application/openmetrics-text; version=1.0.0`                                  | [OpenMetrics](https://prometheus.io/docs/specs/om/open_metrics_spec/) 1.0.0 |
| `application/openmetrics-text`                                                 | OpenMetrics 0.0.1                                                           |
| `application/vnd.google.protobuf; proto=io.prometheus.client.MetricFamily`     | Prometheus protobuf format                                                  |
| any other value                                                                | Prometheus text format                                                      |

## Access

The endpoint requires authentication, like the rest of the REST API. With
OpenFGA, the user needs the `can_view_metrics` entitlement on the server. The
`viewer` relation grants this entitlement.

The endpoint is enabled by default. Set `metrics_enabled` to `false` in the
[system settings](settings.md) to disable it. A request to a disabled endpoint
returns an error.

## Collect the metrics

Fetch the metrics with a trusted TLS client certificate:

```shell
curl --cert client.crt --key client.key https://operations-center.example.com:8443/1.0/metrics
```

Request the OpenMetrics format:

```shell
curl --cert client.crt --key client.key \
  -H 'Accept: application/openmetrics-text; version=1.0.0' \
  https://operations-center.example.com:8443/1.0/metrics
```

The following example shows a scrape configuration for Prometheus:

```yaml
scrape_configs:
  - job_name: operations-center
    scheme: https
    metrics_path: /1.0/metrics
    tls_config:
      cert_file: /etc/prometheus/operations-center/client.crt
      key_file: /etc/prometheus/operations-center/client.key
      ca_file: /etc/prometheus/operations-center/server.crt
    static_configs:
      - targets:
          - operations-center.example.com:8443
```

The `access_log` component logs every request to the endpoint at the `INFO`
level. The default log level is `WARN`, so these records do not appear by
default. Set `access_log` to `INFO` in `log_levels` in the system settings to
see them.

## Components

Components group the metrics. The `metrics_components` setting lists the
enabled components. A disabled component exposes no metrics.
See [system settings](settings.md) for details.

| Component | Metrics                                                        | Enabled by default |
| :---      | :---                                                           | :---               |
| `runtime` | Go runtime, process, build information and the metrics handler | yes                |

## Metrics of the `runtime` component

| Metric                             | Type  | Description                                                                |
| :---                               | :---  | :---                                                                       |
| `go_*`                             | mixed | Go runtime, for example goroutines, memory and garbage collection          |
| `process_*`                        | mixed | Process, for example CPU time, memory and open file descriptors            |
| `promhttp_metric_handler_*`        | mixed | Requests served by the metrics endpoint                                    |
| `operations_center_build_info`     | gauge | Version of Operations Center in the `version` label. The value is always 1 |
| `operations_center_uptime_seconds` | gauge | Time in seconds since the start of Operations Center                       |
