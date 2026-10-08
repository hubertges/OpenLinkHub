package razer

// Package: razer
// Author: OpenLinkHub
// License: GPL-3.0 or later

import (
	"OpenLinkHub/src/cluster"
	"OpenLinkHub/src/common"
	"OpenLinkHub/src/config"
	"OpenLinkHub/src/logger"
	"OpenLinkHub/src/rgb"
	"OpenLinkHub/src/stats"
	"encoding/json"
	"os"
	"slices"
	"sync"
	"time"
)

var (
	pwd = ""
)

// DPIProfile represents a mouse DPI stage
type DPIProfile struct {
	Name  string     `json:"name"`
	Value uint16     `json:"value"`
	Color *rgb.Color `json:"color,omitempty"`
}

// DeviceProfile stores all configurable settings for a Razer device
type DeviceProfile struct {
	Active              bool                  `json:"active"`
	Product             string                `json:"product"`
	Serial              string                `json:"serial"`
	Brightness          uint8                 `json:"brightness"`
	BrightnessSlider    *uint8                `json:"brightnessSlider"`
	OriginalBrightness  uint8                 `json:"originalBrightness"`
	RGBProfile          string                `json:"rgbProfile"`
	RGBCluster          bool                  `json:"rgbCluster"`
	DPI                 int                   `json:"dpi"`
	DPIX                int                   `json:"dpiX"`
	DPIY                int                   `json:"dpiY"`
	DPIStage            int                   `json:"dpiStage"`
	DPIStages           map[int]DPIProfile    `json:"dpiStages"`
	PollingRate         int                   `json:"pollingRate"`
	GameMode            bool                  `json:"gameMode"`
	MacroMode           bool                  `json:"macroMode"`
	ScrollMode          int                   `json:"scrollMode"`
	SmartReel           bool                  `json:"smartReel"`
	ScrollAcceleration  bool                  `json:"scrollAcceleration"`
	IdleTime            int                   `json:"idleTime"`
	LowBatteryThreshold uint8                 `json:"lowBatteryThreshold"`
	SyncEffects         bool                  `json:"syncEffects"`
	RgbOff              bool                  `json:"rgbOff"`
}

// Device represents an integrated Razer device
type Device struct {
	Manufacturer       string                    `json:"manufacturer"`
	Product            string                    `json:"product"`
	Serial             string                    `json:"serial"`
	Firmware           string                    `json:"firmware"`
	DriverVersion      string                    `json:"driverVersion"`
	Path               string                    `json:"path"`
	DeviceType         uint32                    `json:"deviceType"`
	ProductType        uint16                    `json:"productType"`
	Connected          bool                      `json:"connected"`
	Usb                bool                      `json:"usb"`
	LEDChannels        int                       `json:"ledChannels"`
	MatrixRows         int                       `json:"matrixRows"`
	MatrixCols         int                       `json:"matrixCols"`
	HasMatrix          bool                      `json:"hasMatrix"`
	HasBattery         bool                      `json:"hasBattery"`
	BatteryLevel       int                       `json:"batteryLevel"`
	IsCharging         bool                      `json:"isCharging"`
	HasDPI             bool                      `json:"hasDPI"`
	MaxDPI             int                       `json:"maxDpi"`
	HasScrollOptions   bool                      `json:"hasScrollOptions"`
	HasGameMode        bool                      `json:"hasGameMode"`
	HasMacroMode       bool                      `json:"hasMacroMode"`
	PollingRates       map[int]string            `json:"pollingRates"`
	RGBModes           []string                  `json:"rgbModes"`
	DeviceProfile      *DeviceProfile            `json:"deviceProfile"`
	UserProfiles       map[string]*DeviceProfile `json:"userProfiles"`
	Rgb                *rgb.RGB                  `json:"rgb"`
	clientDev          *DeviceObject             `json:"-"`
	activeRgb          *rgb.ActiveRGB            `json:"-"`
	mutex              sync.RWMutex              `json:"-"`
	deviceLock         sync.Mutex                `json:"-"`
	Exit               bool                      `json:"-"`
	autoRefreshChan    chan struct{}             `json:"-"`
	timer              *time.Ticker              `json:"-"`
	lastClusterWrite   time.Time                 `json:"-"`
}

