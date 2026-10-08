package razer

// Package: razer
// Author: OpenLinkHub
// License: GPL-3.0 or later

import (
	"OpenLinkHub/src/common"
	"OpenLinkHub/src/config"
	"OpenLinkHub/src/logger"
	"fmt"
	"sync"
	"time"
)

var (
	managerMu        sync.Mutex
	managedDevices   = make(map[string]*Device)
	stopScanChan     chan struct{}
	addDeviceFunc    func(*common.Device)
	deleteDeviceFunc func(string)
)

// Init initializes the Razer subsystem, discovering devices and registering them
func Init(addDevice func(*common.Device), deleteDevice func(string)) {
	managerMu.Lock()
	defer managerMu.Unlock()

	addDeviceFunc = addDevice
	deleteDeviceFunc = deleteDevice

	client, err := GetClient()
	if err != nil {
		logger.Log(logger.Fields{"error": err}).Warn("OpenRazer DBus client connection failed")
		return
	}

	if !client.IsAvailable() {
		logger.Log(logger.Fields{}).Info("OpenRazer daemon is not currently active on DBus")
		return
	}

	logger.Log(logger.Fields{}).Info("Initializing OpenRazer devices...")

	scanDevices(client)

	if config.GetConfig().RazerSyncEffects {
		_ = client.SyncEffects(true)
	}

	// Start background watcher for device plug/unplug
	stopScanChan = make(chan struct{})
	go backgroundWatcher()
}

// scanDevices queries OpenRazer and registers all new devices
func scanDevices(client *Client) {
	serials, err := client.GetDevices()
	if err != nil {
		logger.Log(logger.Fields{"error": err}).Warn("Failed to list Razer devices from daemon")
		return
	}

	for _, s := range serials {
		if _, exists := managedDevices[s]; exists {
			continue
		}

		devObj, err := client.GetDeviceObject(s)
		if err != nil {
			logger.Log(logger.Fields{"serial": s, "error": err}).Warn("Failed to get Razer device info")
			continue
		}

		dev := NewRazerDevice(devObj)
		managedDevices[s] = dev

		if addDeviceFunc != nil {
			addDeviceFunc(dev.toCommonDevice())
		}

		logger.Log(logger.Fields{
			"serial":  s,
			"product": dev.Product,
			"type":    devObj.Type,
			"matrix":  fmtMatrix(devObj),
		}).Info("Registered Razer device")
	}
}

func fmtMatrix(d *DeviceObject) string {
	if d.HasMatrix {
		return fmt.Sprintf("%dx%d", d.MatrixRows, d.MatrixCols)
	}
	return "none"
}

// backgroundWatcher periodically checks for device connections and disconnections
func backgroundWatcher() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stopScanChan:
			return
		case <-ticker.C:
			client, err := GetClient()
			if err != nil || !client.IsAvailable() {
				continue
			}

			serials, err := client.GetDevices()
			if err != nil {
				continue
			}

			serialMap := make(map[string]bool)
			for _, s := range serials {
				serialMap[s] = true
			}

			managerMu.Lock()
			// Check removed
			for s, dev := range managedDevices {
				if !serialMap[s] {
					dev.StopDirty()
					delete(managedDevices, s)
					if deleteDeviceFunc != nil {
						deleteDeviceFunc(s)
					}
					logger.Log(logger.Fields{"serial": s}).Info("Removed disconnected Razer device")
				}
			}

			// Check added
			scanDevices(client)
			managerMu.Unlock()
		}
	}
}

// SyncEffects sets daemon-wide effect sync
func SyncEffects(enabled bool) error {
	client, err := GetClient()
	if err != nil {
		return err
	}
	return client.SyncEffects(enabled)
}

// Stop stops the Razer manager and all device loops
func Stop() {
	managerMu.Lock()
	defer managerMu.Unlock()

	if stopScanChan != nil {
		close(stopScanChan)
		stopScanChan = nil
	}

	for s, dev := range managedDevices {
		dev.Stop()
		delete(managedDevices, s)
	}

	client, err := GetClient()
	if err == nil {
		client.Close()
	}
}
