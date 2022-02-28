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
)

var (
	listenPort  int
	configFile  string
	sfxRegistry *prometheus.Registry
	sfxGauges   map[string]*prometheus.GaugeVec
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
		sfxRegistry = prometheus.NewRegistry()
		sfxGauges = make(map[string]*prometheus.GaugeVec)
		for _, fp := range cfg.Flows {
			go func(fp config.FlowProgram) {
				streamData(cfg.Sfx, fp)
			}(fp)
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

		<-cmd.Context().Done()

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
	serveCmd.Flags().StringVarP(&configFile, "config", "c", "", "flow config file")

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

func streamData(sfx config.SignalFxConfig, fp config.FlowProgram) {
	client, _ := signalflow.NewClient(
		signalflow.StreamURLForRealm(sfx.Realm),
		signalflow.AccessToken(sfx.Token),
	)

	comp, _ := client.Execute(&signalflow.ExecuteRequest{
		Program: fp.Query,
	})

	for msg := range comp.Data() {
		if len(msg.Payloads) == 0 {
			continue
		}
		for _, pl := range msg.Payloads {
			meta := comp.TSIDMetadata(pl.TSID)
			gauge, err := getGauge(fp.Metric, meta)
			if err != nil {
				// todo log
			} else {
				gauge.Set(pl.Float64())
			}
		}
	}
}

func getGauge(metric *config.PrometheusMetric, sfxMeta *messages.MetadataProperties) (prometheus.Gauge, error) {
	// data for template rendering
	safeMetricName := strings.ReplaceAll(sfxMeta.OriginatingMetric, ".", "_")
	safeMetricName = strings.ReplaceAll(safeMetricName, ":", "_")
	tmplRenderingVars := struct {
		SignalFxMetricName string
		Meta               *messages.MetadataProperties
	}{
		SignalFxMetricName: safeMetricName,
		Meta:               sfxMeta,
	}

	// build name
	name, err := metric.GetMetricName(tmplRenderingVars)
	if err != nil {
		return nil, err
	}

	// build labels
	labelNames := make([]string, len(metric.Labels))
	labelValues := make([]string, len(metric.Labels))
	var i = 0
	for name := range metric.Labels {
		labelNames[i] = name
		value, err := metric.GetLabelValue(name, tmplRenderingVars)
		if err != nil {
			return nil, err
		}
		labelValues[i] = value
		i++
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
