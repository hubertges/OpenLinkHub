package razer

// Package: razer
// Author: OpenLinkHub
// License: GPL-3.0 or later

import (
	"OpenLinkHub/src/logger"
	"errors"
	"fmt"
	"sync"

	"github.com/godbus/dbus/v5"
)

const (
	dbusServiceName   = "org.razer"
	dbusManagerPath   = "/org/razer"
	dbusDevicePrefix  = "/org/razer/device/"
	ifaceDevices      = "razer.devices"
	ifaceMisc         = "razer.device.misc"
	ifaceChroma       = "razer.device.lighting.chroma"
	ifaceBrightness   = "razer.device.lighting.brightness"
	ifaceDpi          = "razer.device.dpi"
	ifacePower        = "razer.device.power"
	ifaceScroll       = "razer.device.scroll"
	ifaceGameMode     = "razer.device.led.gamemode"
	ifaceMacroMode    = "razer.device.led.macromode"
	ifaceLogo         = "razer.device.lighting.logo"
	ifaceScrollLight  = "razer.device.lighting.scroll"
)

// Client handles DBus session connection to openrazer-daemon
type Client struct {
	mu   sync.RWMutex
	conn *dbus.Conn
}

var (
	clientInstance *Client
	clientMu       sync.Mutex
)

// GetClient returns a singleton DBus client
func GetClient() (*Client, error) {
	clientMu.Lock()
	defer clientMu.Unlock()

	if clientInstance != nil && clientInstance.conn != nil {
		return clientInstance, nil
	}

	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to session bus: %w", err)
	}

	clientInstance = &Client{conn: conn}
	return clientInstance, nil
}

// Close closes the DBus connection
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
}

// IsAvailable checks whether the openrazer-daemon is active on DBus
func (c *Client) IsAvailable() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.conn == nil {
		return false
	}

	var hasOwner bool
	err := c.conn.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, dbusServiceName).Store(&hasOwner)
	return err == nil && hasOwner
}

// GetDevices returns all device serial identifiers from OpenRazer
func (c *Client) GetDevices() ([]string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.conn == nil {
		return nil, errors.New("dbus connection not available")
	}

	obj := c.conn.Object(dbusServiceName, dbusManagerPath)
	var serials []string
	err := obj.Call(ifaceDevices+".getDevices", 0).Store(&serials)
	if err != nil {
		return nil, err
	}
	return serials, nil
}

// SyncEffects enables or disables daemon-level effect synchronization across Razer devices
func (c *Client) SyncEffects(enable bool) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.conn == nil {
		return errors.New("dbus connection not available")
	}

	obj := c.conn.Object(dbusServiceName, dbusManagerPath)
	return obj.Call(ifaceDevices+".syncEffects", 0, enable).Err
}

// GetSyncEffects queries if effect synchronization is enabled on daemon level
func (c *Client) GetSyncEffects() (bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.conn == nil {
		return false, errors.New("dbus connection not available")
	}

	obj := c.conn.Object(dbusServiceName, dbusManagerPath)
	var enabled bool
	err := obj.Call(ifaceDevices+".getSyncEffects", 0).Store(&enabled)
	return enabled, err
}

// DeviceObject wraps a single Razer device via DBus
type DeviceObject struct {
	Serial        string
	devObj        dbus.BusObject
	Name          string
	Type          string
	Firmware      string
	DriverVersion string
	Vid           int
	Pid           int
	MatrixRows    int
	MatrixCols    int
	HasMatrix     bool
}

