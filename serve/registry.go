package serve

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

type FilteringRegistry struct {
	Registry       prometheus.Gatherer
	VectorSelector string
}

func (fr *FilteringRegistry) Gather() ([]*dto.MetricFamily, error) {
	mfs, err := fr.Registry.Gather()
	if err != nil {
		return nil, err
	}

	filteredMfs := []*dto.MetricFamily{}
	for _, mf := range mfs {
		metrics := []*dto.Metric{}
		for _, m := range mf.GetMetric() {
			for _, l := range m.GetLabel() {
				labelString := fmt.Sprintf("{%s=\"%s\"}", *l.Name, *l.Value)
				if labelString == fr.VectorSelector {
					metrics = append(metrics, m)
					break
				}
			}
		}
		if len(metrics) > 0 {
			filteredMfs = append(filteredMfs, &dto.MetricFamily{
				Name:   mf.Name,
				Help:   mf.Help,
				Type:   mf.Type,
				Metric: metrics,
			})
		}
	}

	return filteredMfs, nil
}
