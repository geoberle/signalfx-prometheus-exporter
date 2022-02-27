package config

import (
	"io/ioutil"
	"log"

	"gopkg.in/yaml.v2"
)

type PrometheusMetric struct {
	Name   string            `yaml:"name"`
	Labels map[string]string `yaml:"labels"`
}

type FlowProgram struct {
	Name string `yaml:"name"`
	Sfx  struct {
		Realm string `yaml:"realm"`
		Token string `yaml:"token"`
	} `yaml:"sfx"`
	Query    string `yaml:"query"`
	Selector struct {
		MatchExpressions []struct {
			Key      string   `yaml:"key"`
			Operator string   `yaml:"operator"`
			Values   []string `yaml:"values"`
		} `yaml:"matchExpressions"`
	} `yaml:"selector"`
	Metric PrometheusMetric `yaml:"prometheusMetric"`
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

	return flowProgramList.Flows, nil
}