// GetDeviceObject queries device metadata and constructs a DeviceObject
func (c *Client) GetDeviceObject(serial string) (*DeviceObject, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.conn == nil {
		return nil, errors.New("dbus connection not available")
	}

	devPath := dbus.ObjectPath(dbusDevicePrefix + serial)
	devObj := c.conn.Object(dbusServiceName, devPath)

	d := &DeviceObject{
		Serial: serial,
		devObj: devObj,
	}

	_ = devObj.Call(ifaceMisc+".getDeviceName", 0).Store(&d.Name)
	_ = devObj.Call(ifaceMisc+".getDeviceType", 0).Store(&d.Type)
	_ = devObj.Call(ifaceMisc+".getFirmware", 0).Store(&d.Firmware)
	_ = devObj.Call(ifaceMisc+".getDriverVersion", 0).Store(&d.DriverVersion)

	var vidPid []int32
	if err := devObj.Call(ifaceMisc+".getVidPid", 0).Store(&vidPid); err == nil && len(vidPid) >= 2 {
		d.Vid = int(vidPid[0])
		d.Pid = int(vidPid[1])
	}

	var dims []int32
	if err := devObj.Call(ifaceMisc+".getMatrixDimensions", 0).Store(&dims); err == nil && len(dims) >= 2 {
		d.MatrixRows = int(dims[0])
		d.MatrixCols = int(dims[1])
		d.HasMatrix = d.MatrixRows > 0 && d.MatrixCols > 0
	}

	return d, nil
}

// SetStatic sets a single static RGB color
func (d *DeviceObject) SetStatic(r, g, b byte) error {
	err := d.devObj.Call(ifaceChroma+".setStatic", 0, r, g, b).Err
	if err != nil {
		// Try logo / scroll lighting fallback
		_ = d.devObj.Call(ifaceLogo+".setLogoStatic", 0, r, g, b)
		_ = d.devObj.Call(ifaceScrollLight+".setScrollStatic", 0, r, g, b)
	}
	return err
}

// SetSpectrum activates the spectrum cycling effect
func (d *DeviceObject) SetSpectrum() error {
	err := d.devObj.Call(ifaceChroma+".setSpectrum", 0).Err
	if err != nil {
		_ = d.devObj.Call(ifaceLogo+".setLogoSpectrum", 0)
		_ = d.devObj.Call(ifaceScrollLight+".setScrollSpectrum", 0)
	}
	return err
}

// SetWave activates the wave effect (dir: 1 = left, 2 = right)
func (d *DeviceObject) SetWave(dir int32) error {
	if dir <= 0 {
		dir = 1
	}
	err := d.devObj.Call(ifaceChroma+".setWave", 0, dir).Err
	if err != nil {
		_ = d.devObj.Call(ifaceLogo+".setLogoWave", 0, dir)
		_ = d.devObj.Call(ifaceScrollLight+".setScrollWave", 0, dir)
	}
	return err
}

// SetBreathSingle sets single color breathing effect
func (d *DeviceObject) SetBreathSingle(r, g, b byte) error {
	err := d.devObj.Call(ifaceChroma+".setBreathSingle", 0, r, g, b).Err
	if err != nil {
		_ = d.devObj.Call(ifaceLogo+".setLogoBreathSingle", 0, r, g, b)
		_ = d.devObj.Call(ifaceScrollLight+".setScrollBreathSingle", 0, r, g, b)
	}
	return err
}

// SetBreathDual sets dual color breathing effect
func (d *DeviceObject) SetBreathDual(r1, g1, b1, r2, g2, b2 byte) error {
	return d.devObj.Call(ifaceChroma+".setBreathDual", 0, r1, g1, b1, r2, g2, b2).Err
}

// SetBreathRandom sets random color breathing effect
func (d *DeviceObject) SetBreathRandom() error {
	err := d.devObj.Call(ifaceChroma+".setBreathRandom", 0).Err
	if err != nil {
		_ = d.devObj.Call(ifaceLogo+".setLogoBreathRandom", 0)
		_ = d.devObj.Call(ifaceScrollLight+".setScrollBreathRandom", 0)
	}
	return err
}

// SetReactive sets reactive typing effect
func (d *DeviceObject) SetReactive(r, g, b byte, speed byte) error {
	if speed == 0 {
		speed = 2
	}
	return d.devObj.Call(ifaceChroma+".setReactive", 0, r, g, b, speed).Err
}

// SetStarlightRandom sets random starlight effect
func (d *DeviceObject) SetStarlightRandom(speed byte) error {
	if speed == 0 {
		speed = 2
	}
	return d.devObj.Call(ifaceChroma+".setStarlightRandom", 0, speed).Err
}

