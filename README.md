![build](https://ci.ext.devshift.net/buildStatus/icon?job=app-sre-signalfx-prometheus-exporter-gh-build-main)
![license](https://img.shields.io/github/license/app-sre/signalfx-prometheus-exporter.svg?style=flat)

# SignalFX Prometheus exporter

SignalFX Prometheus exporter provides a Prometheus scrape target for SignalFX metrics.

It leverages the [SignalFlow](https://dev.splunk.com/observability/docs/signalflow/) language to filter, analyze and aggregate data from the Splunk Observability Cloud and offers the extracted data as a standard Prometheus scrape target.

SignalFX Prometheus exporter can be used to bring Splunk Observability Cloud data into existing Prometheus environments, allowing common dashboarding and alerting rules to be applied.

## Configuration
SignalFX Prometheus exporter is configured via a [configuration file](docs/configuration.md) and commandline flags.

The configuration file declares how data is read from SignalFX and how it is processed into scrapable Prometheus metrics.

Each data flow is described as a `query` defined in the [SignalFlow](https://dev.splunk.com/observability/docs/signalflow/) language. Such a query yields metrics from single or multiple time series.

The metrics provided by the `query` are translated to Prometheus compatible metrics. The `prometheusMetricTemplate` section of a `flow` supports [go templates](https://pkg.go.dev/text/template) to dynamically build Prometheus metric metdata from [SignalFX metadata](docs/signalflow.md)

```yaml
sfx:
  realm: us1
  token: $token
flows:
- name: catchpoint-metrics
  query: |
    data('catchpoint.counterfailedrequests').publish(prometheus_name="catchpoint_failures_total")
  prometheusMetricTemplates:
  - name: "{{ .SignalFxLabels.prometheus_name }}"
    type: counter
    labels:
      probe: '{{ .SignalFxLabels.cp_testname }}'
```

The exporter process needs to be restarted for configuration changes to become effective.

Have a look at the [examples directory](/examples) for inspiration.

## Running this software
SignalFX Prometheus Exporter is available as container image.

```bash
docker run -d --rm -p 9091:9091 --name sfxpe -v `pwd`:/config quay.io/goberlec/signalfx-prometheus-exporter:latest serve --config /config/config.yaml
```

Visiting [http://localhost:9091/metrics](http://localhost:9091/metrics) will show the metrics as defined in the configuration file.

Observability metrics for the exporter itself are available on http://localhost:9090/metrics

## Architecture
SignalFX Prometheus exporter bridges the gap between the stream based data extraction from SignalFX and the pull based data collection approach of Prometheus.

SignalFX proposed way to consume metrics is their stream based approach driven by the [SignalFlow](https://dev.splunk.com/observability/docs/signalflow/) data processing language. SignalFlow is capable enough to power streams of raw data or even aggregated and pre-analyzed data.

SignalFX Prometheus exporter instantiates SignalFlow driven streams of metrics that will be processed into Prometheus metrics and kept in memory, ready to be scraped. Metric names and labels of the resulting Prometheus metrics can be freely defined with go-templates based on SignalFX metadata access.

Since the data delivery mechanism from SignalFX is a stream of metrics, the process-local store for metrics requires a warmup time until every metric is available. Scraping the endpoint during that warumup time will result in partial metric discovery only.

![architecture](docs/arch.png)

## Scraping metric groups

Metrics can also be scraped based on a metric label. This can be enabled by providing
grouping [configuration](docs/configuration.md).

Once configured, the endpoint for a group scrape looks like `:9091/probe/$label?target=$value` and returns only metrics with the `$label` set to `$value`.

The async metric delivery mode of SignalFX makes it necessary to handle situations
where metrics are yet missing (e.g. cold cache on process startup). The
`grouping.groupReadyConditions` config section provides options to declare the behaviour
for such situations, failing a scrape on the `/metric/$label` endpoint when
the conditions are not satisfied. This will result in the default metric `up`
on the scraper side to highlight that metrics could not be aquired. Depending
on the situation, this behaviour might be better than scraping incomplete
metrics. Right now, the `minMetrics` condition is supported, failing a scrape
when less than `minMetrics` metrics would be exposed.

The `target` parameter to supply a filter for the label makes this scrape
endpoint compatible with the [`Probe`](https://prometheus-operator.dev/docs/operator/design/#probe)
CRD from the Prometheus operator.

### Example

The following example enables filtering based on the `instance` label of metrics. A filtered
scrape on this label can be done via the `:9091/metrics/instance?target=value` endpoint,
where the `target` query parameter supplies the value to filter on. Additionally, the scrape
will fail when less than 2 metrics are left after filtering.

```yaml
grouping:
- label: instance
  groupReadyCondition:
    minMetrics: 2
```


## Observability
Obersvability metrics for flow programs and the go runtime are available on observability endpoint `:9090/metrics`.

| Metric name| Metric type | Labels |
| ---------- | ----------- | ------ |
| sfxpe_flow_metrics_received_total | Counter | `flow`=&lt;flow program name&gt; <br> `stream`=&lt;stream name&gt; |
| sfxpe_flow_metrics_failed_total | Counter | `flow`=&lt;flow program name&gt; <br> `stream`=&lt;stream name&gt; |
| sfxpe_flow_last_received_seconds | Gauge | `flow`=&lt;flow program name&gt; <br> `stream`=&lt;stream name&gt; |

An article that goes into details about the exposed go runtime metrics can be found [here](https://povilasv.me/prometheus-go-metrics/).

## Known issues
- verify query - publish() must exists at least once
