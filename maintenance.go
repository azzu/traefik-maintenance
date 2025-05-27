package traefik_maintenance

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"text/template"
)

type Config struct {
	Enabled          bool   `json:"enabled"`
	FileName         string `json:"fileName"`
	TriggerUrl       string `json:"triggerUrl"`
	HttpResponseCode int    `json:"httpResponseCode"`
	HttpContentType  string `json:"httpContentType"`
}

type MaintenancePage struct {
	next             http.Handler
	enabled          bool
	fileName         string
	triggerUrl       string
	httpResponseCode int
	httpContentType  string
	name             string
	template         *template.Template
}

func CreateConfig() *Config {
	return &Config{
		Enabled:          false,
		FileName:         "",
		TriggerUrl:       "",
		HttpResponseCode: http.StatusOK,
		HttpContentType:  "text/html; charset=utf-8",
	}
}

func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	if len(config.FileName) == 0 {
		return nil, fmt.Errorf("file_name is required")
	}

	return &MaintenancePage{
		enabled:          config.Enabled,
		fileName:         config.FileName,
		triggerUrl:       config.TriggerUrl,
		httpResponseCode: config.HttpResponseCode,
		httpContentType:  config.HttpContentType,
		next:             next,
		name:             name,
		template:         template.New("MaintenancePage").Delims("[[", "]]"),
	}, nil
}

func (m *MaintenancePage) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if m.maintenanceEnabled() && m.triggerMaintenance(m.triggerUrl) {
		if m.isURL(m.fileName) {
			resp, err := http.Get(m.fileName)
			if err != nil {
				log.Printf("Error checking maintenance status: %v", err)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				w.Header().Set("Content-Type", m.httpContentType)
				w.WriteHeader(m.httpResponseCode)
				io.Copy(w, resp.Body)
				return
			} else {
				log.Printf("Error checking maintenance status: %v", resp.StatusCode)
			}
		} else {
			bytes, err := os.ReadFile(m.fileName)
			if err != nil {
				log.Printf("Error reading file: %v", err)
			}
			w.Header().Set("Content-Type", m.httpContentType)
			w.WriteHeader(m.httpResponseCode)
			w.Write(bytes)
		}
	}
	m.next.ServeHTTP(w, r)
}

func (m *MaintenancePage) maintenanceEnabled() bool {
	if !m.enabled {
		return false
	}

	if m.enabled && len(m.triggerUrl) > 0 {
		return true
	}

	if _, err := os.Stat(m.fileName); err == nil {
		return true
	}

	return false
}

func (m *MaintenancePage) isURL(triggerUrl string) bool {
	u, err := url.Parse(triggerUrl)
	if err != nil {
		return false
	}
	return u.Scheme != "" && u.Host != ""
}

func (m *MaintenancePage) triggerMaintenance(triggerUrl string) bool {
	resp, err := http.Get(triggerUrl)
	if err != nil {
		log.Printf("Error triggering maintenance: %v", err)
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
