package config

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"text/template"

	"gopkg.in/yaml.v2"
)

type GroupReadyCondition struct {
	MinMetrics uint `yaml:"minMetrics"`
}

type Grouping struct {
	Label               string
	GroupReadyCondition GroupReadyCondition `yaml:"groupReadyCondition"`
}

type PrometheusMetric struct {
	Name           string            `yaml:"name"`
	Stream         string            `yaml:"stream"`
	Type           string            `yaml:"type"`
	Labels         map[string]string `yaml:"labels"`
	nameTemplate   template.Template
	labelTemplates map[string]template.Template
}

type NameTemplateVars struct {
	SignalFxMetricName string
	SignalFxLabels     map[string]string
}

func (pm *PrometheusMetric) Validate() error {
	// name template
	name := pm.Name
	if name == "" {
		name = "{{ .SignalFxMetricName }}"
	}
	tmpl, err := template.New("x").Parse(name)
	if err != nil {
		return err
	}
	pm.nameTemplate = *tmpl

	// label templates
	labelTemplates := map[string]template.Template{}
	for labelName, labelValue := range pm.Labels {
		tmpl, err := template.New("x").Parse(labelValue)
		if err != nil {
			return err
		}
		labelTemplates[labelName] = *tmpl
	}
	pm.labelTemplates = labelTemplates

	return nil
}

func (pm *PrometheusMetric) GetMetricName(data NameTemplateVars) (string, error) {
	var buffer bytes.Buffer
	err := pm.nameTemplate.Execute(&buffer, data)
	return buffer.String(), err
}

func (pm *PrometheusMetric) GetLabelValue(labelName string, data NameTemplateVars) (string, error) {
	tmpl, ok := pm.labelTemplates[labelName]
	if !ok {
		return "", fmt.Errorf("Could not find label named %s", labelName)
	}
	var buffer bytes.Buffer
	err := tmpl.Execute(&buffer, data)
	return buffer.String(), err
}

type FlowProgram struct {
	Name              string             `yaml:"name"`
	Query             string             `yaml:"query"`
	MetricTemplates   []PrometheusMetric `yaml:"prometheusMetricTemplates"`
	templatesByStream map[string]PrometheusMetric
}

func (fp *FlowProgram) GetMetricTemplateForStream(stream string) (PrometheusMetric, error) {
	mt, ok := fp.templatesByStream[stream]
	if !ok {
		return PrometheusMetric{}, fmt.Errorf("No metric template found for stream %s", stream)
	}
	return mt, nil
}

func (fp *FlowProgram) Validate() error {
	defaultStreamFound := false
	fp.templatesByStream = make(map[string]PrometheusMetric)
	for i := range fp.MetricTemplates {
		mtp := &fp.MetricTemplates[i]
		if err := mtp.Validate(); err != nil {
			return err
		}
		if mtp.Stream == "" {
			mtp.Stream = "default"
		}
		if mtp.Stream == "default" && defaultStreamFound {
			return fmt.Errorf("More than one default stream found in flow %s", fp.Name)
		} else if mtp.Stream == "default" {
			defaultStreamFound = true
		}
		fp.templatesByStream[mtp.Stream] = *mtp
	}
	return nil
}

type Sfx struct {
	Realm string `yaml:"realm"`
	Token string `yaml:"token"`
}

func (sfx *Sfx) Validate() error {
	if sfx.Realm == "" {
		sfx.Realm = "us1"
	}
	return nil
}

type Config struct {
	Sfx       Sfx           `yaml:"sfx"`
	Flows     []FlowProgram `yaml:"flows"`
	Groupings []Grouping    `yaml:"grouping"`
}

func (c *Config) Validate() error {
	if err := c.Sfx.Validate(); err != nil {
		return err
	}
	for i := range c.Flows {
		fp := &c.Flows[i]
		if err := fp.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func LoadConfigFromBytes(configBytes []byte) (*Config, error) {
	var cfg Config
	err := yaml.Unmarshal(configBytes, &cfg)
	if err != nil {
		log.Printf("Unmarshal: %v\n", err)
		return nil, err
	}

	cfg.Validate()
	return &cfg, nil
}

func LoadConfig(file string) (*Config, error) {
	configBytes, err := ioutil.ReadFile(file)
	if err != nil {
		log.Printf("yamlFile.Get err   #%v ", err)
		return nil, err
	}
	return LoadConfigFromBytes(configBytes)
}