// NewRazerDevice creates a Device from a client DeviceObject
func NewRazerDevice(devObj *DeviceObject) *Device {
	pwd = config.GetConfig().ConfigPath

	d := &Device{
		Manufacturer:  "Razer",
		Product:       devObj.Name,
		Serial:        devObj.Serial,
		Firmware:      devObj.Firmware,
		DriverVersion: devObj.DriverVersion,
		Path:          devObj.Serial,
		Connected:     true,
		Usb:           true,
		MatrixRows:    devObj.MatrixRows,
		MatrixCols:    devObj.MatrixCols,
		HasMatrix:     devObj.HasMatrix,
		clientDev:     devObj,
		UserProfiles:  make(map[string]*DeviceProfile),
		PollingRates: map[int]string{
			125:  "125 Hz",
			500:  "500 Hz",
			1000: "1000 Hz",
		},
		autoRefreshChan: make(chan struct{}),
		timer:           time.NewTicker(3 * time.Second),
	}

	if d.HasMatrix {
		d.LEDChannels = d.MatrixRows * d.MatrixCols
	} else {
		d.LEDChannels = 1
	}

	if devObj.Type == "keyboard" {
		d.DeviceType = common.DeviceTypeKeyboard
		d.ProductType = common.ProductTypeRazerKeyboard
		d.RGBModes = []string{
			"static",
			"spectrum",
			"wave",
			"colorpulse",
			"rainbow",
			"reactive",
			"starlight",
			"watercolor",
			"spiralrainbow",
			"cluster",
			"off",
		}
		// Check game mode & macro mode
		if _, err := devObj.GetGameMode(); err == nil {
			d.HasGameMode = true
		}
		if _, err := devObj.GetMacroMode(); err == nil {
			d.HasMacroMode = true
		}
	} else if devObj.Type == "mouse" {
		d.DeviceType = common.DeviceTypeMouse
		d.ProductType = common.ProductTypeRazerMouse
		d.RGBModes = []string{
			"static",
			"spectrum",
			"wave",
			"colorpulse",
			"rainbow",
			"cluster",
			"off",
		}

		if maxDPI, err := devObj.MaxDPI(); err == nil && maxDPI > 0 {
			d.HasDPI = true
			d.MaxDPI = maxDPI
		} else {
			d.MaxDPI = 35000
			d.HasDPI = true
		}

		if batt, err := devObj.GetBattery(); err == nil && batt >= 0 {
			d.HasBattery = true
			d.BatteryLevel = int(batt)
			d.IsCharging, _ = devObj.IsCharging()
		}

		if _, err := devObj.GetScrollMode(); err == nil {
			d.HasScrollOptions = true
		}
	} else {
		d.DeviceType = common.DeviceTypeAccessory
		d.ProductType = common.ProductTypeRazerGeneric
		d.RGBModes = []string{"static", "spectrum", "wave", "colorpulse", "cluster", "off"}
	}

	d.loadRgb()
	d.loadDeviceProfile()

	// Initial hardware state read
	if curBright, err := devObj.GetBrightness(); err == nil {
		bVal := uint8(curBright)
		d.DeviceProfile.Brightness = bVal
		d.DeviceProfile.BrightnessSlider = &bVal
	}
	if d.HasDPI {
		if x, y, err := devObj.GetDPI(); err == nil && x > 0 {
			d.DeviceProfile.DPI = x
			d.DeviceProfile.DPIX = x
			d.DeviceProfile.DPIY = y
		}
	}
	if pr, err := devObj.GetPollRate(); err == nil && pr > 0 {
		d.DeviceProfile.PollingRate = pr
	}
	if d.HasGameMode {
		d.DeviceProfile.GameMode, _ = devObj.GetGameMode()
	}
	if d.HasMacroMode {
		d.DeviceProfile.MacroMode, _ = devObj.GetMacroMode()
	}
	if d.HasScrollOptions {
		d.DeviceProfile.ScrollMode, _ = devObj.GetScrollMode()
		d.DeviceProfile.SmartReel, _ = devObj.GetScrollSmartReel()
		d.DeviceProfile.ScrollAcceleration, _ = devObj.GetScrollAcceleration()
	}
	if d.HasBattery {
		if idle, err := devObj.GetIdleTime(); err == nil {
			d.DeviceProfile.IdleTime = idle
		}
		if lowBatt, err := devObj.GetLowBatteryThreshold(); err == nil {
			d.DeviceProfile.LowBatteryThreshold = lowBatt
		}
	}

	d.saveDeviceProfile()

	// Setup cluster controller if enabled
	if d.DeviceProfile.RGBCluster {
		d.setupClusterController()
	}

	// Apply lighting effect
	d.setDeviceColor()

	// Start status refresh loop
	go d.statusMonitorLoop()

	return d
}

