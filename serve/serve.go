package serve

import (
	"context"
	"errors"
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
	sfxRegistry               = prometheus.NewRegistry()
	sfxCounters               = make(map[string]*prometheus.CounterVec)
	sfxGauges                 = make(map[string]*prometheus.GaugeVec)
	lastMetricInFlowTimestamp = make(map[string]time.Time)

	// self observability
	flowMetricsReceived *prometheus.CounterVec
	flowMetricsFailed   *prometheus.CounterVec
	flowLastReceived    *prometheus.GaugeVec
)

func CollectoAndServe(configFile string, listenPort int, observabilityPort int, ctx context.Context) {
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		log.Printf("failed to load config: %+s\n", err)
		return
	}

	// start streaming data from signalfx
	errs, ctx := errgroup.WithContext(ctx)
	for i := range cfg.Flows {
		fp := cfg.Flows[i]
		errs.Go(func() error {
			err := streamData(cfg.Sfx, fp)
			log.Printf("Flow %s failed because of %+s\n", fp.Name, err)
			return err
		})
	}

	// start observability server
	flowMetricsReceived = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "sfxpe_flow_metrics_received_total",
		Help: "Number of received metrics",
	}, []string{"flow", "stream"})
	flowMetricsFailed = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "sfxpe_flow_metrics_failed_total",
		Help: "Number of metrics that failed do process",
	}, []string{"flow", "stream"})
	flowLastReceived = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sfxpe_flow_last_received_seconds",
		Help: "Timestamp where the last metric was received",
	}, []string{"flow", "stream"})
	prometheus.MustRegister(flowMetricsReceived)
	prometheus.MustRegister(flowMetricsFailed)
	prometheus.MustRegister(flowLastReceived)
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
	mux.HandleFunc("/ready", readinessHandler)
	mux.HandleFunc("/healthy", livenessHandler)
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

	log.Printf("Server stopped\n")

	ctxShutDown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer func() {
		cancel()
	}()

	if err := server.Shutdown(ctxShutDown); err != nil {
		log.Printf("server Shutdown Failed: %+s\n", err)
	}
}

func readinessHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func livenessHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
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
	// initialize flow metrics
	for _, mt := range fp.MetricTemplates {
		flowMetricsReceived.WithLabelValues(fp.Name, mt.Stream)
		flowMetricsFailed.WithLabelValues(fp.Name, mt.Stream)
		flowMetricsFailed.WithLabelValues(fp.Name, mt.Stream)
	}

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
			flowLastReceived.WithLabelValues(fp.Name, stream).SetToCurrentTime()
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

	/* signalflow programs without stop timestamp should run forever. if the
	above loop exists, it implies that the program exited. if comp.Err() is
	not set, we have to assume an unknown error */
	err = comp.Err()
	if err == nil {
		err = errors.New("flow failed for an unknown reason")
	}
	client.Close()
	return err
}

func buildPrometheusMetadata(metric config.PrometheusMetric, sfxMeta *messages.MetadataProperties) (string, []string, []string, error) {
	// data for template rendering
	safeMetricName := strings.ReplaceAll(sfxMeta.OriginatingMetric, ".", "_")
	safeMetricName = strings.ReplaceAll(safeMetricName, ":", "_")
	templateVars := config.NameTemplateVars{
		SignalFxMetricName: safeMetricName,
		SignalFxLabels:     sfxMeta.CustomProperties,
	}

	// build name
	name, err := metric.GetMetricName(templateVars)
	if err != nil {
		return "", nil, nil, err
	}

	// build labels
	labelNames := make([]string, len(metric.Labels))
	labelValues := make([]string, len(metric.Labels))
	var i = 0
	for name := range metric.Labels {
		labelNames[i] = name
		value, err := metric.GetLabelValue(name, templateVars)
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