// SetNone turns off device lighting
func (d *DeviceObject) SetNone() error {
	err := d.devObj.Call(ifaceChroma+".setNone", 0).Err
	if err != nil {
		_ = d.devObj.Call(ifaceLogo+".setLogoNone", 0)
		_ = d.devObj.Call(ifaceScrollLight+".setScrollNone", 0)
	}
	return err
}

// SetBrightness sets lighting brightness (0-100)
func (d *DeviceObject) SetBrightness(brightness float64) error {
	if brightness < 0 {
		brightness = 0
	} else if brightness > 100 {
		brightness = 100
	}
	err := d.devObj.Call(ifaceBrightness+".setBrightness", 0, brightness).Err
	if err != nil {
		_ = d.devObj.Call(ifaceLogo+".setLogoBrightness", 0, brightness)
		_ = d.devObj.Call(ifaceScrollLight+".setScrollBrightness", 0, brightness)
	}
	return err
}

// GetBrightness returns current brightness (0-100)
func (d *DeviceObject) GetBrightness() (float64, error) {
	var val float64
	err := d.devObj.Call(ifaceBrightness+".getBrightness", 0).Store(&val)
	if err != nil {
		err = d.devObj.Call(ifaceLogo+".getLogoBrightness", 0).Store(&val)
	}
	return val, err
}

// SetKeyRow sends a row of RGB data to the driver
func (d *DeviceObject) SetKeyRow(payload []byte) error {
	return d.devObj.Call(ifaceChroma+".setKeyRow", 0, payload).Err
}

// SetCustom activates the custom matrix effect
func (d *DeviceObject) SetCustom() error {
	return d.devObj.Call(ifaceChroma+".setCustom", 0).Err
}

// SetCustomMatrix maps a raw RGB buffer onto the device matrix rows and activates custom mode
func (d *DeviceObject) SetCustomMatrix(rgbData []byte) error {
	if !d.HasMatrix || d.MatrixRows <= 0 || d.MatrixCols <= 0 {
		return errors.New("device does not support matrix lighting")
	}

	cols := d.MatrixCols
	rows := d.MatrixRows
	totalLeds := rows * cols
	expectedLen := totalLeds * 3

	// Pad if needed
	buf := rgbData
	if len(buf) < expectedLen {
		padded := make([]byte, expectedLen)
		copy(padded, buf)
		buf = padded
	}

	for r := 0; r < rows; r++ {
		rowPayload := make([]byte, 3+cols*3)
		rowPayload[0] = byte(r)
		rowPayload[1] = 0
		rowPayload[2] = byte(cols - 1)

		startOffset := r * cols * 3
		copy(rowPayload[3:], buf[startOffset:startOffset+cols*3])

		if err := d.SetKeyRow(rowPayload); err != nil {
			logger.Log(logger.Fields{"serial": d.Serial, "row": r, "error": err}).Warn("Razer SetKeyRow failed")
			return err
		}
	}

	return d.SetCustom()
}

// GetDPI returns current X and Y DPI
func (d *DeviceObject) GetDPI() (int, int, error) {
	var dpi []int32
	err := d.devObj.Call(ifaceDpi+".getDPI", 0).Store(&dpi)
	if err != nil || len(dpi) == 0 {
		return 0, 0, err
	}
	if len(dpi) == 1 {
		return int(dpi[0]), int(dpi[0]), nil
	}
	return int(dpi[0]), int(dpi[1]), nil
}

// SetDPI sets DPI for X and Y axes
func (d *DeviceObject) SetDPI(x, y int) error {
	return d.devObj.Call(ifaceDpi+".setDPI", 0, int32(x), int32(y)).Err
}

// MaxDPI returns the maximum supported DPI
func (d *DeviceObject) MaxDPI() (int, error) {
	var maxDpi int32
	err := d.devObj.Call(ifaceDpi+".maxDPI", 0).Store(&maxDpi)
	return int(maxDpi), err
}

// GetPollRate returns current USB polling rate (e.g. 1000)
func (d *DeviceObject) GetPollRate() (int, error) {
	var rate int32
	err := d.devObj.Call(ifaceMisc+".getPollRate", 0).Store(&rate)
	return int(rate), err
}

