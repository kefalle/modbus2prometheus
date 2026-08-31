package telegram

import (
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"log"
	"modbus2prometheus/controller"
	"strings"
	"time"
)

type BotConfig struct {
	BotToken string
	Owners   map[int64]string
	Api      []ICommand
	Ctrl     *controller.Controller
}

type BotState struct {
	BotConfig
	lastCommandTime time.Time        // Время вызова команды
	currentCommand  ICommand         // Текущая команда, если nil то ждем любую
	bot             *tgbotapi.BotAPI // Бот
}

type botFactory func(string) (*tgbotapi.BotAPI, error)

func reply(bot *tgbotapi.BotAPI, update tgbotapi.Update, cmd ICommand) {
	text := cmd.Reply()

	var chatId int64
	if update.Message != nil {
		chatId = update.Message.Chat.ID
	} else if update.CallbackQuery != nil {
		chatId = update.CallbackQuery.Message.Chat.ID
	} else {
		log.Printf("Unknown update type")
		return
	}

	// fmt.Println("RESPONSE", text)
	if strings.TrimSpace(text) != "" {
		msg := tgbotapi.NewMessage(chatId, strings.TrimSpace(text))
		_, err := bot.Send(msg)
		if err != nil {
			log.Printf("Telegram send error: %s", err.Error())
		}
	}
}

func New(conf BotConfig) error {
	return newWithBotFactory(conf, tgbotapi.NewBotAPI)
}

func buildBotCommands(api []ICommand) (map[string]ICommand, []tgbotapi.BotCommand) {
	commandMap := make(map[string]ICommand)
	var botCommands []tgbotapi.BotCommand

	for _, v := range api {
		commandMap[v.Command()] = v
		botCommands = append(botCommands, tgbotapi.BotCommand{
			Command:     v.Command(),
			Description: v.Description(),
		})
	}
	return commandMap, botCommands
}

func newWithBotFactory(conf BotConfig, factory botFactory) error {
	commandMap, botCommands := buildBotCommands(conf.Api)
	bot, err := factory(conf.BotToken)
	if err != nil {
		return fmt.Errorf("initialize Telegram bot: %w", err)
	}
	state := BotState{conf, time.Now(), nil, bot}
	bot.Debug = true

	log.Printf("Authorized on account %s", bot.Self.UserName)

	command := tgbotapi.NewSetMyCommands(botCommands...)
	_, err = bot.Request(command)
	if err != nil {
		return fmt.Errorf("register Telegram commands: %w", err)
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	go func() {
		updates := bot.GetUpdatesChan(u)
		delta, _ := time.ParseDuration("5m")

		for update := range updates {
			// Автоматический сброс если у нас ничего не произошло
			if time.Now().Sub(state.lastCommandTime) >= delta && state.currentCommand != nil {
				state.currentCommand = nil
			}

			// Тут только ждем команды
			if update.Message != nil {
				// Обрабатываем только типы Message
				_, exists := conf.Owners[update.Message.From.ID]
				if !exists {
					continue
				}

				log.Printf("[%d:%s] %s", update.Message.Chat.ID, update.Message.From.UserName, update.Message.Text)

				if update.Message.IsCommand() {
					if v, exists := commandMap[update.Message.Command()]; exists {
						// Если команда вернула false, значит требуется дополнительная обработка
						if !v.Action(bot, update) {
							state.currentCommand = commandMap[update.Message.Command()]
							state.lastCommandTime = time.Now()
							continue
						}

						reply(bot, update, v)
						state.currentCommand = nil
					}
				} else {
					// Обработка текста через Action
					if state.currentCommand != nil && state.currentCommand.Action(bot, update) {
						reply(bot, update, state.currentCommand)
						state.currentCommand = nil
					}
				}
			} else if update.CallbackQuery != nil && state.currentCommand != nil { // Пришло нажатие на inline кнопку
				done, err := state.currentCommand.Callback(bot, update)
				if err != nil {
					log.Printf("Telegram callback error: %s", err.Error())
					state.currentCommand = nil
					continue
				}
				if !done {
					continue
				}
				reply(bot, update, state.currentCommand)
				state.currentCommand = nil
			}
		}

		log.Printf("Telegram updates channel closed")
	}()

	return nil
}
