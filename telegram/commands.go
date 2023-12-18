package telegram

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
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
