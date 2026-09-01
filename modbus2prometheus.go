package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/mcuadros/go-defaults"
	"log"
	"modbus2prometheus/controller"
	"modbus2prometheus/telegram"
	"modbus2prometheus/telegram/commands"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const APP = "modbus2prometheus"
const VERSION = "0.0.2"

var (
	httpListenAddr = flag.String("httpListenAddr", ":9101", "TCP address to listen for http connections.")
	modbusTcpAddr  = flag.String("modbusTcpAddr", "rtuovertcp://192.168.1.200:8899", "TCP address to modbus device with modbus TCP.")
	configPath     = flag.String("config", "./config.yaml", "Modbus controller configuration")
	maxAttempts    = flag.Uint("maxAttempts", 100, "Max attempts before fail exit")
	botApiToken    = flag.String("botApiToken", "", "Telegram bot API token")

	config *Config
)

// Инициализация модбас контроллера
func initController() (ctrl *controller.Controller, err error) {
	log.Println("Configuring modbus controller " + *modbusTcpAddr)
	methods := make([]uint8, len(config.Tags))
	for i, tag := range config.Tags {
		method, err := controller.ParseOperation(tag.Operation)
		if err != nil {
			return nil, fmt.Errorf("parse operation for tag %q: %w", tag.Name, err)
		}
		methods[i] = method
	}

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
		return nil, err
	}

	for i, tag := range config.Tags {
		ctrl.AddTag(&controller.Tag{
			Name:        tag.Name,
			DisplayName: tag.Desc,
			Group:       tag.Group,
			Address:     tag.Address,
			Method:      methods[i]})
	}

	return ctrl, nil
}

// Инициализация сервера http для выдачи состояния и метрик
func initHttpServer(ctrl *controller.Controller, writeBearerToken string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/tags", controller.TagsHahdler(ctrl))
	mux.Handle("/api/v1/write", controller.BearerAuth(writeBearerToken, ctrl.WriteTagsHandler()))
	mux.Handle("/metrics", MetricsHandler())

	return mux
}

func newHTTPServer(address string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

// initTelegram инициализация телеграм бота из конфига
func initTelegram(ctrl *controller.Controller) error {

	listFn := func(group string) func() string {
		return func() string {
			var repl string
			for _, tag := range ctrl.Snapshot() {
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
		commands.NewUstCommand(ctrl),
		commands.NewSensorsCommand(config.Telegram.NodeRedUrl + "/current_th"),
	}

	return telegram.New(telegram.BotConfig{
		BotToken: config.Telegram.ApiToken,
		Owners:   config.Telegram.Owners,
		Api:      apiCommands,
		Ctrl:     ctrl,
	})
}

func ParseFlags() error {
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
		return fmt.Errorf("validate config path: %w", err)
	}

	config, err = NewConfig(*configPath)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	defaults.SetDefaults(config)
	if len(config.DeviceUrl) == 0 {
		config.DeviceUrl = *modbusTcpAddr
	}

	if *botApiToken != "" {
		config.Telegram.ApiToken = *botApiToken
	}

	return nil
}

func run() error {
	if err := ParseFlags(); err != nil {
		return err
	}
	log.Println("Starting...")

	// Инициализация модбас конроллера
	ctrl, err := initController()
	if err != nil {
		return fmt.Errorf("init Modbus device: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Запуск телеграм бота, управления домом
	if err := initTelegram(ctrl); err != nil {
		return errors.Join(fmt.Errorf("init Telegram bot: %w", err), ctrl.Close())
	}

	controllerErr := make(chan error, 1)
	go func() {
		controllerErr <- ctrl.Run(ctx)
	}()

	// Инициализация сервера
	mux := initHttpServer(ctrl, config.HTTP.WriteBearerToken)
	server := newHTTPServer(*httpListenAddr, mux)
	serverErr := make(chan error, 1)
	log.Println("Listening " + *httpListenAddr + " ...")
	go func() {
		serverErr <- server.ListenAndServe()
	}()

	var runErr error
	controllerStopped := false
	serverStopped := false
	select {
	case <-ctx.Done():
	case err := <-controllerErr:
		controllerStopped = true
		if err != nil && !errors.Is(err, context.Canceled) {
			runErr = fmt.Errorf("controller stopped: %w", err)
		}
	case err := <-serverErr:
		serverStopped = true
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			runErr = fmt.Errorf("HTTP server stopped: %w", err)
		}
	}
	stop()

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		runErr = errors.Join(runErr, fmt.Errorf("shutdown HTTP server: %w", err))
	}

	if !controllerStopped {
		select {
		case err := <-controllerErr:
			if err != nil && !errors.Is(err, context.Canceled) {
				runErr = errors.Join(runErr, fmt.Errorf("controller stopped: %w", err))
			}
		case <-shutdownCtx.Done():
			runErr = errors.Join(runErr, fmt.Errorf("wait for controller shutdown: %w", shutdownCtx.Err()))
		}
	}

	if !serverStopped {
		select {
		case err := <-serverErr:
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				runErr = errors.Join(runErr, fmt.Errorf("HTTP server stopped: %w", err))
			}
		case <-shutdownCtx.Done():
			runErr = errors.Join(runErr, fmt.Errorf("wait for HTTP server shutdown: %w", shutdownCtx.Err()))
		}
	}

	return runErr
}

func main() {
	if err := run(); err != nil {
		log.Println(err.Error())
		os.Exit(1)
	}
}
