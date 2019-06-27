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

import (
	"time"
)

// SensorType signify what sensor in use.
type SensorType int

// String implement Stringer interface.
func (v SensorType) String() string {
	if v == DHT11 {
		return "DHT11"
	} else if v == DHT12 {
		return "DHT12"
	} else if v == DHT22 || v == AM2302 {
		return "DHT22|AM2302"
	} else {
		return "!!! unknown !!!"
	}
}

// GetHandshakeDuration specify signal duration necessary
// to initiate sensor response.
func (v SensorType) GetHandshakeDuration() time.Duration {
	if v == DHT11 {
		return 18 * time.Millisecond
	} else if v == DHT12 {
		return 200 * time.Millisecond
	} else if v == DHT22 || v == AM2302 {
		return 18 * time.Millisecond
	} else {
		return 0
	}
}

// GetRetryTimeout return recommended timeout necessary
// to wait before new round of data exchange.
func (v SensorType) GetRetryTimeout() time.Duration {
	return 1500 * time.Millisecond
}

const (
	// DHT11 is most popular sensor
	DHT11 SensorType = iota + 1
	// DHT12 is more precise than DHT11 (has scale parts)
	DHT12
	// DHT22 is more expensive and precise than DHT11
	DHT22
	// AM2302 aka DHT22
	AM2302 = DHT22
)

// Pulse keep pulse signal state with how long it lasted.
type Pulse struct {
	Value    byte
	Duration time.Duration
}
