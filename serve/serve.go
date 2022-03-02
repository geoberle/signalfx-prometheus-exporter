package serve

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"signalfx-prometheus-exporter/config"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/signalfx/signalfx-go/signalflow"
	"github.com/signalfx/signalfx-go/signalflow/messages"

	"golang.org/x/sync/errgroup"
)

var (
	// sfx metrics state
	sfxRegistry = prometheus.NewRegistry()
	sfxCounters = make(map[string]*prometheus.CounterVec)
	sfxGauges   = make(map[string]*prometheus.GaugeVec)

	// self observability
	flowMetricsReceived *prometheus.CounterVec
	flowMetricsFailed   *prometheus.CounterVec
)

func CollectoAndServe(configFile string, listenPort int, observabilityPort int, ctx context.Context) {
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		log.Fatalf("failed to load config: %+s", err)
		return
	}

	// start streaming data from signalfx
	errs, ctx := errgroup.WithContext(ctx)
	for i := range cfg.Flows {
		fp := cfg.Flows[i]
		errs.Go(func() error {
			err := streamData(cfg.Sfx, fp)
			if err != nil {
				log.Fatalf("Flow %s failed because of %+s", fp.Name, err)
			}
			return err
		})
	}

	// start observability server
	flowMetricsReceived = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "sfxpe_flow_metrics_received_total",
		Help: "Number of received metrics",
	}, []string{"flow", "stream"})
	flowMetricsFailed = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "sfxpe_flow_metrics_failed",
		Help: "Number of metrics that failed do process",
	}, []string{"flow", "stream"})
	prometheus.MustRegister(flowMetricsReceived)
	prometheus.MustRegister(flowMetricsFailed)
	obsMux := mux.NewRouter()
	obsMux.Handle("/metrics", promhttp.Handler())
	obsServer := &http.Server{Addr: fmt.Sprintf(":%v", observabilityPort), Handler: obsMux}
	go func() {
		if err := obsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("observability server failure: %+s\n", err)
		}
	}()
	log.Printf("Observability server listening on port %v\n", observabilityPort)

	// start probe server
	mux := mux.NewRouter()
	mux.HandleFunc("/metrics", metricsHandler)
	mux.HandleFunc("/probe/{target}", probeHandler)
	server := &http.Server{Addr: fmt.Sprintf(":%v", listenPort), Handler: mux}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("metrics server failure: %+s\n", err)
		}
	}()
	log.Printf("Scrape server listening on port %v\n", listenPort)

	<-ctx.Done()

	log.Printf("Server stopped")

	ctxShutDown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer func() {
		cancel()
	}()

	if err := server.Shutdown(ctxShutDown); err != nil {
		log.Fatalf("server Shutdown Failed:%+s", err)
	}
}

func probeHandler(w http.ResponseWriter, r *http.Request) {
	// blackbox exporter compatible scrape handler
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(5*float64(time.Second)))
	defer cancel()
	r = r.WithContext(ctx)

	vars := mux.Vars(r)
	targetLabel, ok := vars["target"]
	if ok && targetLabel != "" {
		targetValue, ok := r.URL.Query()["target"]
		if ok && len(targetValue) > 0 {
			metricGatherer := &FilteringRegistry{
				Registry:    sfxRegistry,
				FilterLabel: targetLabel,
				FilterValue: targetValue[0],
			}
			h := promhttp.HandlerFor(metricGatherer, promhttp.HandlerOpts{})
			h.ServeHTTP(w, r)
			return
		}
	}
	w.WriteHeader(http.StatusBadRequest)
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	// renders all metrics
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(5*float64(time.Second)))
	defer cancel()
	r = r.WithContext(ctx)
	h := promhttp.HandlerFor(sfxRegistry, promhttp.HandlerOpts{})
	h.ServeHTTP(w, r)
}

