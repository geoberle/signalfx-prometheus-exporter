package config_test

import (
	"signalfx-prometheus-exporter/config"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExampleSingleMetric(t *testing.T) {
	c, err := config.LoadConfig("../examples/1_single_metric.yml")
	assert.Nil(t, err)
	assert.Equal(t, "xxx", c.Sfx.Token)
	assert.Equal(t, "us1", c.Sfx.Realm)
	assert.Equal(t, 1, len(c.Flows))

	_, err = c.Flows[0].GetMetricTemplateForStream("foo")
	assert.NotNil(t, err)
	_, err = c.Flows[0].GetMetricTemplateForStream("default")
	assert.Nil(t, err)
}

func TestGetMetricName(t *testing.T) {
	c, _ := config.LoadConfig("../examples/1_single_metric.yml")
	mt, _ := c.Flows[0].GetMetricTemplateForStream("default")

	x, err := mt.GetMetricName(config.NameTemplateVars{
		SignalFxLabels:     map[string]string{"prometheus_name": "foo"},
		SignalFxMetricName: "test",
	})
	assert.Nil(t, err)
	assert.Equal(t, "foo", x)

}

func TestGetLabelValue(t *testing.T) {
	c, _ := config.LoadConfig("../examples/1_single_metric.yml")
	mt, _ := c.Flows[0].GetMetricTemplateForStream("default")

	x, err := mt.GetLabelValue("instance", config.NameTemplateVars{
		SignalFxLabels:     map[string]string{"cp_testname": "test"},
		SignalFxMetricName: "test",
	})
	assert.Nil(t, err)
	assert.Equal(t, "test", x)

}

func TestMinMetricsNotAUInt(t *testing.T) {
	configFile := `---
sfx:
token: xxx
flows:
- name: catchpoint-data
  query: |
    data('catchpoint.counterfailedrequests').publish()
    data('catchpoint.counterrequests').publish()
  prometheusMetricTemplates:
  - type: counter
  labels:
    instance: '{{ .SignalFxLabels.cp_testname }}'
grouping:
- label: instance
  groupReadyCondition:
    minMetrics: -1
`
	_, err := config.LoadConfigFromBytes([]byte(configFile))
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "`-1` into uint")
}