// toCommonDevice converts Device to OpenLinkHub common.Device
func (d *Device) toCommonDevice() *common.Device {
	icon := "icon-device.svg"
	if d.DeviceType == common.DeviceTypeKeyboard {
		icon = "icon-keyboard.svg"
	} else if d.DeviceType == common.DeviceTypeMouse {
		icon = "icon-mouse.svg"
	}

	return &common.Device{
		ProductType: d.ProductType,
		Product:     d.Product,
		Serial:      d.Serial,
		Firmware:    d.Firmware,
		Image:       icon,
		Instance:    d,
		Hidden:      false,
		DeviceType:  d.DeviceType,
	}
}

// GetDeviceTemplate returns the template file name for the device
func (d *Device) GetDeviceTemplate() string {
	if d.DeviceType == common.DeviceTypeKeyboard {
		return "razer_keyboard.html"
	}
	return "razer_mouse.html"
}

// GetDevice returns device instance
func (d *Device) GetDevice() interface{} {
	return d
}

// GetDeviceProfile returns current device profile
func (d *Device) GetDeviceProfile() interface{} {
	return d.DeviceProfile
}

// GetRgbProfiles returns RGB profiles
func (d *Device) GetRgbProfiles() interface{} {
	return d.Rgb
}

// GetRgbProfile returns specific RGB profile
func (d *Device) GetRgbProfile(profile string) *rgb.Profile {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	if d.Rgb == nil {
		return nil
	}
	if p, ok := d.Rgb.Profiles[profile]; ok {
		return &p
	}
	return nil
}

// ChangeDeviceBrightness changes device brightness mode
func (d *Device) ChangeDeviceBrightness(mode uint8) uint8 {
	d.DeviceProfile.Brightness = mode
	d.saveDeviceProfile()
	if d.clientDev != nil {
		_ = d.clientDev.SetBrightness(float64(mode))
	}
	return 1
}

// ChangeDeviceBrightnessValue sets brightness by slider value (0-100)
func (d *Device) ChangeDeviceBrightnessValue(value uint8) uint8 {
	if value > 100 {
		value = 100
	}
	d.DeviceProfile.Brightness = value
	d.DeviceProfile.BrightnessSlider = &value
	d.saveDeviceProfile()

	if d.clientDev != nil {
		_ = d.clientDev.SetBrightness(float64(value))
	}
	return 1
}

func (d *Device) stopActiveRGB() {
	if d.activeRgb != nil {
		select {
		case d.activeRgb.Exit <- true:
		default:
		}
		d.activeRgb = nil
	}
}

