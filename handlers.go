package main

import (
	"github.com/VictoriaMetrics/metrics"
	"net/http"
)

func MetricsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		metrics.WritePrometheus(w, true)
	}
}
