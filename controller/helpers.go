package controller

import (
	"log"
	"os"
	"strconv"
	"strings"
)

func defaultFloat32Action(val interface{}, t *Tag) {
	if t.LastValue != val {
		v := val.(float32)
		log.Printf("req %d tag %s = %f", t.controller.reqCounter.Get(), t.Name, v)
		t.LastValue = v
	}
}

func defaultUint16Action(val interface{}, t *Tag) {
	if t.LastValue != val {
		v := val.(uint16)
		log.Printf("req %d tag %s = %d", t.controller.reqCounter.Get(), t.Name, v)
		t.LastValue = val
	}
}

func isFlag(t *Tag, f OperationType) bool {
	uf := uint8(f)
	return (t.Method & uf) == uf
}

func isUint(t *Tag) bool {
	return isFlag(t, READ_UINT)
}

func isWriteUint(t *Tag) bool {
	return isFlag(t, WRITE_UINT)
}

func isFloat(t *Tag) bool {
	return isFlag(t, READ_FLOAT)
}

func isWriteFloat(t *Tag) bool {
	return isFlag(t, WRITE_FLOAT)
}

func Writable(t *Tag) bool {
	return isWriteUint(t) || isWriteFloat(t)
}

func ValToStr(t *Tag) string {
	if t.LastValue == nil {
		return "0"
	}

	if isUint(t) {
		return strconv.Itoa(int(t.LastValue.(uint16)))
	} else if isFloat(t) {
		return strconv.FormatFloat(float64(t.LastValue.(float32)), 'f', 2, 32)
	} else {
		return "unknown"
	}
}

func ParseOperation(op string) (t uint8) {
	var res uint8 = 0

	if strings.Contains(op, "read_uint") {
		res |= READ_UINT
	}
	if strings.Contains(op, "read_float") {
		res |= READ_FLOAT
	}
	if strings.Contains(op, "write_uint") {
		res |= WRITE_UINT
	}
	if strings.Contains(op, "write_float") {
		res |= WRITE_FLOAT
	}

	if res > 0 {
		return res
	}

	log.Println("Unsupported operation " + op + " must be read_uint, read_float")
	os.Exit(1)

	return 0
}