// UpdateRgbProfile updates active RGB profile and applies effect
func (d *Device) UpdateRgbProfile(_ int, profile string) uint8 {
	d.deviceLock.Lock()
	defer d.deviceLock.Unlock()

	if profile == "cluster" {
		return d.ProcessSetRgbCluster(true)
	}

	if d.DeviceProfile.RGBCluster {
		cluster.Get().RemoveDeviceControllerBySerial(d.Serial)
		d.DeviceProfile.RGBCluster = false
	}

	d.DeviceProfile.RGBProfile = profile
	d.saveDeviceProfile()

	d.stopActiveRGB()

	d.setDeviceColor()
	return 1
}

// UpdateRgbProfileData updates profile settings
func (d *Device) UpdateRgbProfileData(profileName string, profile rgb.Profile) uint8 {
	d.mutex.Lock()
	if d.Rgb == nil {
		d.mutex.Unlock()
		return 0
	}
	d.Rgb.Profiles[profileName] = profile
	d.mutex.Unlock()

	d.saveRgbProfile()

	if d.DeviceProfile.RGBProfile == profileName && !d.DeviceProfile.RGBCluster {
		d.stopActiveRGB()
		d.setDeviceColor()
	}
	return 1
}

// ProcessSetRgbCluster toggles synchronization with OpenLinkHub RGB Cluster
func (d *Device) ProcessSetRgbCluster(enabled bool) uint8 {
	d.deviceLock.Lock()
	defer d.deviceLock.Unlock()

	d.DeviceProfile.RGBCluster = enabled
	if enabled {
		d.DeviceProfile.RGBProfile = "cluster"
	}
	d.saveDeviceProfile()

	d.stopActiveRGB()

	if enabled {
		d.setupClusterController()
	} else {
		cluster.Get().RemoveDeviceControllerBySerial(d.Serial)
		d.setDeviceColor()
	}
	return 1
}

// setupClusterController registers this device into the OpenLinkHub RGB Cluster
func (d *Device) setupClusterController() {
	if d.DeviceProfile == nil || !d.DeviceProfile.RGBCluster {
		return
	}

	ledCount := d.LEDChannels
	if ledCount <= 0 {
		ledCount = 1
	}

	clusterController := &common.ClusterController{
		Product:      d.Product,
		Serial:       d.Serial,
		LedChannels:  uint32(ledCount),
		WriteColorEx: d.writeColorCluster,
	}

	cluster.Get().AddDeviceController(clusterController)
}

// writeColorCluster writes color buffer from OpenLinkHub RGB Cluster engine to Razer hardware
func (d *Device) writeColorCluster(data []byte, _ int) {
	if d.Exit || d.DeviceProfile == nil || !d.DeviceProfile.RGBCluster {
		return
	}

	// Throttle to max ~40 FPS to avoid DBus congestion
	now := time.Now()
	if now.Sub(d.lastClusterWrite) < 25*time.Millisecond {
		return
	}
	d.lastClusterWrite = now

	if d.HasMatrix && d.clientDev != nil {
		_ = d.clientDev.SetCustomMatrix(data)
	} else if len(data) >= 3 && d.clientDev != nil {
		_ = d.clientDev.SetStatic(data[0], data[1], data[2])
	}
}

