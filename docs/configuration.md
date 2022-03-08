# SignalFX Prometheus exporter configuration

The configuration file is written in YAML format and adheres to the schema described below.

Generic placeholders are defined as follows:

* `<string>`: a regular string
* `<int>`: an integer
* `<prometheus-label>`: a string following the prometheus label regex `[a-zA-Z_][a-zA-Z0-9_]*`
* `<go-template>`: a string that contains a go-template

The variables usable in go templates are described in the [SignalFlow primer](signalflow.md).

### Schema
```yml

  # SignalFX connection information
  sfx:
    [ realm: <string> | default = "us1" ]
    token: <string>

  # The list of metric flows from SignalFX to process into Prometheus metrics
  flows:
    [ - <flow>, ... ]

  # Optional configuration for scraping based on labels
  grouping:
    [ - <grouping>, ...]
```

### Flow
A flow describes how metrics are queried from SignalFX and processed into Prometheus metrics.

```yml
  name: <prometheus-label>

  # The SignalFlow program to query data from SignalFX
  query: <string>

  # A collection of templates to turn SignalFlow query results into Prometheus metrics
  prometheusMetricTemplate:
    [ - <prometheusMetricTemplate>, ... ]
```

### Prometheus metric template
A Prometheus metric translates a SignalFX metric into a Prometheus metric.

```yml
  # The name of the result Prometheus metric
  [ name: <go-template> | default = "{{ .SignalFxMetricName }}" ]

  # The type of Prometheus to raise for a SignalFX metric
  type: counter | gauge

  # The stream field acts as a selector of a template based on the stream label used in
  # the .publish($stream) command of the query. This way different metric streams from the
  # query can be processed by different metric templates.
  # If a query does not declare any stream in the .publish command, resulting SignalFX
  # metrics will be processed by the default metric template.
  [ stream: <string> | default = "default" ]

  # Labels for the Prometheus metric
  labels:
    [ <prometheus-label>: <go-template>, ... ]
```

### Grouping
Grouping configuration enables scraping metrics based on labels.

```yml
  # The label that can be used for grouped scrapes
  label: <prometheus-label>
  # Conditions that will fail the group scrape when they are not true
  [ groupReadyConditions: ]
    # Minimum number of metrics within a group to let the scrape succeed
    minMetrics: <int>
```
