package commands

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestParseDataDetailsUsesPressure(t *testing.T) {
	got := parseData("details", []SensorJson{{
		Name: "room",
		Data: SensorData{Humidity: 45.2, Pressure: 748.1},
	}})

	if !strings.Contains(got, "P:748.1 mmR") {
		t.Fatalf("details must contain pressure, got %q", got)
	}
}

func TestFetchSensorsReturnsRequestError(t *testing.T) {
	want := errors.New("node-red unavailable")
	cmd := NewSensorsCommand("http://node-red/current_th")
	cmd.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, want
	})

	_, err := cmd.fetchSensors(context.Background())
	if !errors.Is(err, want) {
		t.Fatalf("want %v, got %v", want, err)
	}
}

func TestFetchSensorsRejectsNonSuccessfulStatus(t *testing.T) {
	cmd := NewSensorsCommand("http://node-red/current_th")
	cmd.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Status:     "503 Service Unavailable",
			Body:       io.NopCloser(strings.NewReader("[]")),
		}, nil
	})

	_, err := cmd.fetchSensors(context.Background())
	if err == nil {
		t.Fatal("want an error for non-successful HTTP status")
	}
}