// setDeviceColor activates the chosen lighting effect
func (d *Device) setDeviceColor() {
	if d.DeviceProfile == nil || d.clientDev == nil {
		return
	}

	if d.DeviceProfile.RGBCluster {
		return // Cluster engine takes over
	}

	mode := d.DeviceProfile.RGBProfile
	pf := d.GetRgbProfile(mode)

	switch mode {
	case "static":
		if pf != nil {
			_ = d.clientDev.SetStatic(byte(pf.StartColor.Red), byte(pf.StartColor.Green), byte(pf.StartColor.Blue))
		} else {
			_ = d.clientDev.SetStatic(0, 255, 0)
		}
	case "spectrum":
		_ = d.clientDev.SetSpectrum()
	case "wave":
		_ = d.clientDev.SetWave(1)
	case "colorpulse":
		if pf != nil {
			_ = d.clientDev.SetBreathSingle(byte(pf.StartColor.Red), byte(pf.StartColor.Green), byte(pf.StartColor.Blue))
		} else {
			_ = d.clientDev.SetBreathRandom()
		}
	case "reactive":
		if pf != nil {
			_ = d.clientDev.SetReactive(byte(pf.StartColor.Red), byte(pf.StartColor.Green), byte(pf.StartColor.Blue), 2)
		} else {
			_ = d.clientDev.SetReactive(0, 255, 0, 2)
		}
	case "starlight":
		_ = d.clientDev.SetStarlightRandom(2)
	case "off":
		_ = d.clientDev.SetNone()
	default:
		// Run software animation loop for advanced OpenLinkHub effects (rainbow, watercolor, etc.)
		if d.HasMatrix {
			d.runSoftwareEffectLoop(mode)
		} else {
			_ = d.clientDev.SetSpectrum()
		}
	}
}

// runSoftwareEffectLoop runs OpenLinkHub effect engine for advanced software effects
func (d *Device) runSoftwareEffectLoop(mode string) {
	activeRgb := rgb.Exit()
	d.activeRgb = activeRgb
	d.activeRgb.RGBStartColor = rgb.GenerateRandomColor(1)
	d.activeRgb.RGBEndColor = rgb.GenerateRandomColor(1)
	exitChan := activeRgb.Exit

	go func() {
		startTime := time.Now()
		speed := 1.0
		channels := d.LEDChannels

		pf := d.GetRgbProfile(mode)
		if pf != nil && pf.Speed > 0 {
			speed = float64(pf.Speed)
		}

		r := rgb.New(channels, speed, nil, nil, 1.0, 100, time.Duration(speed)*time.Second, false)
		if pf != nil {
			r.RGBStartColor = &pf.StartColor
			r.RGBEndColor = &pf.EndColor
		} else {
			r.RGBStartColor = activeRgb.RGBStartColor
			r.RGBEndColor = activeRgb.RGBEndColor
		}

		ticker := time.NewTicker(30 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-exitChan:
				return
			case <-ticker.C:
				if d.Exit || d.DeviceProfile.RGBCluster {
					return
				}

				switch mode {
				case "rainbow":
					r.Rainbow(startTime)
				case "spiralrainbow":
					r.SpiralRainbow(startTime)
				case "watercolor":
					r.Watercolor(startTime)
				case "visor":
					r.Visor(&startTime)
				case "marquee":
					r.Marquee(&startTime)
				default:
					r.Rainbow(startTime)
				}

				if len(r.Output) > 0 && d.clientDev != nil {
					_ = d.clientDev.SetCustomMatrix(r.Output)
				}
			}
		}
	}()
}

// SaveMouseDPI updates mouse DPI settings
func (d *Device) SaveMouseDPI(stages ...interface{}) uint8 {
	if len(stages) == 0 {
		return 0
	}

	// Case 1: single int value (direct DPI)
	if val, ok := stages[0].(int); ok {
		d.DeviceProfile.DPI = val
		d.DeviceProfile.DPIX = val
		d.DeviceProfile.DPIY = val
		d.saveDeviceProfile()
		if d.clientDev != nil {
			_ = d.clientDev.SetDPI(val, val)
		}
		return 1
	}

	// Case 2: stages slice
	d.saveDeviceProfile()
	return 1
}

// SetDPI sets DPI values directly
func (d *Device) SetDPI(x, y int) uint8 {
	if x <= 0 {
		return 0
	}
	if y <= 0 {
		y = x
	}
	d.DeviceProfile.DPI = x
	d.DeviceProfile.DPIX = x
	d.DeviceProfile.DPIY = y
	d.saveDeviceProfile()

	if d.clientDev != nil {
		if err := d.clientDev.SetDPI(x, y); err != nil {
			logger.Log(logger.Fields{"serial": d.Serial, "error": err}).Error("Failed to set Razer DPI")
			return 0
		}
	}
	return 1
}

