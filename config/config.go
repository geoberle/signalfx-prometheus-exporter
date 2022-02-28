package config

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"text/template"

	"gopkg.in/yaml.v2"
)

type PrometheusMetric struct {
	Name           string            `yaml:"name"`
	Labels         map[string]string `yaml:"labels"`
	nameTemplate   *template.Template
	labelTemplates map[string]*template.Template
}

func (pm *PrometheusMetric) Validate() error {
	// name template
	tmpl, err := template.New("x").Parse(pm.Name)
	if err != nil {
		return err
	}
	pm.nameTemplate = tmpl

	// label templates
	labelTemplates := map[string]*template.Template{}
	for labelName, labelValue := range pm.Labels {
		tmpl, err := template.New("x").Parse(labelValue)
		if err != nil {
			return err
		}
		labelTemplates[labelName] = tmpl
	}
	pm.labelTemplates = labelTemplates

	return nil
}

func (pm *PrometheusMetric) GetMetricName(data interface{}) (string, error) {
	var buffer bytes.Buffer
	err := pm.nameTemplate.Execute(&buffer, data)
	return buffer.String(), err
}

func (pm *PrometheusMetric) GetLabelValue(labelName string, data interface{}) (string, error) {
	tmpl, ok := pm.labelTemplates[labelName]
	if !ok {
		return "", fmt.Errorf("Could not find label named %s", labelName)
	}
	var buffer bytes.Buffer
	err := tmpl.Execute(&buffer, data)
	return buffer.String(), err
}

type FlowProgram struct {
	Name     string `yaml:"name"`
	Query    string `yaml:"query"`
	Selector struct {
		MatchExpressions []struct {
			Key      string   `yaml:"key"`
			Operator string   `yaml:"operator"`
			Values   []string `yaml:"values"`
		} `yaml:"matchExpressions"`
	} `yaml:"selector"`
	Metric *PrometheusMetric `yaml:"prometheusMetricTemplate"`
}

func (fp *FlowProgram) Validate() error {
	return fp.Metric.Validate()
}

type SignalFxConfig struct {
	Realm string `yaml:"realm"`
	Token string `yaml:"token"`
}

func (sfx *SignalFxConfig) Validate() error {
	return nil
}

type Config struct {
	Sfx   SignalFxConfig `yaml:"sfx"`
	Flows []FlowProgram  `yaml:"flows"`
}

func (c *Config) Validate() error {
	if err := c.Sfx.Validate(); err != nil {
		return err
	}
	for _, fp := range c.Flows {
		if err := fp.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func LoadConfig(file string) (*Config, error) {
	yamlFile, err := ioutil.ReadFile(file)
	if err != nil {
		log.Printf("yamlFile.Get err   #%v ", err)
		return nil, err
	}

	var cfg Config
	err = yaml.Unmarshal(yamlFile, &cfg)
	if err != nil {
		log.Fatalf("Unmarshal: %v", err)
		return nil, err
	}

	cfg.Validate()
	return &cfg, nil
}
