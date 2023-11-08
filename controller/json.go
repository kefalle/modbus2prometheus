package controller

import (
	"encoding/json"
	"log"
)

type JsonTag struct {
	Name    string      `json:"name"`
	Address uint16      `json:"address"`
	Value   interface{} `json:"value"`
}

type JsonResponse struct {
	ReqCount uint64    `json:"req_count"`
	ErrCount uint64    `json:"err_count"`
	Tags     []JsonTag `json:"tags"`
}

func (c *Controller) Json() (data []byte, err error) {
	c.RLock()
	defer c.RUnlock()

	var response = JsonResponse{
		ReqCount: c.reqCounter.Get(),
		ErrCount: c.errCounter.Get(),
	}

	for _, tag := range c.tags {
		t := JsonTag{
			Name:    tag.Name,
			Address: tag.Address,
			Value:   tag.LastValue,
		}
		response.Tags = append(response.Tags, t)
	}

	data, err = json.Marshal(response)
	if err != nil {
		log.Println("Can not make json out put")
	}
	return
}
