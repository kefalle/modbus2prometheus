package controller

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/VictoriaMetrics/metrics"
)

func TestWriteTagsHandlerContract(t *testing.T) {
	writeErr := errors.New("write failed")
	tests := []struct {
		name        string
		method      string
		body        string
		tagMethod   uint8
		addTag      bool
		writeErr    error
		wantStatus  int
		wantMessage string
		wantWrites  int
		wantAllow   string
	}{
		{
			name:        "method not allowed",
			method:      http.MethodGet,
			wantStatus:  http.StatusMethodNotAllowed,
			wantMessage: "method not allowed",
			wantAllow:   http.MethodPost,
		},
		{
			name:        "malformed json",
			method:      http.MethodPost,
			body:        "{",
			wantStatus:  http.StatusBadRequest,
			wantMessage: "invalid request body",
		},
		{
			name:        "unknown json field",
			method:      http.MethodPost,
			body:        `{"name":"setpoint","value":42,"extra":true}`,
			wantStatus:  http.StatusBadRequest,
			wantMessage: "invalid request body",
		},
		{
			name:        "body too large",
			method:      http.MethodPost,
			body:        strings.Repeat(" ", (1<<20)+1) + `{"name":"setpoint","value":42}`,
			addTag:      true,
			tagMethod:   WRITE_UINT,
			wantStatus:  http.StatusBadRequest,
			wantMessage: "invalid request body",
		},
		{
			name:        "unknown tag",
			method:      http.MethodPost,
			body:        `{"name":"missing","value":42}`,
			wantStatus:  http.StatusNotFound,
			wantMessage: "tag not found",
		},
		{
			name:        "read only tag",
			method:      http.MethodPost,
			body:        `{"name":"setpoint","value":42}`,
			addTag:      true,
			tagMethod:   READ_UINT,
			wantStatus:  http.StatusForbidden,
			wantMessage: "operation not permitted",
		},
		{
			name:        "modbus write error",
			method:      http.MethodPost,
			body:        `{"name":"setpoint","value":42}`,
			addTag:      true,
			tagMethod:   WRITE_UINT,
			writeErr:    writeErr,
			wantStatus:  http.StatusBadGateway,
			wantMessage: "modbus write failed",
			wantWrites:  1,
		},
		{
			name:       "valid write",
			method:     http.MethodPost,
			body:       `{"name":"setpoint","value":42}`,
			addTag:     true,
			tagMethod:  WRITE_UINT,
			wantStatus: http.StatusNoContent,
			wantWrites: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeRegisterClient{
				writeRegisterFn: func(uint16, uint16) error {
					return tt.writeErr
				},
			}
			ctrl := newWithClient(Configuration{}, client, metrics.NewSet())
			if tt.addTag {
				ctrl.AddTag(&Tag{Name: "setpoint", Method: tt.tagMethod})
			}

			req := httptest.NewRequest(tt.method, "/api/v1/write", strings.NewReader(tt.body))
			recorder := httptest.NewRecorder()
			ctrl.WriteTagsHandler().ServeHTTP(recorder, req)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%q", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
			if got := recorder.Header().Get("Allow"); got != tt.wantAllow {
				t.Fatalf("Allow = %q, want %q", got, tt.wantAllow)
			}
			if client.writeRegisterCalls != tt.wantWrites {
				t.Fatalf("WriteRegister calls = %d, want %d", client.writeRegisterCalls, tt.wantWrites)
			}

			if tt.wantMessage == "" {
				if recorder.Body.Len() != 0 {
					t.Fatalf("response body = %q, want empty", recorder.Body.String())
				}
				return
			}

			if got := recorder.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("Content-Type = %q, want application/json", got)
			}
			var response struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode error response: %v; body=%q", err, recorder.Body.String())
			}
			if response.Error != tt.wantMessage {
				t.Fatalf("error = %q, want %q", response.Error, tt.wantMessage)
			}
		})
	}
}
