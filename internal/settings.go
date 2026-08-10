package internal

import (
	"fmt"
	"strings"

	"github.com/Muxcore-Media/core/pkg/contracts"
)

func (m *Module) Settings() []contracts.SettingDef {
	return m.settingsDefs()
}

func (m *Module) UpdateSetting(key, value string) error {
	return m.updateSetting(key, value)
}

func (m *Module) settingsDefs() []contracts.SettingDef {
	m.cfgMu.RLock()
	path := m.metricsPath
	m.cfgMu.RUnlock()
	return []contracts.SettingDef{
		{
			Key:         "metrics_path",
			Label:       "Scrape Path",
			Type:        contracts.SettingTypeString,
			Value:       path,
			Default:     "/metrics",
			Description: "HTTP path for Prometheus scrapes (METRICS_PATH); updates apply live",
			Group:       "Scrape",
		},
	}
}

func (m *Module) updateSetting(key, value string) error {
	value = strings.TrimSpace(value)
	switch key {
	case "metrics_path", "METRICS_PATH":
		path := normalizePath(value)
		if path == "/" {
			return fmt.Errorf("metrics_path must not be root /")
		}
		m.cfgMu.Lock()
		m.metricsPath = path
		m.cfgMu.Unlock()
		if m.scrape != nil {
			m.scrape.setPath(path)
		}
		return nil
	default:
		return fmt.Errorf("unknown setting %q", key)
	}
}
