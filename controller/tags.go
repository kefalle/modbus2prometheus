package controller

import "github.com/VictoriaMetrics/metrics"

type Tag struct {
	Name        string
	DisplayName string
	Group       string
	Address     uint16
	Action      func(interface{}, *Tag)
	Method      uint8
	LastValue   interface{}
	Gauge       *metrics.Gauge
	controller  *Controller
}

func (t *Tag) GetName() string {
	if t.DisplayName != "" {
		return t.DisplayName
	}
	return t.Name
}
