# Metrics

Operations Center exposes metrics about itself on the REST API. A monitoring
system can collect these metrics to observe the health of Operations Center.

## Endpoint

Operations Center serves the metrics at `GET /1.0/metrics` on the regular REST
API listener.

The endpoint supports several formats. The `Accept` header of the request selects
the format:

| `Accept` header                                                                                | Format                                                                      |
| :---                                                                                           | :---                                                                        |
| `application/openmetrics-text; version=1.0.0`                                                  | [OpenMetrics](https://prometheus.io/docs/specs/om/open_metrics_spec/) 1.0.0 |
| `application/openmetrics-text`                                                                 | OpenMetrics 0.0.1                                                           |
| `application/vnd.google.protobuf; proto=io.prometheus.client.MetricFamily; encoding=delimited` | Prometheus protobuf format                                                  |
| any other value                                                                                | Prometheus text format                                                      |

## Access

The endpoint requires authentication. With OpenFGA, the user needs the
`can_view_metrics` entitlement on the server. The `viewer` relation grants this
entitlement.

A monitoring system does not need full access to Operations Center. Add its TLS
client certificate to `trusted_tls_metrics_client_cert_fingerprints` or
`trusted_tls_metrics_client_certificates`. A certificate in one of these lists is
a metrics only certificate. See
[security settings](settings.md#security-settings) for the rules of these
settings.

A metrics only certificate can only read the metrics. Every other request that
needs a permission fails with a 403 error. An OpenFGA relation does not change
this. Endpoints that need no authentication stay reachable.

`GET /1.0` returns `tls` in the `auth` field for a metrics only certificate. The
field shows how the client is authenticated. It does not show what the client is
allowed to do. The UI and the command line client therefore treat a metrics only
certificate as logged in, but every request except the metrics fails with a 403
error. Use a metrics only certificate only to collect metrics.

The endpoint is enabled by default. Set `metrics_enabled` to `false` in the
[system settings](settings.md) to disable it. A request to a disabled endpoint
returns an error.

## Collect the metrics

Fetch the metrics with a trusted TLS client certificate or with a metrics only
certificate:

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

| Component     | Metrics                                                             | Enabled by default |
| :---          | :---                                                                | :---               |
| `runtime`     | Go runtime, process, build information and the metrics handler      | yes                |
| `daemon.task` | Errors, duration, last success and interval of the background tasks | yes                |

## Metrics of the `runtime` component

| Metric                             | Type  | Description                                                                |
| :---                               | :---  | :---                                                                       |
| `go_*`                             | mixed | Go runtime, for example goroutines, memory and garbage collection          |
| `process_*`                        | mixed | Process, for example CPU time, memory and open file descriptors            |
| `promhttp_metric_handler_*`        | mixed | Requests served by the metrics endpoint                                    |
| `operations_center_build_info`     | gauge | Version of Operations Center in the `version` label. The value is always 1 |
| `operations_center_uptime_seconds` | gauge | Time in seconds since the start of Operations Center                       |

## Metrics of the `daemon.task` component

Operations Center runs background tasks at a fixed interval, for example the
refresh of the updates or the polling of the servers. These metrics record
scheduled background task runs. Each background task has its own component
`daemon.task.<name>`. See [Metrics components](settings.md#metrics-components)
for the complete list.
The component is enabled by default. Remove `daemon.task` from
`metrics_components` to disable the metrics of all background tasks.

| Metric                                                  | Type      | Labels              | Description                          |
| :---                                                    | :---      | :---                | :---                                 |
| `operations_center_task_errors_total`                   | counter   | `task`, `retryable` | Number of failed runs                |
| `operations_center_task_duration_seconds`               | histogram | `task`              | Duration of the runs in seconds      |
| `operations_center_task_last_success_timestamp_seconds` | gauge     | `task`              | Unix time of the last successful run |
| `operations_center_task_interval_seconds`               | gauge     | `task`              | Interval of the task in seconds      |

The labels have the following values:

| Label       | Value                                                                                        |
| :---        | :---                                                                                         |
| `task`      | Name of the background task without the `daemon.task.` prefix, for example `refresh_updates` |
| `retryable` | `true` if the error is temporary and a later run can succeed, otherwise `false`              |

A run fails if the task returns an error. Use `retryable="false"` for alerts.
A high rate of `retryable="true"` shows operations that often fail on the first
attempt.

A run that ends because Operations Center stops is not recorded. A run that ends
because the member loses the cluster leadership is not recorded either.

There is no counter for the number of runs. The interval of the task defines
the rate. `operations_center_task_duration_seconds_count` counts the recorded
runs. Use it as the base for an error ratio.

The histogram uses the buckets 0.1, 0.5, 1, 5, 10, 60, 300 and 600 seconds.
Some tasks run for several minutes. The default Prometheus buckets end at 10
seconds, so these buckets go further. A run longer than 600 seconds counts only
in the `+Inf` bucket.

On IncusOS, the first recorded `refresh_updates` run includes a 30-second
startup delay. The first recorded `refresh_image_sources` run includes a
10-minute startup delay.

### Cluster

On a cluster, only the leader runs the background tasks. The errors and the
duration of a member keep their values after the member loses the leadership.

The gauges `operations_center_task_last_success_timestamp_seconds` and
`operations_center_task_interval_seconds` exist only on the member that runs the
task. When a member starts a task, it sets the last success to the current
time. So the example alert below also detects a task that never succeeds.
