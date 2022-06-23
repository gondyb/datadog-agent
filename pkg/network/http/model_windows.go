// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm
// +build windows,npm

package http

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/driver"
	"github.com/DataDog/datadog-agent/pkg/network/etw"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"golang.org/x/sys/windows"
)

const HTTPBufferSize = driver.HttpBufferSize
const HTTPBatchSize = driver.HttpBatchSize

type httpTX interface {
	ReqFragment() []byte
	StatusClass() int
	RequestLatency() float64
	isIPV4() bool
	SrcIPLow() uint64
	SrcIPHigh() uint64
	SrcPort() uint16
	DstIPLow() uint64
	DstIPHigh() uint64
	DstPort() uint16
	Method() Method
	StatusCode() uint16
	StaticTags() uint64
	DynamicTags() []string
	Incomplete() bool
}

type driverHttpTX struct {
	//	httpTX
	*driver.HttpTransactionType
}

type etwHttpTX struct {
	//	httpTX
	*etw.Http
}

// errLostBatch isn't a valid error in windows
var errLostBatch = errors.New("invalid error")

// StatusClass returns an integer representing the status code class
// Example: a 404 would return 400
func statusClass(statusCode uint16) int {
	return (int(statusCode) / 100) * 100
}

func requestLatency(responseLastSeen uint64, requestStarted uint64) float64 {
	return nsTimestampToFloat(uint64(responseLastSeen - requestStarted))
}

func isIPV4(tup *driver.ConnTupleType) bool {
	return tup.Family == windows.AF_INET
}

func ipLow(isIp4 bool, addr [16]uint8) uint64 {
	// Source & dest IP are given to us as a 16-byte slices in network byte order (BE). To convert to
	// low/high representation, we must convert to host byte order (LE).
	if isIp4 {
		return uint64(binary.LittleEndian.Uint32(addr[:4]))
	}
	return binary.LittleEndian.Uint64(addr[8:])
}

func ipHigh(isIp4 bool, addr [16]uint8) uint64 {
	if isIp4 {
		return uint64(0)
	}
	return binary.LittleEndian.Uint64(addr[:8])
}

func srcIPLow(tup *driver.ConnTupleType) uint64 {
	return ipLow(isIPV4(tup), tup.CliAddr)
}

func srcIPHigh(tup *driver.ConnTupleType) uint64 {
	return ipHigh(isIPV4(tup), tup.CliAddr)
}

func dstIPLow(tup *driver.ConnTupleType) uint64 {
	return ipLow(isIPV4(tup), tup.SrvAddr)
}

func dstIPHigh(tup *driver.ConnTupleType) uint64 {
	return ipHigh(isIPV4(tup), tup.SrvAddr)
}

// --------------------------
//
// driverHttpTX interface
//

// ReqFragment returns a byte slice containing the first HTTPBufferSize bytes of the request
func (tx *driverHttpTX) ReqFragment() []byte {
	return tx.RequestFragment[:]
}

func (tx *driverHttpTX) StatusClass() int {
	return statusClass(tx.ResponseStatusCode)
}

func (tx *driverHttpTX) RequestLatency() float64 {
	return requestLatency(tx.ResponseLastSeen, tx.RequestStarted)
}

func (tx *driverHttpTX) isIPV4() bool {
	return isIPV4(&tx.Tup)
}

func (tx *driverHttpTX) SrcIPLow() uint64 {
	return srcIPLow(&tx.Tup)
}

func (tx *driverHttpTX) SrcIPHigh() uint64 {
	return srcIPHigh(&tx.Tup)
}

func (tx *driverHttpTX) SrcPort() uint16 {
	return tx.Tup.CliPort
}

func (tx *driverHttpTX) DstIPLow() uint64 {
	return dstIPLow(&tx.Tup)
}

func (tx *driverHttpTX) DstIPHigh() uint64 {
	return dstIPHigh(&tx.Tup)
}

func (tx *driverHttpTX) DstPort() uint16 {
	return tx.Tup.SrvPort
}

func (tx *driverHttpTX) Method() Method {
	return Method(tx.RequestMethod)
}

func (tx *driverHttpTX) StatusCode() uint16 {
	return tx.ResponseStatusCode
}

