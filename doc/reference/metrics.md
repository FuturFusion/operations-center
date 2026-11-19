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

| Component | Metrics                                                               | Enabled by default |
| :---      | :---                                                                  | :---               |
| `runtime` | Go runtime, process, build information and the metrics handler        | yes                |
| `api`     | Count, server errors, aborted responses and duration of HTTP requests | yes                |

## Metrics of the `runtime` component

| Metric                             | Type  | Description                                                                |
| :---                               | :---  | :---                                                                       |
| `go_*`                             | mixed | Go runtime, for example goroutines, memory and garbage collection          |
| `process_*`                        | mixed | Process, for example CPU time, memory and open file descriptors            |
| `promhttp_metric_handler_*`        | mixed | Requests served by the metrics endpoint                                    |
| `operations_center_build_info`     | gauge | Version of Operations Center in the `version` label. The value is always 1 |
| `operations_center_uptime_seconds` | gauge | Time in seconds since the start of Operations Center                       |

## Metrics of the `api` component

| Metric                                            | Type      | Labels                    | Description                                      |
| :---                                              | :---      | :---                      | :---                                             |
| `operations_center_http_requests_total`           | counter   | `method`, `route`, `code` | Number of HTTP requests                          |
| `operations_center_http_request_errors_total`     | counter   | `method`, `route`         | Number of HTTP requests with a server error      |
| `operations_center_http_requests_aborted_total`   | counter   | `method`, `route`         | Number of HTTP requests with an aborted response |
| `operations_center_http_request_duration_seconds` | histogram | `method`, `route`         | Duration of the HTTP requests in seconds         |

The labels have the following values:

| Label    | Value                                                                                                                                           |
| :---     | :---                                                                                                                                            |
| `method` | HTTP method of the request. The value is `other` for a method that is not a standard HTTP method in upper case                                  |
| `route`  | Path pattern of the matched route, for example `/1.0/provisioning/servers/{name}`. The value is `unmatched` for a request that matches no route |
| `code`   | HTTP status code of the response                                                                                                                |

The metrics cover the HTTP requests that reach the handlers of Operations Center. This includes requests to the web UI and to the metrics endpoint.
The metrics do not cover requests that the HTTP server rejects before a handler runs, for example a malformed request.

A request matches no route if the path is unknown. A request also matches no route if the route does not support the method of the request.
A redirect, for example to the clean form of the path, has the route of its target.
If the target matches no route, the redirect has the route `unmatched`.

A server error is a response with a status code of 500 or higher.
If a handler aborts before it starts the response, the metrics record the code 500 and a server error. The client receives no response.
A response that aborts after it starts keeps its status code. The abort counts as an aborted response. The abort itself is not a server error.
A handler aborts the response if it cannot write it, for example because the client closed the connection.
A handler can also crash, for example because of a bug. A crash always counts as a server error, also after the response started.
Some handlers ignore a failed read or write, for example the download of an image. Then the request keeps its status code. It does not count as an aborted response.
An aborted response does not always mean that the client closed the connection. A handler also aborts the response if it cannot read the data, for example from IncusOS or from a file.

A handler can hijack the connection to switch the protocol. A hijacked connection has the code 101.
The duration ends with the hijack. It does not include the time of the session after the switch.

The histogram uses the buckets 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5 and 10 seconds.
