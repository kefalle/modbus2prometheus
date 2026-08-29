package controller

import "github.com/simonvetter/modbus"

type registerClient interface {
	Open() error
	Close() error
	SetUnitId(uint8) error
	ReadRegister(uint16, modbus.RegType) (uint16, error)
	ReadFloat32(uint16, modbus.RegType) (float32, error)
	WriteRegister(uint16, uint16) error
	WriteFloat32(uint16, float32) error
}

func newRegisterClient(conf Configuration) (registerClient, error) {
	client, err := modbus.NewClient(&modbus.ClientConfiguration{
		URL:     conf.Url,
		Speed:   conf.Speed,
		Timeout: conf.Timeout,
	})
	if err != nil {
		return nil, err
	}

	if err := client.SetUnitId(conf.DeviceId); err != nil {
		return nil, err
	}

	if err := client.Open(); err != nil {
		_ = client.Close()
		return nil, err
	}

	return client, nil
}
