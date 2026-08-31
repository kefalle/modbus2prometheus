package telegram

import (
	"errors"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestNewWithBotFactoryReturnsInitializationError(t *testing.T) {
	want := errors.New("telegram unavailable")
	factory := func(string) (*tgbotapi.BotAPI, error) {
		return nil, want
	}

	err := newWithBotFactory(BotConfig{}, factory)
	if !errors.Is(err, want) {
		t.Fatalf("newWithBotFactory error = %v, want error wrapping %v", err, want)
	}
}

func TestBuildBotCommandsUsesNamesWithoutSlash(t *testing.T) {
	api := []ICommand{NewSimpleCommand(&SimpleCommandConf{
		CommandStr:     "state",
		DescriptionStr: "Show state",
	})}

	_, commands := buildBotCommands(api)
	if len(commands) != 1 {
		t.Fatalf("commands count = %d, want 1", len(commands))
	}
	if commands[0].Command != "state" {
		t.Fatalf("command name = %q, want %q", commands[0].Command, "state")
	}
}
