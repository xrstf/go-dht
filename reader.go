//--------------------------------------------------------------------------------------------------
//
// Copyright (c) 2015-2019 Denis Dyakov
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of this software and
// associated documentation files (the "Software"), to deal in the Software without restriction,
// including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense,
// and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all copies or substantial
// portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING
// BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
// NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM,
// DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
//
//--------------------------------------------------------------------------------------------------

package dht

// #include "gpio.h"
import "C"

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"
	"unsafe"

	"github.com/sirupsen/logrus"
)

type Reader struct {
	sensorType SensorType
	pin        int
	logger     logrus.FieldLogger
}

func NewReader(sensorType SensorType, pin int, logger logrus.FieldLogger) *Reader {
	return &Reader{
		sensorType: sensorType,
		pin:        pin,
		logger: logger.WithFields(logrus.Fields{
			"sensor": sensorType,
			"pin":    pin,
		}),
	}
}

func (r *Reader) Read(ctx context.Context, boostPerfFlag bool, retry int) (float32, float32, int, error) {
	retried := 0

	for {
		temp, hum, err := r.read(boostPerfFlag)
		if err != nil {
			if retry > 0 {
				r.logger.Warning(err)
				retry--
				retried++

				select {
				// check for termination request
				case <-ctx.Done():
					// Interrupt loop, if pending termination.
					return 0, 0, retried, ctx.Err()
				// sleep before new attempt according to specification
				case <-time.After(r.sensorType.GetRetryTimeout()):
					continue
				}
			}

			return 0, 0, retried, err
		}

		return temp, hum, retried, nil
	}
}

func (r *Reader) read(boostPerfFlag bool) (float32, float32, error) {
	// activate sensor and read data to pulses array
	handshakeDur := r.sensorType.GetHandshakeDuration()
	pulses, err := r.dialAndGetResponse(handshakeDur, boostPerfFlag)
	if err != nil {
		return 0, 0, err
	}

	// output debug information
	r.printPulseArray(pulses)

	// decode pulses
	return r.decodePulses(pulses)
}

// Print bunch of pulses for debug purpose.
func (r *Reader) printPulseArray(pulses []Pulse) {
	// var buf bytes.Buffer
	// for i, pulse := range pulses {
	// 	buf.WriteString(fmt.Sprintf("pulse %3d: %v, %v\n", i,
	// 		pulse.Value, pulse.Duration))
	// }
	// r.logger.Debugf("Pulse count %d:\n%v", len(pulses), buf.String())
	r.logger.Debugf("Pulses received from DHTxx sensor: %v", pulses)
}

// Activate sensor and get back bunch of pulses for further decoding.
// C function call wrapper.
func (r *Reader) dialAndGetResponse(handshakeDur time.Duration, boostPerfFlag bool) ([]Pulse, error) {
	var arr *C.int32_t
	var arrLen C.int32_t
	var list []int32
	var hsDurUsec C.int32_t = C.int32_t(handshakeDur / time.Microsecond)
	var boost C.int32_t = 0
	var err2 *C.Error
	if boostPerfFlag {
		boost = 1
	}

	// return array: [pulse, duration, pulse, duration, ...]
	res := C.dial_DHTxx_and_read(C.int32_t(r.pin), hsDurUsec, boost, &arr, &arrLen, &err2)
	if res == -1 {
		var err error
		if err2 != nil {
			msg := C.GoString(err2.message)
			err = errors.New(fmt.Sprintf("error during call C.dial_DHTxx_and_read(): %v", msg))
			C.free_error(err2)
		} else {
			err = errors.New("error during call C.dial_DHTxx_and_read()")
		}
		return nil, err
	}
	defer C.free(unsafe.Pointer(arr))

	// convert original C array arr to Go slice list
	h := (*reflect.SliceHeader)(unsafe.Pointer(&list))
	h.Data = uintptr(unsafe.Pointer(arr))
	h.Len = int(arrLen)
	h.Cap = int(arrLen)
	pulses := make([]Pulse, len(list)/2)

	// convert original int array ([pulse, duration, pulse, duration, ...])
	// to Pulse struct array
	for i := 0; i < len(list)/2; i++ {
		var value byte = 0
		if list[i*2] != 0 {
			value = 1
		}

		pulses[i] = Pulse{
			Value:    value,
			Duration: time.Duration(list[i*2+1]) * time.Microsecond,
		}
	}

	return pulses, nil
}