// ChangePollingRate updates USB polling rate
func (d *Device) ChangePollingRate(rate int) uint8 {
	d.DeviceProfile.PollingRate = rate
	d.saveDeviceProfile()

	if d.clientDev != nil {
		_ = d.clientDev.SetPollRate(rate)
	}
	return 1
}

// SetGameMode toggles Game Mode
func (d *Device) SetGameMode(enabled bool) uint8 {
	d.DeviceProfile.GameMode = enabled
	d.saveDeviceProfile()

	if d.clientDev != nil {
		_ = d.clientDev.SetGameMode(enabled)
	}
	return 1
}

// SetMacroMode toggles Macro Mode
func (d *Device) SetMacroMode(enabled bool) uint8 {
	d.DeviceProfile.MacroMode = enabled
	d.saveDeviceProfile()

	if d.clientDev != nil {
		_ = d.clientDev.SetMacroMode(enabled)
	}
	return 1
}

// SetScrollMode updates scroll mode (0 = tactile, 1 = free spin)
func (d *Device) SetScrollMode(mode int) uint8 {
	d.DeviceProfile.ScrollMode = mode
	d.saveDeviceProfile()

	if d.clientDev != nil {
		_ = d.clientDev.SetScrollMode(mode)
	}
	return 1
}

// SetSmartReel toggles Smart Reel
func (d *Device) SetSmartReel(enabled bool) uint8 {
	d.DeviceProfile.SmartReel = enabled
	d.saveDeviceProfile()

	if d.clientDev != nil {
		_ = d.clientDev.SetScrollSmartReel(enabled)
	}
	return 1
}

// SetScrollAcceleration toggles scroll acceleration
func (d *Device) SetScrollAcceleration(enabled bool) uint8 {
	d.DeviceProfile.ScrollAcceleration = enabled
	d.saveDeviceProfile()

	if d.clientDev != nil {
		_ = d.clientDev.SetScrollAcceleration(enabled)
	}
	return 1
}

// SetIdleTime sets idle sleep timeout
func (d *Device) SetIdleTime(sec int) uint8 {
	d.DeviceProfile.IdleTime = sec
	d.saveDeviceProfile()

	if d.clientDev != nil {
		_ = d.clientDev.SetIdleTime(sec)
	}
	return 1
}

// SetLowBatteryThreshold sets low battery threshold percentage
func (d *Device) SetLowBatteryThreshold(val uint8) uint8 {
	d.DeviceProfile.LowBatteryThreshold = val
	d.saveDeviceProfile()

	if d.clientDev != nil {
		_ = d.clientDev.SetLowBatteryThreshold(val)
	}
	return 1
}

// SetSyncEffects toggles daemon-wide effect sync
func (d *Device) SetSyncEffects(enabled bool) uint8 {
	d.DeviceProfile.SyncEffects = enabled
	d.saveDeviceProfile()

	client, err := GetClient()
	if err == nil {
		_ = client.SyncEffects(enabled)
	}
	return 1
}

// statusMonitorLoop periodically polls battery and connection status
func (d *Device) statusMonitorLoop() {
	for {
		select {
		case <-d.autoRefreshChan:
			return
		case <-d.timer.C:
			if d.Exit {
				return
			}
			if d.HasBattery && d.clientDev != nil {
				if batt, err := d.clientDev.GetBattery(); err == nil && batt >= 0 {
					d.BatteryLevel = int(batt)
					d.IsCharging, _ = d.clientDev.IsCharging()
					stats.UpdateBatteryStats(d.Serial, d.Product, uint16(d.BatteryLevel), uint8(d.DeviceType))
				}
			}
		}
	}
}

// Stop stops the device operations
func (d *Device) Stop() {
	d.Exit = true
	d.stopActiveRGB()
	if d.timer != nil {
		d.timer.Stop()
	}
	if d.autoRefreshChan != nil {
		close(d.autoRefreshChan)
		d.autoRefreshChan = nil
	}
	cluster.Get().RemoveDeviceControllerBySerial(d.Serial)
}

