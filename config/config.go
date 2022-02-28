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
	Name string `yaml:"name"`
	Sfx  struct {
		Realm string `yaml:"realm"`
		Token string `yaml:"token"`
	} `yaml:"sfxAuthentication"`
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

func LoadFlowPrograms(file string) ([]FlowProgram, error) {
	yamlFile, err := ioutil.ReadFile(file)
	if err != nil {
		log.Printf("yamlFile.Get err   #%v ", err)
		return nil, err
	}

	flowProgramList := struct {
		Flows []FlowProgram `yaml:"flows"`
	}{}
	err = yaml.Unmarshal(yamlFile, &flowProgramList)
	if err != nil {
		log.Fatalf("Unmarshal: %v", err)
		return nil, err
	}

	for _, fp := range flowProgramList.Flows {
		if err := fp.Validate(); err != nil {
			return nil, err
		}
	}

	return flowProgramList.Flows, nil
}