func streamData(sfx config.SignalFxConfig, fp config.FlowProgram) error {
	client, err := signalflow.NewClient(
		signalflow.StreamURLForRealm(sfx.Realm),
		signalflow.AccessToken(sfx.Token),
	)
	if err != nil {
		return fmt.Errorf("Error connecting to SignalFX realm %s - %+s", sfx.Realm, err)
	}

	comp, err := client.Execute(&signalflow.ExecuteRequest{
		Program: fp.Query,
	})
	if err != nil {
		return fmt.Errorf("SignalFlow program for %s is invalid - %+s", fp.Name, err)
	}

	for msg := range comp.Data() {
		if len(msg.Payloads) == 0 {
			continue
		}
		for _, pl := range msg.Payloads {
			meta := comp.TSIDMetadata(pl.TSID)
			stream, ok := meta.InternalProperties["sf_streamLabel"].(string)
			if !ok {
				stream = "default"
			}
			flowMetricsReceived.WithLabelValues(fp.Name, stream).Inc()
			mt, err := fp.GetMetricTemplateForStream(stream)
			if err != nil {
				// todo log
				flowMetricsFailed.WithLabelValues(fp.Name, stream).Inc()
				continue
			}

			if mt.Type == "gauge" {
				gauge, err := getGauge(mt, meta)
				if err != nil {
					flowMetricsFailed.WithLabelValues(fp.Name, stream).Inc()
					// todo log
				} else {
					gauge.Set(pl.Float64())
				}
			} else if mt.Type == "counter" {
				counter, err := getCounter(mt, meta)
				if err != nil {
					flowMetricsFailed.WithLabelValues(fp.Name, stream).Inc()
					// todo log
				} else {
					counter.Add(pl.Float64())
				}
			}
		}
	}
	err = comp.Err()
	return err
}

func buildPrometheusMetadata(metric config.PrometheusMetric, sfxMeta *messages.MetadataProperties) (string, []string, []string, error) {
	// data for template rendering
	safeMetricName := strings.ReplaceAll(sfxMeta.OriginatingMetric, ".", "_")
	safeMetricName = strings.ReplaceAll(safeMetricName, ":", "_")
	tmplRenderingVars := struct {
		SignalFxMetricName string
		SignalFxLabels     map[string]string
	}{
		SignalFxMetricName: safeMetricName,
		SignalFxLabels:     sfxMeta.CustomProperties,
	}

	// build name
	name, err := metric.GetMetricName(tmplRenderingVars)
	if err != nil {
		return "", nil, nil, err
	}

	// build labels
	labelNames := make([]string, len(metric.Labels))
	labelValues := make([]string, len(metric.Labels))
	var i = 0
	for name := range metric.Labels {
		labelNames[i] = name
		value, err := metric.GetLabelValue(name, tmplRenderingVars)
		if err != nil {
			return "", nil, nil, err
		}
		labelValues[i] = value
		i++
	}

	return name, labelNames, labelValues, nil
}

func getGauge(metric config.PrometheusMetric, sfxMeta *messages.MetadataProperties) (prometheus.Gauge, error) {
	name, labelNames, labelValues, err := buildPrometheusMetadata(metric, sfxMeta)
	if err != nil {
		return nil, nil
	}

	// build  or reuse gauge
	g, ok := sfxGauges[name]
	if !ok {
		g = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: name,
		}, labelNames)
		sfxGauges[name] = g
		sfxRegistry.MustRegister(g)
	}
	return g.WithLabelValues(labelValues...), nil
}

func getCounter(metric config.PrometheusMetric, sfxMeta *messages.MetadataProperties) (prometheus.Counter, error) {
	name, labelNames, labelValues, err := buildPrometheusMetadata(metric, sfxMeta)
	if err != nil {
		return nil, nil
	}

	// build  or reuse gauge
	c, ok := sfxCounters[name]
	if !ok {
		c = prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: name,
		}, labelNames)
		sfxCounters[name] = c
		sfxRegistry.MustRegister(c)
	}
	return c.WithLabelValues(labelValues...), nil
}
