package traefik_maintenance

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/template"
)

type Config struct {
	Enabled          bool     `json:"enabled"`
	FileName         string   `json:"fileName"`
	TriggerUrl       string   `json:"triggerUrl"`
	HttpResponseCode int      `json:"httpResponseCode"`
	HttpContentType  string   `json:"httpContentType"`
	WhiteListIps     []string `json:"whiteListIps"`
}

type MaintenancePage struct {
	next             http.Handler
	enabled          bool
	fileName         string
	triggerUrl       string
	httpResponseCode int
	httpContentType  string
	whiteListIps     []net.IPNet
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
		WhiteListIps:     make([]string, 0),
	}
}

func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	if len(config.FileName) == 0 {
		return nil, fmt.Errorf("file_name is required")
	}

	var whiteIps []net.IPNet
	for _, ip := range config.WhiteListIps {
		_, ipnet, err := net.ParseCIDR(ip)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR: %v", err)
		}
		whiteIps = append(whiteIps, *ipnet)
	}

	return &MaintenancePage{
		enabled:          config.Enabled,
		fileName:         config.FileName,
		triggerUrl:       config.TriggerUrl,
		httpResponseCode: config.HttpResponseCode,
		httpContentType:  config.HttpContentType,
		whiteListIps:     whiteIps,
		next:             next,
		name:             name,
		template:         template.New("MaintenancePage").Delims("[[", "]]"),
	}, nil
}

func (m *MaintenancePage) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if m.maintenanceEnabled() && m.triggerMaintenance(m.triggerUrl) && !m.isWhiteListed(m.getRealIP(r)) {
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

func (m *MaintenancePage) isWhiteListed(ipStr string) bool {
	ipPortPair := strings.Split(ipStr, ":")
	for _, whiteListIp := range m.whiteListIps {
		ip := net.ParseIP(ipPortPair[0])
		if whiteListIp.Contains(ip) {
			return true
		}
	}
	return false
}

func (m *MaintenancePage) getRealIP(r *http.Request) string {
	// Check X-Forwarded-For header
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		ips := strings.Split(xff, ",")
		ip := strings.TrimSpace(ips[0])
		if ip != "" {
			return ip
		}
	}
	// Check X-Real-IP header
	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		return xri
	}
	// Fallback to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return ip
	}
	return r.RemoteAddr
}
