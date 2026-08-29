package controller

import (
	"context"
	"errors"
	"fmt"
	"github.com/VictoriaMetrics/metrics"
	"github.com/mcuadros/go-defaults"
	"github.com/simonvetter/modbus"
	"log"
	"sync"
	"time"
)

type OperationType uint

const (
	READ_UINT   = 0x1
	READ_FLOAT  = 0x2
	WRITE_UINT  = 0x4
	WRITE_FLOAT = 0x8
)

type logger struct {
	prefix       string
	customLogger *log.Logger
}

type Configuration struct {
	DeviceId    uint8 `default:"16"`
	Url         string
	Speed       uint          `default:"19200"`
	Timeout     time.Duration `default:"1s"`
	PollingTime time.Duration `default:"1s"`
	ReadPeriod  time.Duration `default:"20ms"`
	ErrTimeout  time.Duration `default:"500ms"`
	MaxAttempts uint          `default:"20"`
}

type Controller struct {
	sync.RWMutex
	conf         Configuration
	logger       *logger
	modbusClient registerClient
	metricsSet   *metrics.Set
	tags         []*Tag

	// metrics
	errCounter *metrics.Counter
	reqCounter *metrics.Counter
}

func New(conf *Configuration) (c *Controller, err error) {
	defaults.SetDefaults(conf)
	client, err := newRegisterClient(*conf)
	if err != nil {
		return nil, err
	}

	set := metrics.NewSet()
	metrics.RegisterSet(set)

	return newWithClient(*conf, client, set), nil
}

func newWithClient(conf Configuration, client registerClient, set *metrics.Set) *Controller {
	return &Controller{
		conf:         conf,
		modbusClient: client,
		metricsSet:   set,
		reqCounter:   set.NewCounter("req_counter"),
		errCounter:   set.NewCounter("err_counter"),
	}
}

func (c *Controller) FindTag(name string) *Tag {
	c.RLock()
	defer c.RUnlock()

	for i, tag := range c.tags {
		if tag.Name == name {
			return c.tags[i]
		}
	}

	return nil
}

func (c *Controller) Snapshot() []TagSnapshot {
	c.RLock()
	defer c.RUnlock()

	snapshot := make([]TagSnapshot, len(c.tags))
	for i, tag := range c.tags {
		snapshot[i] = TagSnapshot{
			Name:        tag.Name,
			DisplayName: tag.DisplayName,
			Group:       tag.Group,
			Address:     tag.Address,
			Value:       tag.LastValue,
			Writable:    Writable(tag),
		}
	}

	return snapshot
}

func (c *Controller) AddTag(tag *Tag) {
	c.Lock()
	defer c.Unlock()

	tag.Gauge = c.metricsSet.NewGauge(tag.Name, func() float64 {
		c.RLock()
		defer c.RUnlock()
		if tag.LastValue != nil {
			if isUint(tag) {
				return float64(tag.LastValue.(uint16))
			} else if isFloat(tag) {
				return float64(tag.LastValue.(float32))
			}
		}
		return 0.0
	})

	if tag.Action == nil {
		if isUint(tag) {
			tag.Action = defaultUint16Action
		} else if isFloat(tag) {
			tag.Action = defaultFloat32Action

		}
	}
	tag.controller = c

	c.tags = append(c.tags, tag)
}

func (c *Controller) WriteTag(tag *Tag, value float64) (err error) {
	// Пробуем записать
	if isWriteUint(tag) {
		err = c.modbusClient.WriteRegister(tag.Address, uint16(value))
	} else if isWriteFloat(tag) {
		err = c.modbusClient.WriteFloat32(tag.Address, float32(value))
	}

	return
}

func (c *Controller) WriteTagByName(name string, value float64) error {
	tag := c.FindTag(name)
	if tag == nil {
		return fmt.Errorf("tag %q not found", name)
	}
	if !Writable(tag) {
		return fmt.Errorf("tag %q is not writable", name)
	}

	return c.WriteTag(tag, value)
}

func (c *Controller) Close() error {
	return c.modbusClient.Close()
}

func (c *Controller) incCounter() {
	c.reqCounter.Inc()
}

func (c *Controller) incErrCounter() {
	c.errCounter.Inc()
}

func wait(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *Controller) Run(ctx context.Context) (runErr error) {
	log.Println("Start polling...")
	defer func() {
		if err := c.Close(); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("close Modbus client: %w", err))
		}
	}()

	var failAttempts uint = 0
	needRestart := false
polling:
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		for i, tag := range c.tags {
			// Принудительный рестарт
			if needRestart {
				log.Println("Restarting connect...")
				err := c.modbusClient.Open()
				if err != nil {
					log.Println("Can not open connect")
					failAttempts++
					if failAttempts >= c.conf.MaxAttempts {
						return fmt.Errorf("reconnect failed after %d attempts: %w", failAttempts, err)
					}
					if err := wait(ctx, c.conf.ErrTimeout); err != nil {
						return err
					}
					continue polling
				}
				needRestart = false
			}

			if err := wait(ctx, c.conf.ReadPeriod); err != nil {
				return err
			}

			c.Lock()
			var err error
			var val interface{}

			if tag.Action != nil {
				if isUint(tag) {
					val, err = c.modbusClient.ReadRegister(tag.Address, modbus.HOLDING_REGISTER)
					c.incCounter()
				} else if isFloat(tag) {
					val, err = c.modbusClient.ReadFloat32(tag.Address, modbus.HOLDING_REGISTER)
					c.incCounter()
				}
			}

			// Обработка ошибок
			if err != nil {
				c.incErrCounter()
				log.Printf("Req %d error get tag %s err: %s", c.reqCounter.Get(), tag.Name, err.Error())

				//if cause, ok := err.(interface{ Unwrap() error }); ok {
				//	if _, ok := cause.(net.Error); ok {
				//		needRestart = true
				//	}
				//}
				needRestart = true
				if closeErr := c.modbusClient.Close(); closeErr != nil {
					log.Printf("Controller close error: %s", closeErr.Error())
				}
				c.Unlock()
				if err := wait(ctx, c.conf.ErrTimeout); err != nil {
					return err
				}
				break
			}
			tag.Action(val, c.tags[i])
			failAttempts = 0 // Сбрасываем счетчик попыток
			c.Unlock()
		}
		if err := wait(ctx, c.conf.PollingTime); err != nil {
			return err
		}
	}
}

func (c *Controller) Poll() {
	if err := c.Run(context.Background()); err != nil {
		log.Printf("Controller polling stopped: %s", err.Error())
	}
}
