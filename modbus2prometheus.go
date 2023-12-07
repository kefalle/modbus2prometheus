package main

import (
	"flag"
	"fmt"
	"github.com/mcuadros/go-defaults"
	"log"
	"modbus2prometheus/controller"
	"modbus2prometheus/telegram"
	"net/http"
	"os"
)

const APP = "modbus2prometheus"
const VERSION = "0.0.2"

var (
	httpListenAddr = flag.String("httpListenAddr", ":9101", "TCP address to listen for http connections.")
	modbusTcpAddr  = flag.String("modbusTcpAddr", "rtuovertcp://192.168.1.200:8899", "TCP address to modbus device with modbus TCP.")
	configPath     = flag.String("config", "./config.yaml", "Modbus controller configuration")
	maxAttempts    = flag.Uint("maxAttempts", 20, "Max attempts before fail exit")

	config *Config
)

// Инициализация модбас контроллера
func initController() (ctrl *controller.Controller, err error) {
	log.Println("Configuring modbus controller " + *modbusTcpAddr)
	ctrl, err = controller.New(&controller.Configuration{
		Url:         config.DeviceUrl,
		DeviceId:    config.DeviceId,
		Speed:       config.Speed,
		Timeout:     config.Timeout,
		PollingTime: config.PollingTime,
		ReadPeriod:  config.ReadPeriod,
		MaxAttempts: *maxAttempts,
	})
	if err != nil {
		log.Println(err.Error())
		os.Exit(1)
	}

	for _, tag := range config.Tags {
		ctrl.AddTag(&controller.Tag{
			Name:        tag.Name,
			DisplayName: tag.Desc,
			Group:       tag.Group,
			Address:     tag.Address,
			Method:      controller.ParseOperation(tag.Operation)})
	}

	return
}

// Инициализация сервера http для выдачи состояния и метрик
func initHttpServer(ctrl *controller.Controller) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/tags", controller.TagsHahdler(ctrl))
	mux.Handle("/api/v1/write", ctrl.WriteTagsHandler())
	mux.Handle("/metrics", MetricsHandler())

	return mux
}

// initTelegram инициализация телеграм бота из конфига
func initTelegram(ctrl *controller.Controller) {

	listFn := func(group string) func() string {
		return func() string {
			var repl string
			for _, tag := range ctrl.Tags() {
				if group == tag.Group || group == "" {
					if tag.DisplayName != "" {
						repl += tag.DisplayName + ": " + controller.ValToStr(tag) + "\n"
					} else {
						repl += tag.Name + ": " + controller.ValToStr(tag) + "\n"
					}
				}
			}
			return repl
		}
	}

	apiCommands := []telegram.ICommand{
		telegram.NewSimpleCommand(&telegram.SimpleCommandConf{
			CommandStr:     "state_all",
			DescriptionStr: "Отобразить все параметры",
			ReplyFunc:      listFn(""),
		}),
		telegram.NewSimpleCommand(&telegram.SimpleCommandConf{
			CommandStr:     "state",
			DescriptionStr: "Отобразить только измерения",
			ReplyFunc:      listFn("state"),
		}),
		telegram.NewSimpleCommand(&telegram.SimpleCommandConf{
			CommandStr:     "ust",
			DescriptionStr: "Отобразить только уставки",
			ReplyFunc:      listFn("ust"),
		}),
		telegram.NewUstCommand(ctrl),
	}

	telegram.New(telegram.BotConfig{config.Telegram.ApiToken, config.Telegram.Owners, apiCommands, ctrl})
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

	err := ValidateConfigPath(*configPath)
	if err != nil {
		log.Println("Cannot find configPath: " + err.Error())
		os.Exit(1)
	}

	config, err = NewConfig(*configPath)
	if err != nil {
		log.Println("Cannot parse configPath" + err.Error())
		os.Exit(1)
	}

	defaults.SetDefaults(config)
	if len(config.DeviceUrl) == 0 {
		config.DeviceUrl = *modbusTcpAddr
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

	// Запуск телеграм бота, управления домом
	initTelegram(ctrl)

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
