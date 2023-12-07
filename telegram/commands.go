package telegram

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"log"
	"modbus2prometheus/controller"
	"strconv"
)

type ICommand interface {
	Command() string
	Description() string
	Reply() string
	Action(bot *tgbotapi.BotAPI, update tgbotapi.Update) bool
	Callback(bot *tgbotapi.BotAPI, update tgbotapi.Update) bool
}

// SimpleCommandConf Описание простой команды
type SimpleCommandConf struct {
	CommandStr     string
	DescriptionStr string

	// Возвращается когда action=true
	ReplyStr string
	// Возвращается когда action=true
	ReplyFunc func() string
	// Действие, если оно возвращает true, значит можно завершить Reply или ReplyFunc, если false то будем ждать Callback
	ActionFunc func(bot *tgbotapi.BotAPI, update tgbotapi.Update) bool
	// Колбек на действие
	CallbackFunc func(bot *tgbotapi.BotAPI, update tgbotapi.Update) bool
}

// SimpleCommand Класс для простых команд без дополнительных действий
type SimpleCommand struct {
	SimpleCommandConf
}

func NewSimpleCommand(c *SimpleCommandConf) (cmd *SimpleCommand) {
	cmd = &SimpleCommand{*c}
	return
}

func (cmd *SimpleCommand) Command() string {
	return cmd.CommandStr
}

func (cmd *SimpleCommand) Description() string {
	return cmd.DescriptionStr
}

func (cmd *SimpleCommand) Reply() string {
	text := cmd.ReplyStr
	if cmd.ReplyFunc != nil {
		text = cmd.ReplyFunc()
	}

	return text
}

func (cmd *SimpleCommand) Action(bot *tgbotapi.BotAPI, update tgbotapi.Update) bool {
	if cmd.ActionFunc != nil {
		return cmd.ActionFunc(bot, update)
	}

	return true
}

func (cmd *SimpleCommand) Callback(bot *tgbotapi.BotAPI, update tgbotapi.Update) bool {
	if cmd.CallbackFunc != nil {
		return cmd.CallbackFunc(bot, update)
	}

	return true
}

type UstCommand struct {
	ctrl       *controller.Controller
	curChatId  int64
	currentTag *controller.Tag
}

func (u *UstCommand) Command() string {
	return "sust"
}

func (u *UstCommand) Description() string {
	return "Установка переменных отопления"
}

func (u *UstCommand) Reply() string {
	u.currentTag = nil
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

func (u *UstCommand) Action(bot *tgbotapi.BotAPI, update tgbotapi.Update) bool {
	if u.currentTag == nil { // Спрашиваем тип уставки
		var buttons []tgbotapi.InlineKeyboardButton
		for _, tag := range u.ctrl.Tags() {
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
		text := "Значение устновлено "
		// Пытаемся изменить значение
		val, err := strconv.ParseFloat(update.Message.Text, 32)
		if err != nil {
			text = "Введено не корректное значение!"
		}

		err = u.ctrl.WriteTag(u.currentTag, val)
		if err != nil {
			text = "Ошибка записи: " + err.Error()
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

func (u *UstCommand) Callback(bot *tgbotapi.BotAPI, update tgbotapi.Update) bool {
	// Respond to the callback query, telling Telegram to show the user
	// a message with the data received.
	callback := tgbotapi.NewCallback(update.CallbackQuery.ID, update.CallbackQuery.Data)
	if _, err := bot.Request(callback); err != nil {
		panic(err)
	}

	text := "Введите значение"
	tagName := update.CallbackQuery.Data
	u.currentTag = u.ctrl.FindTag(tagName)
	if u.currentTag == nil {
		text = "Выбран не корректный тег " + tagName
	} else if !controller.Writable(u.currentTag) {
		text = "Тег " + tagName + " не может быть записан, см. конфигурацию"
	}

	// And finally, send a message containing the data received.
	msg := tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, text)
	if _, err := bot.Send(msg); err != nil {
		panic(err)
	}

	return false
}

func NewUstCommand(ctrl *controller.Controller) *UstCommand {
	return &UstCommand{ctrl: ctrl}
}