// Static Tags are not part of windows driver http transactions
func (tx *driverHttpTX) StaticTags() uint64 {
	return 0
}

// Dynamic Tags are not part of windows driver http transactions
func (tx *driverHttpTX) DynamicTags() []string {
	return nil
}

// Incomplete transactions does not apply to windows
func (tx *driverHttpTX) Incomplete() bool {
	return false
}

// --------------------------
//
// etwHttpTX interface
//

// ReqFragment returns a byte slice containing the first HTTPBufferSize bytes of the request
func (tx *etwHttpTX) ReqFragment() []byte {
	return tx.RequestFragment[:]
}

func (tx *etwHttpTX) StatusClass() int {
	return statusClass(tx.ResponseStatusCode)
}

func (tx *etwHttpTX) RequestLatency() float64 {
	return requestLatency(tx.ResponseLastSeen, tx.RequestStarted)
}

func (tx *etwHttpTX) isIPV4() bool {
	return isIPV4(&tx.Tup)
}

func (tx *etwHttpTX) SrcIPLow() uint64 {
	return srcIPLow(&tx.Tup)
}

func (tx *etwHttpTX) SrcIPHigh() uint64 {
	return srcIPHigh(&tx.Tup)
}

func (tx *etwHttpTX) SrcPort() uint16 {
	return tx.Tup.CliPort
}

func (tx *etwHttpTX) DstIPLow() uint64 {
	return dstIPLow(&tx.Tup)
}

func (tx *etwHttpTX) DstIPHigh() uint64 {
	return dstIPHigh(&tx.Tup)
}

func (tx *etwHttpTX) DstPort() uint16 {
	return tx.Tup.SrvPort
}

func (tx *etwHttpTX) Method() Method {
	return Method(tx.RequestMethod)
}

func (tx *etwHttpTX) StatusCode() uint16 {
	return tx.ResponseStatusCode
}

// Static Tags are not part of windows http transactions
func (tx *etwHttpTX) StaticTags() uint64 {
	return 0
}

// Dynamic Tags are  part of windows http transactions
func (tx *etwHttpTX) DynamicTags() []string {
	return []string{fmt.Sprintf("http.iis.app_pool:%v", tx.AppPool)}
}

// Incomplete transactions does not apply to windows
func (tx *etwHttpTX) Incomplete() bool {
	return false
}

// below is copied from pkg/trace/stats/statsraw.go
// 10 bits precision (any value will be +/- 1/1024)
const roundMask uint64 = 1 << 10

// nsTimestampToFloat converts a nanosec timestamp into a float nanosecond timestamp truncated to a fixed precision
func nsTimestampToFloat(ns uint64) float64 {
	var shift uint
	for ns > roundMask {
		ns = ns >> 1
		shift++
	}
	return float64(ns << shift)
}

// generateIPv4HTTPTransaction is a testing helper function required for the http_statkeeper tests
func generateIPv4HTTPTransaction(client util.Address, server util.Address, cliPort int, srvPort int, path string, code int, latency time.Duration) httpTX {
	var tx driverHttpTX

	reqFragment := fmt.Sprintf("GET %s HTTP/1.1\nHost: example.com\nUser-Agent: example-browser/1.0", path)
	latencyNS := uint64(uint64(latency))
	cli := client.Bytes()
	srv := server.Bytes()

	tx.RequestStarted = 1
	tx.ResponseLastSeen = tx.RequestStarted + latencyNS
	tx.ResponseStatusCode = uint16(code)
	for i := 0; i < len(tx.RequestFragment) && i < len(reqFragment); i++ {
		tx.RequestFragment[i] = uint8(reqFragment[i])
	}
	for i := 0; i < len(tx.Tup.CliAddr) && i < len(cli); i++ {
		tx.Tup.CliAddr[i] = cli[i]
	}
	for i := 0; i < len(tx.Tup.SrvAddr) && i < len(srv); i++ {
		tx.Tup.SrvAddr[i] = srv[i]
	}
	tx.Tup.CliPort = uint16(cliPort)
	tx.Tup.SrvPort = uint16(srvPort)

	return &tx
}
