// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm
// +build windows,npm

package http

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/etw"
)

type EtwMonitor struct {
	hei        *httpEtwInterface
	telemetry  *telemetry
	statkeeper *httpStatKeeper

	mux         sync.Mutex
	eventLoopWG sync.WaitGroup
}

// NewEtwMonitor returns a new EtwMonitor instance
func NewEtwMonitor(c *config.Config) (Monitor, error) {
	hei := newHttpEtwInterface(c)

	telemetry := newTelemetry()

	return &EtwMonitor{
		hei:        hei,
		telemetry:  telemetry,
		statkeeper: newHTTPStatkeeper(c, telemetry),
	}, nil
}

// Start consuming HTTP events
func (m *EtwMonitor) Start() {
	m.hei.startReadingHttpFlows()

	m.eventLoopWG.Add(1)
	go func() {
		defer m.eventLoopWG.Done()
		for {
			select {
			case transactions, ok := <-m.hei.dataChannel:
				if !ok {
					return
				}
				m.process(transactions)
			}
		}
	}()
}

func (m *EtwMonitor) process(transactions []etw.Http) {
	m.mux.Lock()
	defer m.mux.Unlock()

	for _, transactionEtw := range transactions {
		tx := &etwHttpTX{Http: &transactionEtw}

		m.telemetry.aggregate(tx)

		m.statkeeper.Process(tx)
	}
}

func (m *EtwMonitor) removeDuplicates(stats map[Key]RequestStats) {
	// With localhost traffic, the driver will create a flow for both endpoints. Both
	// these flows will be normalized so that source=client and dest=server, which
	// results in 2 identical http transactions being sent up to userspace & processed.
	// To fix this, we'll find all localhost keys and half their transaction counts.

	for k, v := range stats {
		if isLocalhost(k) {
			for i := 0; i < NumStatusClasses; i++ {
				v[i].Count = v[i].Count / 2
				stats[k] = v
			}
		}
	}
}

// GetHTTPStats returns a map of HTTP stats stored in the following format:
// [source, dest tuple, request path] -> RequestStats object
func (m *EtwMonitor) GetHTTPStats() map[Key]RequestStats {

	transactions := m.hei.getHttpFlows()
	if transactions == nil {
		return nil
	}

	m.process(transactions)

	m.mux.Lock()
	defer m.mux.Unlock()

	stats := m.statkeeper.GetAndResetAllStats()
	m.removeDuplicates(stats)

	delta := m.telemetry.reset()
	delta.report()

	return stats
}

// GetStats gets driver stats related to the HTTP handle
func (m *EtwMonitor) GetStats() (map[string]int64, error) {
	return m.hei.getStats()
}

// Stop HTTP monitoring
func (m *EtwMonitor) Stop() error {
	m.hei.close()
	m.eventLoopWG.Wait()
	return nil
}
