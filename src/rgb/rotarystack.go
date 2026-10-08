package rgb

import (
	"math"
	"time"
)

// RotaryStack will run RGB function
func (r *ActiveRGB) RotaryStack(startTime *time.Time) {
	elapsed := time.Since(*startTime).Milliseconds()
	if r.RgbModeSpeed <= 0 {
		r.RgbModeSpeed = 1.0
	}

	cycleDuration := r.RgbModeSpeed * 1000
	ledCount := r.LightChannels
	if ledCount <= 0 {
		return
	}

	fullCycleDuration := cycleDuration * float64(ledCount)
	if fullCycleDuration <= 0 {
		fullCycleDuration = 1000
	}

	cycleElapsed := math.Mod(float64(elapsed), fullCycleDuration)
	totalCycles := int(cycleElapsed / cycleDuration)
	trailCount := totalCycles
	if trailCount > ledCount {
		trailCount = ledCount
	}

	progress := math.Mod(cycleElapsed/cycleDuration, 1.0)
	rotorPos := int(progress * float64(ledCount-trailCount))
	if rotorPos >= ledCount-trailCount && ledCount > trailCount {
		rotorPos = ledCount - trailCount - 1
	}

	colors := make([]struct{ R, G, B float64 }, ledCount)
	rotorRGB := interpolateColors(r.RGBStartColor, r.RGBStartColor, 0, r.RGBBrightness)
	trailRGB := interpolateColors(r.RGBEndColor, r.RGBEndColor, 0, r.RGBBrightness)

	for i := 0; i < ledCount; i++ {
		switch {
		case i >= ledCount-trailCount:
			// Already marked trail LEDs
			colors[i] = struct{ R, G, B float64 }{trailRGB.Red, trailRGB.Green, trailRGB.Blue}
		case i <= rotorPos:
			// Current rotor LEDs (behind rotor)
			colors[i] = struct{ R, G, B float64 }{rotorRGB.Red, rotorRGB.Green, rotorRGB.Blue}
		default:
			// Not yet reached
			colors[i] = struct{ R, G, B float64 }{0, 0, 0}
		}
	}

	// Update Buffer
	for j, c := range colors {
		if len(r.Buffer) > 0 {
			r.Buffer[j] = byte(c.R)
			r.Buffer[j+r.ColorOffset] = byte(c.G)
			r.Buffer[j+(r.ColorOffset*2)] = byte(c.B)
		}
	}

	// Update Raw for controller
	raw := map[int][]byte{}
	for j, c := range colors {
		raw[j] = []byte{byte(c.R), byte(c.G), byte(c.B)}
	}
	r.Raw = raw

	// Output
	if r.Inverted {
		r.Output = SetColorInverted(raw)
	} else {
		r.Output = SetColor(raw)
	}
}
