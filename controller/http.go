package controller

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
)

const maxWriteBodyBytes = 1 << 20

type WriteTag struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
}

func TagsHahdler(c *Controller) http.HandlerFunc {
	fn := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		data, err := c.Json()
		if err != nil {
			log.Println("Cannot make json err: " + err.Error())
			return
		}

		_, err = w.Write(data)
		if err != nil {
			log.Println("Cannot send response")
		}
	}

	return fn
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": message}); err != nil {
		log.Printf("Cannot send JSON error response: %s", err.Error())
	}
}

func BearerAuth(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}

	expectedTokenHash := sha256.Sum256([]byte(token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scheme, providedToken, ok := strings.Cut(r.Header.Get("Authorization"), " ")
		providedTokenHash := sha256.Sum256([]byte(providedToken))
		if !ok || !strings.EqualFold(scheme, "Bearer") ||
			subtle.ConstantTimeCompare(providedTokenHash[:], expectedTokenHash[:]) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (c *Controller) WriteTagsHandler() http.HandlerFunc {
	fn := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		var writeTag WriteTag
		r.Body = http.MaxBytesReader(w, r.Body, maxWriteBodyBytes)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&writeTag); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			writeJSONError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		log.Printf("Request to write %s tag with value %f", writeTag.Name, writeTag.Value)
		if err := c.WriteTagByName(writeTag.Name, writeTag.Value); err != nil {
			switch {
			case errors.Is(err, ErrTagNotFound):
				writeJSONError(w, http.StatusNotFound, "tag not found")
			case errors.Is(err, ErrTagNotWritable):
				writeJSONError(w, http.StatusForbidden, "operation not permitted")
			default:
				log.Printf("Write tag %s error: %s", writeTag.Name, err.Error())
				writeJSONError(w, http.StatusBadGateway, "modbus write failed")
			}
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}

	return fn
}
