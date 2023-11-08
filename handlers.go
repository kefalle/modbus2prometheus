package main

import (
	"github.com/VictoriaMetrics/metrics"
	"log"
	"modbus2prometheus/controller"
	"net/http"
)

func TagsHahdler(c *controller.Controller) http.HandlerFunc {
	fn := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		data, err := c.Json()
		if err != nil {
			log.Println("Cannot make json err: " + err.Error())
			return
		}

		_, err = w.Write(data)
		if err != nil {
			log.Println("Cannot send response")
		}
	}

	return fn
}

func MetricsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		metrics.WritePrometheus(w, true)
	}
}
