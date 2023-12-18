package commands

import (
	"encoding/json"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type SensorData struct {
	Battery     float64 `json:"battery"`
	Humidity    float64 `json:"humidity"`
	Pressure    float64 `json:"pressure"`
	Temperature float64 `json:"temperature"`
}

type SensorJson struct {
	Name string     `json:"name"`
	Data SensorData `json:"data"`
}

type SensorsCommand struct {
	NodeRedUrl string
	client     *http.Client
	currentVar string
	sync.RWMutex
}

func NewSensorsCommand(nodeRedUrl string) *SensorsCommand {
	return &SensorsCommand{
		NodeRedUrl: nodeRedUrl,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *SensorsCommand) Command() string {
	return "sens_th"
}

func (s *SensorsCommand) Description() string {
	return "Датчики умного дома"
}

func (s *SensorsCommand) Reply() string {
	s.currentVar = ""
	return ""
}

func (s *SensorsCommand) Action(bot *tgbotapi.BotAPI, update tgbotapi.Update) bool {
	if s.currentVar == "" {
		var keyboard = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Температура", "temp"),
				tgbotapi.NewInlineKeyboardButtonData("Влажность", "humi"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Заряд", "battery"),
				tgbotapi.NewInlineKeyboardButtonData("Всё", "details"),
			),
		)

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, update.Message.Text)
		msg.ReplyMarkup = keyboard
		// Send the message.
		if _, err := bot.Send(msg); err != nil {
			log.Printf("Telegram send err: %s", err.Error())
		}

		return false
	}

	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Ждал кнопку... а не текст! Давай заново")
	// Send the message.
	if _, err := bot.Send(msg); err != nil {
		log.Printf("Telegram send err: %s", err.Error())
	}

	return true
}

func float2str(v float64) string {
	return strconv.FormatFloat(v, 'f', 1, 64)
}

func parseData(button string, data []SensorJson) (text string) {

	for _, sensor := range data {
		if data == nil {
			continue
		}

		if button == "temp" {
			text += sensor.Name + ": " + float2str(sensor.Data.Temperature) + " c\n"
		} else if button == "humi" {
			text += sensor.Name + ": " + float2str(sensor.Data.Humidity) + "%RH\n"
		} else if button == "battery" {
			text += sensor.Name + ": " + float2str(sensor.Data.Battery) + "%\n"
		} else if button == "details" {
			text += sensor.Name + "\n"
			text += "    Т:" + float2str(sensor.Data.Temperature) + " c\n"
			text += "    H:" + float2str(sensor.Data.Humidity) + " %RH\n"
			text += "    P:" + float2str(sensor.Data.Humidity) + " mmR\n"
			text += "    B:" + float2str(sensor.Data.Battery) + " %\n"
		}
	}

	return text
}

func (s *SensorsCommand) Callback(bot *tgbotapi.BotAPI, update tgbotapi.Update) bool {
	s.Lock()
	defer s.Unlock()

	var text = "Что-то странное произошло..."

	resp, err := s.client.Get(s.NodeRedUrl)
	if err != nil {
		text = "Ошибка запроса данных: " + err.Error()
	}
	defer resp.Body.Close()

	var sensors []SensorJson
	err = json.NewDecoder(resp.Body).Decode(&sensors)
	if err != nil {
		text = "Ошибка обработки данных: " + err.Error()
	}

	res := parseData(update.CallbackQuery.Data, sensors)
	if res != "" {
		text = res
	}

	msg := tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, text)
	// Send the message.
	if _, err := bot.Send(msg); err != nil {
		log.Printf("Telegram send err: %s", err.Error())
	}

	return true
}