// Decode bunch of pulse read from DHTxx sensors.
// Use pdf specifications from /docs folder to read 5 bytes and
// convert them to temperature and humidity with results validation.
func (r *Reader) decodePulses(pulses []Pulse) (float32, float32, error) {
	if len(pulses) >= 82 && len(pulses) <= 85 {
		pulses = pulses[len(pulses)-82:]
	} else {
		r.printPulseArray(pulses)

		return 0, 0, fmt.Errorf("cannot decode pulse array received from DHTxx sensor, since incorrect length: %d", len(pulses))
	}

	pulses = pulses[:80]
	// any bit low signal part
	tLow := 50 * time.Microsecond
	// 0 bit high signal part
	tHigh0 := 27 * time.Microsecond
	// 1 bit high signal part
	tHigh1 := 70 * time.Microsecond

	// decode 1st byte
	b0, err := r.decodeByte(tLow, tHigh0, tHigh1, 0, pulses)
	if err != nil {
		return 0, 0, err
	}

	// decode 2nd byte
	b1, err := r.decodeByte(tLow, tHigh0, tHigh1, 16, pulses)
	if err != nil {
		return 0, 0, err
	}

	// decode 3rd byte
	b2, err := r.decodeByte(tLow, tHigh0, tHigh1, 32, pulses)
	if err != nil {
		return 0, 0, err
	}

	// decode 4th byte
	b3, err := r.decodeByte(tLow, tHigh0, tHigh1, 48, pulses)
	if err != nil {
		return 0, 0, err
	}

	// decode 5th byte: control sum to verify all data received from sensor
	sum, err := r.decodeByte(tLow, tHigh0, tHigh1, 64, pulses)
	if err != nil {
		return 0, 0, err
	}

	// produce data consistency check
	calcSum := byte(b0 + b1 + b2 + b3)
	if sum != calcSum {
		err := errors.New(fmt.Sprintf(
			"CRCs do not match: checksum from sensor(%v) != calculated checksum(%v=%v+%v+%v+%v)",
			sum, calcSum, b0, b1, b2, b3))

		return 0, 0, err
	}

	r.logger.Debugf("CRCs verified: checksum from sensor(%v) = calculated checksum(%v=%v+%v+%v+%v)", sum, calcSum, b0, b1, b2, b3)

	// debug output for 5 bytes
	r.logger.Debugf("Decoded from DHTxx sensor: [%d, %d, %d, %d, %d]", b0, b1, b2, b3, sum)

	// extract temperature and humidity depending on sensor type
	var temperature, humidity float32

	if r.sensorType == DHT11 {
		humidity = float32(b0)
		temperature = float32(b2)
	} else if r.sensorType == DHT12 {
		humidity = float32(b0) + float32(b1)/10.0
		temperature = float32(b2) + float32(b3)/10.0
		if b3&0x80 != 0 {
			temperature *= -1.0
		}
	} else if r.sensorType == DHT22 {
		humidity = (float32(b0)*256 + float32(b1)) / 10.0
		temperature = (float32(b2&0x7F)*256 + float32(b3)) / 10.0
		if b2&0x80 != 0 {
			temperature *= -1.0
		}
	}

	// additional check for data correctness
	if humidity > 100.0 {
		return 0, 0, fmt.Errorf("humidity value exceeds 100%%: %v", humidity)
	} else if humidity == 0 {
		return 0, 0, fmt.Errorf("humidity value cannot be zero")
	}

	// success
	return temperature, humidity, nil
}

// decodeByte decode data byte from specific pulse array position.
func (r *Reader) decodeByte(tLow, tHigh0, tHigh1 time.Duration, start int, pulses []Pulse) (byte, error) {
	if len(pulses)-start < 16 {
		return 0, fmt.Errorf("cannot decode byte since range between index (%d) and array length (%d) is less than 16", start, len(pulses))
	}

	HIGH_DUR_MAX := tLow + tHigh1
	HIGH_LOW_DUR_AVG := ((tLow+tHigh1)/2 + (tLow+tHigh0)/2) / 2

	var b int = 0
	for i := 0; i < 8; i++ {
		pulseL := pulses[start+i*2]
		pulseH := pulses[start+i*2+1]
		if pulseL.Value != 0 {
			return 0, fmt.Errorf("low edge value expected at index %d", start+i*2)
		}
		if pulseH.Value == 0 {
			return 0, fmt.Errorf("high edge value expected at index %d", start+i*2+1)
		}

		// const HIGH_DUR_MAX = (70 + (70 + 54)) / 2 * time.Microsecond
		// Calc average value between 24us (bit 0) and 70us (bit 1).
		// Everything that less than this param is bit 0, bigger - bit 1.
		// const HIGH_LOW_DUR_AVG = (24 + (70-24)/2) * time.Microsecond
		if pulseH.Duration > HIGH_DUR_MAX {
			return 0, fmt.Errorf("high edge value duration %v exceed maximum expected %v", pulseH.Duration, HIGH_DUR_MAX)
		}

		if pulseH.Duration > HIGH_LOW_DUR_AVG {
			b = b | (1 << uint(7-i))
		}
	}

	return byte(b), nil
}