// SetPollRate sets USB polling rate
func (d *DeviceObject) SetPollRate(rate int) error {
	return d.devObj.Call(ifaceMisc+".setPollRate", 0, int32(rate)).Err
}

// GetBattery returns battery percentage (0-100)
func (d *DeviceObject) GetBattery() (float64, error) {
	var batt float64
	err := d.devObj.Call(ifacePower+".getBattery", 0).Store(&batt)
	return batt, err
}

// IsCharging checks if device is charging
func (d *DeviceObject) IsCharging() (bool, error) {
	var charging bool
	err := d.devObj.Call(ifacePower+".isCharging", 0).Store(&charging)
	return charging, err
}

// GetIdleTime returns idle sleep timeout in seconds
func (d *DeviceObject) GetIdleTime() (int, error) {
	var idle uint16
	err := d.devObj.Call(ifacePower+".getIdleTime", 0).Store(&idle)
	return int(idle), err
}

// SetIdleTime sets idle sleep timeout in seconds
func (d *DeviceObject) SetIdleTime(sec int) error {
	return d.devObj.Call(ifacePower+".setIdleTime", 0, uint16(sec)).Err
}

// GetLowBatteryThreshold returns battery warning percentage threshold
func (d *DeviceObject) GetLowBatteryThreshold() (uint8, error) {
	var threshold uint8
	err := d.devObj.Call(ifacePower+".getLowBatteryThreshold", 0).Store(&threshold)
	return threshold, err
}

// SetLowBatteryThreshold sets battery warning percentage threshold
func (d *DeviceObject) SetLowBatteryThreshold(val uint8) error {
	return d.devObj.Call(ifacePower+".setLowBatteryThreshold", 0, val).Err
}

// GetScrollMode returns scroll wheel mode (0 = tactile, 1 = free spin)
func (d *DeviceObject) GetScrollMode() (int, error) {
	var mode int32
	err := d.devObj.Call(ifaceScroll+".getScrollMode", 0).Store(&mode)
	return int(mode), err
}

// SetScrollMode sets scroll wheel mode
func (d *DeviceObject) SetScrollMode(mode int) error {
	return d.devObj.Call(ifaceScroll+".setScrollMode", 0, int32(mode)).Err
}

// GetScrollAcceleration queries scroll acceleration status
func (d *DeviceObject) GetScrollAcceleration() (bool, error) {
	var accel bool
	err := d.devObj.Call(ifaceScroll+".getScrollAcceleration", 0).Store(&accel)
	return accel, err
}

// SetScrollAcceleration toggles scroll acceleration
func (d *DeviceObject) SetScrollAcceleration(enabled bool) error {
	return d.devObj.Call(ifaceScroll+".setScrollAcceleration", 0, enabled).Err
}

// GetScrollSmartReel queries Smart Reel status
func (d *DeviceObject) GetScrollSmartReel() (bool, error) {
	var sr bool
	err := d.devObj.Call(ifaceScroll+".getScrollSmartReel", 0).Store(&sr)
	return sr, err
}

// SetScrollSmartReel toggles Smart Reel
func (d *DeviceObject) SetScrollSmartReel(enabled bool) error {
	return d.devObj.Call(ifaceScroll+".setScrollSmartReel", 0, enabled).Err
}

// GetGameMode queries keyboard Game Mode
func (d *DeviceObject) GetGameMode() (bool, error) {
	var gm bool
	err := d.devObj.Call(ifaceGameMode+".getGameMode", 0).Store(&gm)
	return gm, err
}

// SetGameMode toggles keyboard Game Mode
func (d *DeviceObject) SetGameMode(enabled bool) error {
	return d.devObj.Call(ifaceGameMode+".setGameMode", 0, enabled).Err
}

// GetMacroMode queries keyboard Macro Mode
func (d *DeviceObject) GetMacroMode() (bool, error) {
	var mm bool
	err := d.devObj.Call(ifaceMacroMode+".getMacroMode", 0).Store(&mm)
	return mm, err
}

// SetMacroMode toggles keyboard Macro Mode
func (d *DeviceObject) SetMacroMode(enabled bool) error {
	return d.devObj.Call(ifaceMacroMode+".setMacroMode", 0, enabled).Err
}