// StopDirty stops without graceful teardown
func (d *Device) StopDirty() {
	d.Stop()
}

// loadDeviceProfile loads device settings from JSON
func (d *Device) loadDeviceProfile() {
	profileDir := pwd + "/database/profiles/"
	profilePath := profileDir + d.Serial + ".json"

	defaultDPI := 800
	if d.HasDPI {
		defaultDPI = 1600
	}

	bVal := uint8(100)
	defaultProfile := &DeviceProfile{
		Active:              true,
		Product:             d.Product,
		Serial:              d.Serial,
		Brightness:          100,
		BrightnessSlider:    &bVal,
		OriginalBrightness:  100,
		RGBProfile:          "spectrum",
		RGBCluster:          false,
		DPI:                 defaultDPI,
		DPIX:                defaultDPI,
		DPIY:                defaultDPI,
		DPIStage:            2,
		DPIStages: map[int]DPIProfile{
			0: {Name: "Stage 1", Value: 400},
			1: {Name: "Stage 2", Value: 800},
			2: {Name: "Stage 3", Value: 1600},
			3: {Name: "Stage 4", Value: 3200},
			4: {Name: "Stage 5", Value: 6400},
		},
		PollingRate:         1000,
		IdleTime:            300,
		LowBatteryThreshold: 15,
	}

	if !common.FileExists(profilePath) {
		d.DeviceProfile = defaultProfile
		d.UserProfiles["Default"] = defaultProfile
		d.saveDeviceProfile()
		return
	}

	file, err := os.Open(profilePath)
	if err != nil {
		d.DeviceProfile = defaultProfile
		return
	}
	defer file.Close()

	if err := json.NewDecoder(file).Decode(&d.DeviceProfile); err != nil {
		d.DeviceProfile = defaultProfile
		return
	}

	if d.DeviceProfile.BrightnessSlider == nil {
		d.DeviceProfile.BrightnessSlider = &d.DeviceProfile.Brightness
	}
	if len(d.DeviceProfile.DPIStages) == 0 {
		d.DeviceProfile.DPIStages = defaultProfile.DPIStages
	}
	d.UserProfiles["Default"] = d.DeviceProfile
}

// saveDeviceProfile persists device profile to disk
func (d *Device) saveDeviceProfile() {
	if d.DeviceProfile == nil {
		return
	}
	profileDir := pwd + "/database/profiles/"
	profilePath := profileDir + d.Serial + ".json"
	_ = common.SaveJsonData(profilePath, d.DeviceProfile)
}

// loadRgb loads RGB configurations
func (d *Device) loadRgb() {
	rgbDir := pwd + "/database/rgb/"
	rgbPath := rgbDir + d.Serial + ".json"

	if !common.FileExists(rgbPath) {
		profile := rgb.GetRGB()
		profile.Device = d.Product
		_ = common.SaveJsonData(rgbPath, profile)
	}

	file, err := os.Open(rgbPath)
	if err != nil {
		return
	}
	defer file.Close()

	_ = json.NewDecoder(file).Decode(&d.Rgb)
	if d.Rgb == nil {
		p := rgb.GetRGB()
		d.Rgb = &p
	}

	// Filter available modes
	profiles := make(map[string]rgb.Profile)
	for key, value := range d.Rgb.Profiles {
		if slices.Contains(d.RGBModes, key) {
			profiles[key] = value
		}
	}
	d.Rgb.Profiles = profiles
}

// saveRgbProfile saves RGB configurations
func (d *Device) saveRgbProfile() {
	if d.Rgb == nil {
		return
	}
	rgbDir := pwd + "/database/rgb/"
	rgbPath := rgbDir + d.Serial + ".json"
	_ = common.SaveJsonData(rgbPath, d.Rgb)
}
