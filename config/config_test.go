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
