package top

import (
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"
)

const (
	keyQuit      = "q"
	keyPause     = "space"
	keyReset     = "r"
	keyCompute   = "c"
	keyMemory    = "m"
	keyNvlink    = "n"
	keyPcie      = "p"
	keyBandwidth = "b"
	keyDetail    = "d"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typedMsg := msg.(type) {
	case tea.BackgroundColorMsg:
		m.dark = typedMsg.IsDark()
		m.applyTheme()
		if !m.initialized {
			return m, m.spinner.Tick
		}
		return m, m.beginDraw()
	case tea.InterruptMsg:
		m.quitting = true
		return m, tea.Quit
	case tea.KeyPressMsg:
		switch typedMsg.String() {
		case "ctrl+c", keyQuit:
			m.quitting = true
			return m, tea.Quit
		case keyCompute:
			m.toggleSeries(computeSeries)
			return m, m.beginDraw()
		case keyMemory:
			m.toggleSeries(memorySeries)
			return m, m.beginDraw()
		case keyNvlink:
			if !m.showBandwidth {
				return m, nil
			}
			m.toggleSeries(nvlinkSeries)
			return m, m.beginDraw()
		case keyPcie:
			if !m.showBandwidth {
				return m, nil
			}
			m.toggleSeries(pcieSeries)
			return m, m.beginDraw()
		case keyReset:
			m.resetCharts()
			return m, m.beginDraw()
		case keyPause:
			m.paused = !m.paused
			if m.paused {
				m.pausedAt = time.Now()
			} else {
				m.pausedAt = time.Time{}
			}
			return m, m.beginDraw()
		case keyBandwidth:
			m.showBandwidth = !m.showBandwidth
			m.applyLayout()
			return m, m.beginDraw()
		case keyDetail:
			m.detailMode = !m.detailMode
			m.applyDetailMode()
			return m, m.beginDraw()
		default:
			return m, nil
		}
	case tea.WindowSizeMsg:
		m.width = max(typedMsg.Width, 1)
		m.height = max(typedMsg.Height, 1)
		m.applyLayout()
		return m, m.beginDraw()
	case InitMsg:
		m.initCharts(typedMsg.DeviceIDs)
		m.initialized = true
		m.applyLayout()
		return m, m.beginDraw()
	case drawMsg:
		m.draw()
		if m.paused {
			return m, nil
		}
		return m, m.tick()
	case MetricsSnapshotMsg:
		for _, snapshot := range typedMsg.DeviceSnapshots {
			chartIdx, ok := m.deviceIndexMap[snapshot.DeviceID]
			if !ok || chartIdx < 0 || chartIdx >= len(m.solCharts) {
				continue
			}
			chart := m.solCharts[chartIdx]
			if chart == nil {
				continue
			}
			m.online[chartIdx] = true
			if m.paused {
				continue
			}
			chart.Push(computeSeries, typedMsg.Timestamp, snapshot.ComputeSOLPct)
			chart.Push(memorySeries, typedMsg.Timestamp, snapshot.MemorySOLPct)
			m.computeLastValues[chartIdx] = snapshot.ComputeSOLPct
			m.memoryLastValues[chartIdx] = snapshot.MemorySOLPct
		}

		if !m.paused && len(typedMsg.BandwidthSnapshots) > 0 {
			timestamp := typedMsg.Timestamp
			var pcieBytesPerSecond float64
			var nvlinkBytesPerSecond float64
			for _, snapshot := range typedMsg.BandwidthSnapshots {
				pcieBytesPerSecond += snapshot.PCIeTxBytesPerSecond + snapshot.PCIeRxBytesPerSecond
				nvlinkBytesPerSecond += snapshot.NVLinkTxBytesPerSecond + snapshot.NVLinkRxBytesPerSecond
			}
			m.bandwidthChart.Push(pcieSeries, timestamp, pcieBytesPerSecond)
			m.bandwidthChart.Push(nvlinkSeries, timestamp, nvlinkBytesPerSecond)
			m.pcieLastValue = pcieBytesPerSecond
			m.nvlinkLastValue = nvlinkBytesPerSecond
		}
		return m, nil
	case RooflineCeilingMsg:
		m.gpuCeilings = typedMsg.PerGPU
		m.applyCeilingThresholds()
		m.applyLayout()
		if m.ready() {
			m.draw()
		}
		return m, nil
	case ErrorMsg:
		m.err = typedMsg.Error
		return m, nil
	default:
		if !m.initialized {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil
	}
}

func (m model) beginDraw() tea.Cmd {
	if !m.ready() {
		return nil
	}

	return func() tea.Msg {
		return drawMsg{}
	}
}

func (m *model) toggleSeries(series string) {
	i := slices.Index(m.enabledSeries, series)
	if i == -1 {
		m.enabledSeries = append(m.enabledSeries, series)
		return
	}
	m.enabledSeries = slices.Delete(m.enabledSeries, i, i+1)
}

func (m model) seriesEnabled(series string) bool {
	return slices.Contains(m.enabledSeries, series)
}
