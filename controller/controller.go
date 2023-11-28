package controller

import (
	"github.com/VictoriaMetrics/metrics"
	"github.com/mcuadros/go-defaults"
	"github.com/simonvetter/modbus"
	"log"
	"os"
	"strings"
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
	modbusClient *modbus.ModbusClient
	tags         []*Tag
	exit         bool

	// metrics
	errCounter *metrics.Counter
	reqCounter *metrics.Counter
}

func defaultFloat32Action(val interface{}, t *Tag) {
	if t.LastValue != val {
		v := val.(float32)
		log.Printf("req %d tag %s = %f", t.controller.reqCounter.Get(), t.Name, v)
		t.LastValue = v
	}
}

func defaultUint16Action(val interface{}, t *Tag) {
	if t.LastValue != val {
		v := val.(uint16)
		log.Printf("req %d tag %s = %d", t.controller.reqCounter.Get(), t.Name, v)
		t.LastValue = val
	}
}

func ParseOperation(op string) (t uint8) {
	var res uint8 = 0

	if strings.Contains(op, "read_uint") {
		res |= READ_UINT
	}
	if strings.Contains(op, "read_float") {
		res |= READ_FLOAT
	}
	if strings.Contains(op, "write_uint") {
		res |= WRITE_UINT
	}
	if strings.Contains(op, "write_float") {
		res |= WRITE_FLOAT
	}

	if res > 0 {
		return res
	}

	log.Println("Unsupported operation " + op + " must be read_uint, read_float")
	os.Exit(1)

	return 0
}

func New(conf *Configuration) (c *Controller, err error) {
	defaults.SetDefaults(conf)
	c = &Controller{
		conf: *conf,
	}

	// Создаем метрики
	c.reqCounter = metrics.NewCounter("req_counter")
	c.errCounter = metrics.NewCounter("err_counter")

	// for an RTU over TCP device/bus (remote serial port or
	// simple TCP-to-serial bridge)
	c.modbusClient, err = modbus.NewClient(&modbus.ClientConfiguration{
		URL:     c.conf.Url,
		Speed:   c.conf.Speed, // serial link speed
		Timeout: c.conf.Timeout,
	})
	if err != nil {
		return
	}

	err = c.modbusClient.SetUnitId(c.conf.DeviceId)
	if err != nil {
		return
	}

	err = c.modbusClient.Open()

	return
}

func (c *Controller) AddTag(tag *Tag) {
	c.Lock()
	defer c.Unlock()

	tag.Gauge = metrics.NewGauge(tag.Name, func() float64 {
		c.RLock()
		defer c.RUnlock()
		if tag.LastValue != nil {
			if (tag.Method & READ_UINT) == READ_UINT {
				return float64(tag.LastValue.(uint16))
			} else if (tag.Method & READ_FLOAT) == READ_FLOAT {
				return float64(tag.LastValue.(float32))
			}
		}
		return 0.0
	})

	if tag.Action == nil {
		if (tag.Method & READ_UINT) == READ_UINT {
			tag.Action = defaultUint16Action
		} else if (tag.Method & READ_FLOAT) == READ_FLOAT {
			tag.Action = defaultFloat32Action
		}
	}

	tag.controller = c

	c.tags = append(c.tags, tag)
}

func (c *Controller) Close() {
	c.exit = true
}

func (c *Controller) incCounter() {
	c.reqCounter.Inc()
}

func (c *Controller) incErrCounter() {
	c.errCounter.Inc()
}

func (c *Controller) Poll() {
	log.Println("Start polling...")

	var failAttempts uint = 0
	c.exit = false
	needRestart := false
	for {
		// Дали команду на выход или количество ошибок превысило ограничение чтобы выйти
		if c.exit || failAttempts >= c.conf.MaxAttempts {
			break
		}

		for i, tag := range c.tags {
			// Принудительный рестарт
			if needRestart {
				log.Println("Restarting connect...")
				err := c.modbusClient.Open()
				if err != nil {
					log.Println("Can not open connect")
					break
				}
				needRestart = false
				failAttempts += 1
			}

			time.Sleep(c.conf.ReadPeriod)

			c.Lock()
			var err error
			var val interface{}

			if tag.Action != nil {
				if (tag.Method & READ_UINT) == READ_UINT {
					val, err = c.modbusClient.ReadRegister(tag.Address, modbus.HOLDING_REGISTER)
					c.incCounter()
				} else if (tag.Method & READ_FLOAT) == READ_FLOAT {
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
				c.modbusClient.Close()
				c.Unlock()
				time.Sleep(c.conf.ErrTimeout) // Добавляем задержку, чтобы сломанный пакет протух
				break
			}
			tag.Action(val, c.tags[i])
			failAttempts = 0 // Сбрасываем счетчик попыток
			c.Unlock()
		}
		time.Sleep(c.conf.PollingTime)
	}

	log.Println("End polling")
	err := c.modbusClient.Close()
	if err != nil {
		log.Println("Controller close error: " + err.Error())
	}

	os.Exit(2)
}
