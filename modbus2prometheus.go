package main

import (
	"flag"
	"fmt"
	"github.com/mcuadros/go-defaults"
	"log"
	"modbus2prometheus/controller"
	"net/http"
	"os"
)

const APP = "modbus2prometheus"
const VERSION = "0.0.2"

var (
	httpListenAddr = flag.String("httpListenAddr", ":3000", "TCP address to listen for http connections.")
	modbusTcpAddr  = flag.String("modbusTcpAddr", "tcp://192.168.1.200:8899", "TCP address to modbus device with modbus TCP.")
	config         = flag.String("config", "./config.yaml", "Modbus controller configuration")

	ControllerConfig *Config
)

// Инициализация модбас контроллера
func initController() (ctrl *controller.Controller, err error) {
	log.Println("Configuring modbus controller " + *modbusTcpAddr)
	ctrl, err = controller.New(&controller.Configuration{
		Url:         ControllerConfig.DeviceUrl,
		DeviceId:    ControllerConfig.DeviceId,
		Speed:       ControllerConfig.Speed,
		Timeout:     ControllerConfig.Timeout,
		PollingTime: ControllerConfig.PollingTime,
		ReadPeriod:  ControllerConfig.ReadPeriod,
	})
	if err != nil {
		log.Println(err.Error())
		os.Exit(1)
	}

	for _, tag := range ControllerConfig.Tags {
		ctrl.AddTag(&controller.Tag{Name: tag.Name, DisplayName: tag.Desc, Address: tag.Address, Method: controller.ParseOperation(tag.Operation)})
	}

	return
}

// Инициализация сервера http для выдачи состояния и метрик
func initHttpServer(ctrl *controller.Controller) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/tags", TagsHahdler(ctrl))
	mux.Handle("/metrics", MetricsHandler())

	return mux
}

func ParseFlags() {
	flag.CommandLine.SetOutput(os.Stdout)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `%s %s
Usage: %s [options]

`, APP, VERSION, APP)
		flag.PrintDefaults()
	}
	flag.Parse()

	err := ValidateConfigPath(*config)
	if err != nil {
		log.Println("Cannot find config: " + err.Error())
		os.Exit(1)
	}

	ControllerConfig, err = NewConfig(*config)
	if err != nil {
		log.Println("Cannot parse config" + err.Error())
		os.Exit(1)
	}

	defaults.SetDefaults(ControllerConfig)
	if len(ControllerConfig.DeviceUrl) == 0 {
		ControllerConfig.DeviceUrl = *modbusTcpAddr
	}
}

func main() {
	ParseFlags()
	log.Println("Starting...")

	// Инициализация модбас конроллера
	ctrl, err := initController()
	if err != nil {
		log.Println("Can not init modbus device: " + err.Error())
		os.Exit(1)
	}

	// Запуск полера
	go ctrl.Poll()
	defer ctrl.Close()

	// Инициализация сервера
	mux := initHttpServer(ctrl)
	log.Println("Listening " + *httpListenAddr + " ...")
	err = http.ListenAndServe(*httpListenAddr, mux)
	if err != nil {
		log.Println("Can not listen http: " + err.Error())
		os.Exit(1)
	}

	os.Exit(0)
}
