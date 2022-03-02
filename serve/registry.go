package serve

import (
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

type FilteringRegistry struct {
	Registry    prometheus.Gatherer
	FilterLabel string
	FilterValue string
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
				if *l.Name == fr.FilterLabel && *l.Value == fr.FilterValue {
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
