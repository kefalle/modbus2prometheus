package controller

import "github.com/simonvetter/modbus"

type fakeRegisterClient struct {
	openFn          func() error
	closeFn         func() error
	setUnitIDFn     func(uint8) error
	readRegisterFn  func(uint16, modbus.RegType) (uint16, error)
	readFloat32Fn   func(uint16, modbus.RegType) (float32, error)
	writeRegisterFn func(uint16, uint16) error
	writeFloat32Fn  func(uint16, float32) error

	openCalls          int
	closeCalls         int
	setUnitIDCalls     int
	readRegisterCalls  int
	readFloat32Calls   int
	writeRegisterCalls int
	writeFloat32Calls  int
}

func (f *fakeRegisterClient) Open() error {
	f.openCalls++
	if f.openFn != nil {
		return f.openFn()
	}
	return nil
}

func (f *fakeRegisterClient) Close() error {
	f.closeCalls++
	if f.closeFn != nil {
		return f.closeFn()
	}
	return nil
}

func (f *fakeRegisterClient) SetUnitId(id uint8) error {
	f.setUnitIDCalls++
	if f.setUnitIDFn != nil {
		return f.setUnitIDFn(id)
	}
	return nil
}

func (f *fakeRegisterClient) ReadRegister(address uint16, registerType modbus.RegType) (uint16, error) {
	f.readRegisterCalls++
	if f.readRegisterFn != nil {
		return f.readRegisterFn(address, registerType)
	}
	return 0, nil
}

func (f *fakeRegisterClient) ReadFloat32(address uint16, registerType modbus.RegType) (float32, error) {
	f.readFloat32Calls++
	if f.readFloat32Fn != nil {
		return f.readFloat32Fn(address, registerType)
	}
	return 0, nil
}

func (f *fakeRegisterClient) WriteRegister(address uint16, value uint16) error {
	f.writeRegisterCalls++
	if f.writeRegisterFn != nil {
		return f.writeRegisterFn(address, value)
	}
	return nil
}

func (f *fakeRegisterClient) WriteFloat32(address uint16, value float32) error {
	f.writeFloat32Calls++
	if f.writeFloat32Fn != nil {
		return f.writeFloat32Fn(address, value)
	}
	return nil
}

var _ registerClient = (*modbus.ModbusClient)(nil)
var _ registerClient = (*fakeRegisterClient)(nil)
