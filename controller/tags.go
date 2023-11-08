package controller

import "github.com/VictoriaMetrics/metrics"

type Tag struct {
	Name        string
	DisplayName string
	Address     uint16
	Action      func(interface{}, *Tag)
	Method      OperationType
	LastValue   interface{}
	Gauge       *metrics.Gauge
	controller  *Controller
}
