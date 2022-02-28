# SignalFX Prometheus exporter

[![Build Status](https://github.com/geoberle/signalfx-prometheus-exporter/actions/workflows/docker-image.yml/badge.svg)](https://github.com/geoberle/signalfx-prometheus-exporter/actions/workflows/docker-image.yml)

SignalFX Prometheus exporter provides a Prometheus scrape target for SignalFX metrics.

It leverages the [SignalFlow](https://dev.splunk.com/observability/docs/signalflow/) language to filter, analyze and aggregate data from the Splunk Observability Cloud and offers the extracted data as a standard Prometheus scrape target.

SignalFX Prometheus exporter can be used to bring Splunk Observability Cloud data into existing Prometheus environments, allowing common dashboarding and alerting rules to be applied.

## Configuration
SignalFX Prometheus exporter is configured via a configuration file and command-line flags.

The configuration file declares how data is read from SignalFX and how it is processed into scrapable Prometheus metrics.

Each data flow is described as a `query` defined in the [SignalFlow](https://dev.splunk.com/observability/docs/signalflow/) language. Such a query yields metrics from single or multiple time series.

The metrics provided by the `query` are translated to Prometheus compatible metrics. The `prometheusMetricTemplate` section of a `flow` supports [go templates](https://pkg.go.dev/text/template) to dynamically build Prometheus metric metdata from [SignalFX metadata](docs/signalflow-metadata.md)

```yaml
sfx:
  realm: us1
  token: $token
flows:
- name: raw-catchpoint-metrics
  query: |
    data('catchpoint.*').publish()
  prometheusMetricTemplate:
    name: "{{ .SignalFxMetricName }}"
    labels:
      probe: '{{ .Meta.CustomProperties.cp_testname }}'
```

The exporter process needs to be restarted for configuration changes to become effective.

## Running this software
SignalFX Prometheus Exporter is available as container image.

```bash
docker run -d --rm -p 1236:1236 --name sfxpe -v `pwd`:/config quay.io/goberlec/signalfx-prometheus-exporter:latest serve --config /config/config.yaml
```

Visiting [http://localhost:1236/probe](http://localhost:1236/probe) will show the metrics as defined in the configuration file.

## Architecture
SignalFX Prometheus exporter bridges the gap between the stream based data extraction from SignalFX and the pull based data collection approach of Prometheus.

SignalFX proposed way to consume metrics is their stream based approach driven by the [SignalFlow](https://dev.splunk.com/observability/docs/signalflow/) data processing language. SignalFlow is capable enough to power streams of raw data or even aggregated and pre-analyzed data.

SignalFX Prometheus exporter instantiates SignalFlow driven streams of metrics that will be processed into Prometheus metrics and kept in memory, ready to be scraped. Metric names and labels of the resulting Prometheus metrics can be freely defined with go-templates based on SignalFX metadata access.

The scrape endpoint of SignalFX Prometheus exporter supports filtering based on Instant Vector Selectors. This enables multiple scrapers acting on subsets of data while applying different target labels.

Since the data delivery mechanism from SignalFX is a stream of metrics, the process-local store for metrics requires a warmup time until every metric is available. Scraping the endpoint during that warumup time will result in partial metric discovery only.

![architecture](docs/arch.png)

## Known issues
- no data during warmup phase
- self observability missing (e.g. nr of received events)
