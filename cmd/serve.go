package cmd

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"signalfx-prometheus-exporter/config"
	"signalfx-prometheus-exporter/sfxpe"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/signalfx/signalfx-go/signalflow"
	"github.com/signalfx/signalfx-go/signalflow/messages"
	"github.com/spf13/cobra"

	"golang.org/x/sync/errgroup"
)

var (
	listenPort  int
	configFile  string
	sfxRegistry = prometheus.NewRegistry()
	sfxCounters = make(map[string]*prometheus.CounterVec)
	sfxGauges   = make(map[string]*prometheus.GaugeVec)
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Listen for signalfx scrape requests",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			log.Fatalf("failed to load config: %+s", err)
			return
		}

		// start streaming data from signalfx
		errs, ctx := errgroup.WithContext(cmd.Context())
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

		// start http server
		mux := http.NewServeMux()
		mux.HandleFunc("/probe", probeHandler)
		server := &http.Server{Addr: fmt.Sprintf(":%v", listenPort), Handler: mux}
		go func() {
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("listen:%+s\n", err)
			}
		}()
		log.Printf("Listening on port %v\n", listenPort)

		<-ctx.Done()

		log.Printf("Server stopped")

		ctxShutDown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer func() {
			cancel()
		}()

		if err := server.Shutdown(ctxShutDown); err != nil {
			log.Fatalf("server Shutdown Failed:%+s", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().IntVarP(&listenPort, "port", "p", 1236, "listen port for incoming scrape requests")
	serveCmd.Flags().StringVarP(&configFile, "config", "c", "/config/config.yml", "flow config file")
}

func probeHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(5*float64(time.Second)))
	defer cancel()
	r = r.WithContext(ctx)

	var metricGatherer prometheus.Gatherer
	matchQueries, ok := r.URL.Query()["match"]
	if ok && len(matchQueries) > 0 {
		metricGatherer = &sfxpe.FilteringRegistry{
			Registry:       sfxRegistry,
			VectorSelector: matchQueries[0],
		}
	} else {
		metricGatherer = sfxRegistry
	}

	h := promhttp.HandlerFor(metricGatherer, promhttp.HandlerOpts{})
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
			streamLabel, ok := meta.InternalProperties["sf_streamLabel"].(string)
			if !ok {
				streamLabel = "default"
			}
			mt, err := fp.GetMetricTemplateForStream(streamLabel)
			if err != nil {
				// todo log and inc error for stream
				continue
			}

			if mt.Type == "gauge" {
				gauge, err := getGauge(mt, meta)
				if err != nil {
					// todo log
				} else {
					gauge.Set(pl.Float64())
				}
			} else if mt.Type == "counter" {
				counter, err := getCounter(mt, meta)
				if err != nil {
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
