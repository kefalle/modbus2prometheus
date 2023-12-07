package controller

import (
	"encoding/json"
	"log"
	"net/http"
)

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

func (c *Controller) WriteTagsHandler() http.HandlerFunc {
	fn := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			var writeTag WriteTag

			// Парсим тело
			err := json.NewDecoder(r.Body).Decode(&writeTag)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("Bad Request"))
				log.Printf("There was an error decoding the request body into the struct")
				return
			}

			// Пробуем найти тег
			log.Printf("Request to write %s tag with value %f", writeTag.Name, writeTag.Value)
			tag := c.FindTag(writeTag.Name)
			if tag == nil {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("Bad Request: tag not found"))
				log.Printf("Request has unknown tag name %s", writeTag.Name)
				return
			}

			// Пробуем записать
			if !Writable(tag) {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("Bad Request: operation not permitted"))
				log.Printf("Request tag name %s has not permission, see config", writeTag.Name)
				return
			}

			err = c.WriteTag(tag, writeTag.Value)
			if err != nil {
				log.Printf("Write tag %s error: %s", tag.Name, err.Error())
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("Bad Request: write modbus error"))
				return
			}

			w.WriteHeader(http.StatusOK)
		}
	}

	return fn
}
