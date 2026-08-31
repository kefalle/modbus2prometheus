package commands

import (
	"errors"
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"log"
	"math"
	"modbus2prometheus/controller"
	"strconv"
)

type UstCommand struct {
	ctrl           *controller.Controller
	curChatId      int64
	currentTagName string
}

func (u *UstCommand) Command() string {
	return "sust"
}

func (u *UstCommand) Description() string {
	return "Установка переменных отопления"
}

func (u *UstCommand) Reply() string {
	u.currentTagName = ""
	return ""
}

func chunkSlice(slice []tgbotapi.InlineKeyboardButton, chunkSize int) [][]tgbotapi.InlineKeyboardButton {
	var chunks [][]tgbotapi.InlineKeyboardButton
	for {
		if len(slice) == 0 {
			break
		}

		// necessary check to avoid slicing beyond
		// slice capacity
		if len(slice) < chunkSize {
			chunkSize = len(slice)
		}

		chunks = append(chunks, slice[0:chunkSize])
		slice = slice[chunkSize:]
	}

	return chunks
}

func parseSetpoint(text string) (float64, error) {
	value, err := strconv.ParseFloat(text, 32)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, errors.New("setpoint must be finite")
	}

	return value, nil
}

func setpointSuccessMessage(value float64) string {
	return "Значение успешно установлено: " + strconv.FormatFloat(value, 'f', -1, 32)
}

func (u *UstCommand) Action(bot *tgbotapi.BotAPI, update tgbotapi.Update) bool {
	if u.currentTagName == "" { // Спрашиваем тип уставки
		var buttons []tgbotapi.InlineKeyboardButton
		for _, tag := range u.ctrl.Snapshot() {
			if tag.Group == "ust" {
				buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData(tag.GetName(), tag.Name))
				//row := tgbotapi.NewInlineKeyboardRow()
				//keyboard = append(keyboard, row)
			}
		}

		var keyboard = chunkSlice(buttons, 2)

		u.curChatId = update.Message.Chat.ID
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, update.Message.Text)
		msg.ReplyMarkup = tgbotapi.InlineKeyboardMarkup{
			InlineKeyboard: keyboard,
		}
		// Send the message.
		if _, err := bot.Send(msg); err != nil {
			log.Printf("Telegram send err: %s", err.Error())
		}
	} else {
		// Пытаемся изменить значение
		val, err := parseSetpoint(update.Message.Text)
		if err != nil {
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Введено не корректное значение!")
			if _, err := bot.Send(msg); err != nil {
				log.Printf("Telegram send err: %s", err.Error())
			}
			return true
		}

		var text string
		if err := u.ctrl.WriteTagByName(u.currentTagName, val); err != nil {
			text = "Ошибка записи: " + err.Error()
		} else {
			text = setpointSuccessMessage(val)
		}

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)
		// Send the message.
		if _, err := bot.Send(msg); err != nil {
			log.Printf("Telegram send err: %s", err.Error())
		}
		return true
	}

	return false
}

func (u *UstCommand) Callback(bot *tgbotapi.BotAPI, update tgbotapi.Update) (bool, error) {
	// Respond to the callback query, telling Telegram to show the user
	// a message with the data received.
	callback := tgbotapi.NewCallback(update.CallbackQuery.ID, update.CallbackQuery.Data)
	if _, err := bot.Request(callback); err != nil {
		return true, fmt.Errorf("acknowledge Telegram callback: %w", err)
	}

	tagName := update.CallbackQuery.Data
	text := "Введите значени:"
	var selected controller.TagSnapshot
	found := false
	for _, tag := range u.ctrl.Snapshot() {
		if tag.Name == tagName {
			selected = tag
			found = true
			break
		}
	}
	if !found {
		text = "Выбран не корректный тег " + tagName
	} else if !selected.Writable {
		text = "Тег " + tagName + " не может быть записан, см. конфигурацию"
	} else {
		u.currentTagName = selected.Name
		text = "Введите значени для " + selected.GetName() + ":"
	}

	// And finally, send a message containing the data received.
	msg := tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, text)
	if _, err := bot.Send(msg); err != nil {
		return true, fmt.Errorf("send Telegram setpoint prompt: %w", err)
	}

	return false, nil
}

func NewUstCommand(ctrl *controller.Controller) *UstCommand {
	return &UstCommand{ctrl: ctrl}
}
