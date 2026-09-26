package main

/*
#include <stdint.h>
#include <stdlib.h>

typedef struct {
	void* ptr;
	size_t len;
} cliproxy_buffer;

typedef int (*cliproxy_host_call_fn)(void*, const char*, const uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_host_free_fn)(void*, size_t);

typedef struct {
	uint32_t abi_version;
	void* host_ctx;
	cliproxy_host_call_fn call;
	cliproxy_host_free_fn free_buffer;
} cliproxy_host_api;

typedef int (*cliproxy_plugin_call_fn)(char*, uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_plugin_free_fn)(void*, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);

typedef struct {
	uint32_t abi_version;
	cliproxy_plugin_call_fn call;
	cliproxy_plugin_free_fn free_buffer;
	cliproxy_plugin_shutdown_fn shutdown;
} cliproxy_plugin_api;

extern int cliproxyPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
extern void cliproxyPluginFree(void*, size_t);
extern void cliproxyPluginShutdown(void);

static const cliproxy_host_api* stored_host;

static void store_host_api(const cliproxy_host_api* host) {
	stored_host = host;
}

static int call_host_api(const char* method, const uint8_t* request, size_t request_len, cliproxy_buffer* response) {
	if (stored_host == NULL || stored_host->call == NULL) {
		return 1;
	}
	return stored_host->call(stored_host->host_ctx, method, request, request_len, response);
}

static void free_host_buffer(void* ptr, size_t len) {
	if (stored_host != NULL && stored_host->free_buffer != NULL && ptr != NULL) {
		stored_host->free_buffer(ptr, len);
	}
}
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
	"unsafe"

	"gopkg.in/yaml.v3"
)

const (
	abiVersion    = uint32(1)
	schemaVersion = uint32(6)
	pluginName    = "glm-quota"
	pluginVer     = "0.3.0"
	resourcePath  = "/status"
	mgmtRoutePath = "/glm-quota"
)

type pluginConfig struct {
	Provider string `yaml:"provider"`
	APIKey   string `yaml:"api-key"`
	BaseURL  string `yaml:"base-url"`
}

var (
	cfgMu    sync.RWMutex
	cfg      pluginConfig
	cacheMu  sync.Mutex
	cacheRaw []byte
	cacheAt  time.Time
)

func baseURL() string {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	if cfg.BaseURL != "" {
		return strings.TrimRight(cfg.BaseURL, "/")
	}
	if strings.EqualFold(cfg.Provider, "bigmodel") {
		return "https://open.bigmodel.cn"
	}
	return "https://api.z.ai"
}

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *envError       `json:"error,omitempty"`
}

type envError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func okEnvelope(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope{OK: true, Result: raw})
}

func errorEnvelope(code, msg string) []byte {
	raw, _ := json.Marshal(envelope{OK: false, Error: &envError{Code: code, Message: msg}})
	return raw
}

func main() {}

//export cliproxy_plugin_init
func cliproxy_plugin_init(host *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if plugin == nil {
		return 1
	}
	C.store_host_api(host)
	plugin.abi_version = C.uint32_t(abiVersion)
	plugin.call = C.cliproxy_plugin_call_fn(C.cliproxyPluginCall)
	plugin.free_buffer = C.cliproxy_plugin_free_fn(C.cliproxyPluginFree)
	plugin.shutdown = C.cliproxy_plugin_shutdown_fn(C.cliproxyPluginShutdown)
	return 0
}

//export cliproxyPluginCall
func cliproxyPluginCall(method *C.char, request *C.uint8_t, requestLen C.size_t, response *C.cliproxy_buffer) C.int {
	if response != nil {
		response.ptr = nil
		response.len = 0
	}
	if method == nil {
		writeResponse(response, errorEnvelope("invalid_method", "method is required"))
		return 1
	}
	var reqBytes []byte
	if request != nil && requestLen > 0 {
		reqBytes = C.GoBytes(unsafe.Pointer(request), C.int(requestLen))
	}
	raw, err := handleMethod(C.GoString(method), reqBytes)
	if err != nil {
		writeResponse(response, errorEnvelope("plugin_error", err.Error()))
		return 1
	}
	writeResponse(response, raw)
	return 0
}

//export cliproxyPluginFree
func cliproxyPluginFree(ptr unsafe.Pointer, length C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
	_ = length
}

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() {}

func writeResponse(response *C.cliproxy_buffer, raw []byte) {
	if response == nil || len(raw) == 0 {
		return
	}
	ptr := C.CBytes(raw)
	if ptr == nil {
		return
	}
	response.ptr = ptr
	response.len = C.size_t(len(raw))
}

type lifecycleRequest struct {
	ConfigYAML []byte `json:"config_yaml"`
}

type configField struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

type registration struct {
	SchemaVersion uint32       `json:"schema_version"`
	Metadata      metadata     `json:"metadata"`
	Capabilities  capabilities `json:"capabilities"`
}

type metadata struct {
	Name             string        `json:"Name"`
	Version          string        `json:"Version"`
	Author           string        `json:"Author"`
	GitHubRepository string        `json:"GitHubRepository"`
	ConfigFields     []configField `json:"ConfigFields"`
}

type capabilities struct {
	ManagementAPI bool `json:"management_api"`
}

type managementRoute struct {
	Method      string `json:"Method"`
	Path        string `json:"Path"`
	Description string `json:"Description,omitempty"`
}

type managementResource struct {
	Path        string `json:"Path"`
	Menu        string `json:"Menu"`
	Description string `json:"Description,omitempty"`
}

type managementRegistration struct {
	Routes    []managementRoute    `json:"routes,omitempty"`
	Resources []managementResource `json:"resources,omitempty"`
}

type managementRequest struct {
	Method  string      `json:"Method"`
	Path    string      `json:"Path"`
	Query   url.Values  `json:"Query"`
	Headers http.Header `json:"Headers"`
	Body    []byte      `json:"Body"`
}

type managementResponse struct {
	StatusCode int         `json:"StatusCode"`
	Headers    http.Header `json:"Headers"`
	Body       []byte      `json:"Body"`
}

func handleMethod(method string, request []byte) ([]byte, error) {
	switch method {
	case "plugin.register", "plugin.reconfigure":
		if err := loadConfig(request); err != nil {
			return nil, err
		}
		return okEnvelope(registration{
			SchemaVersion: schemaVersion,
			Metadata: metadata{
				Name:             pluginName,
				Version:          pluginVer,
				Author:           "TheMountainTree",
				GitHubRepository: "https://github.com/TheMountainTree/CPA-GLM-Quota",
				ConfigFields: []configField{
					{Name: "provider", Type: "string", Description: "zai (default) or bigmodel"},
					{Name: "api-key", Type: "string", Description: "GLM Coding Plan API key"},
					{Name: "base-url", Type: "string", Description: "Optional override of the monitor API base URL"},
				},
			},
			Capabilities: capabilities{ManagementAPI: true},
		})
	case "management.register":
		return okEnvelope(managementRegistration{
			Routes: []managementRoute{{
				Method:      "GET",
				Path:        mgmtRoutePath,
				Description: "GLM Coding Plan quota inspection endpoint",
			}},
			Resources: []managementResource{{
				Path:        resourcePath,
				Menu:        "GLM Quota",
				Description: "GLM Coding Plan 5h / weekly quota dashboard",
			}},
		})
	case "management.handle":
		return handleManagement(request)
	default:
		return errorEnvelope("unknown_method", "unknown method: "+method), nil
	}
}

func loadConfig(raw []byte) error {
	var req lifecycleRequest
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &req); err != nil {
			return fmt.Errorf("decode lifecycle request: %w", err)
		}
	}
	newCfg := pluginConfig{}
	if len(req.ConfigYAML) > 0 {
		if err := yaml.Unmarshal(req.ConfigYAML, &newCfg); err != nil {
			return fmt.Errorf("decode config: %w", err)
		}
	}
	cfgMu.Lock()
	cfg = newCfg
	cfgMu.Unlock()
	cacheMu.Lock()
	cacheRaw, cacheAt = nil, time.Time{}
	cacheMu.Unlock()
	return nil
}

func handleManagement(raw []byte) ([]byte, error) {
	var req managementRequest
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, fmt.Errorf("decode management request: %w", err)
		}
	}

	wantJSON := false
	if req.Query != nil && strings.EqualFold(req.Query.Get("format"), "json") {
		wantJSON = true
	} else if req.Headers != nil && strings.Contains(strings.ToLower(req.Headers.Get("Accept")), "application/json") {
		wantJSON = true
	}

	cfgMu.RLock()
	key := cfg.APIKey
	provider := cfg.Provider
	cfgMu.RUnlock()

	if key == "" {
		msg := "glm-quota: api-key is empty. Configure plugins.configs.glm-quota.api-key in config.yaml."
		if wantJSON {
			return okEnvelope(jsonResponse(http.StatusServiceUnavailable, map[string]string{"error": msg}))
		}
		return okEnvelope(htmlResponse(http.StatusServiceUnavailable, renderError(msg)))
	}

	quota, status, err := fetchQuota(key, provider)
	if err != nil {
		if wantJSON {
			return okEnvelope(jsonResponse(status, map[string]string{"error": err.Error()}))
		}
		return okEnvelope(htmlResponse(status, renderError(err.Error())))
	}
	if wantJSON {
		return okEnvelope(jsonResponse(http.StatusOK, quota))
	}
	return okEnvelope(htmlResponse(http.StatusOK, renderDashboardHTML(quota)))
}

type limitEntry struct {
	Type          string `json:"type"`
	Unit          int    `json:"unit"`
	Percentage    int    `json:"percentage"`
	NextResetTime int64  `json:"nextResetTime"`
	Usage         *int   `json:"usage,omitempty"`
	CurrentValue  *int   `json:"currentValue,omitempty"`
	Remaining     *int   `json:"remaining,omitempty"`
}

type quotaPayload struct {
	Code    int    `json:"code"`
	Message string `json:"msg"`
	Data    struct {
		Limits []limitEntry `json:"limits"`
		Level  string       `json:"level"`
	} `json:"data"`
	Success bool `json:"success"`
}

type quotaView struct {
	Level     string       `json:"level"`
	FetchedAt string       `json:"fetched_at"`
	Window5h  *windowView  `json:"window_5h,omitempty"`
	Weekly    *windowView  `json:"weekly,omitempty"`
	MCP       *windowView  `json:"mcp_monthly,omitempty"`
	RawLimits []limitEntry `json:"raw_limits,omitempty"`
}

type windowView struct {
	UsedPct      int    `json:"used_pct"`
	RemainingPct int    `json:"remaining_pct"`
	ResetInSec   int64  `json:"reset_in_sec"`
	ResetAt      string `json:"reset_at"`
	CurrentCount *int   `json:"current_count,omitempty"`
	TotalCount   *int   `json:"total_count,omitempty"`
	Remaining    *int   `json:"remaining_count,omitempty"`
}

func fetchQuota(key, provider string) (*quotaView, int, error) {
	cacheMu.Lock()
	if cacheRaw != nil && time.Since(cacheAt) < 60*time.Second {
		cached := cacheRaw
		cacheMu.Unlock()
		out := &quotaView{}
		if err := json.Unmarshal(cached, out); err == nil {
			return out, http.StatusOK, nil
		}
	} else {
		cacheMu.Unlock()
	}

	targetURL := baseURL() + "/api/monitor/usage/quota/limit"
	req, err := http.NewRequest(http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, http.StatusBadGateway, fmt.Errorf("glm-quota: upstream request failed: %w", err)
	}
	defer resp.Body.Close()

	body := make([]byte, 0, 4096)
	buf := make([]byte, 2048)
	for {
		n, readErr := resp.Body.Read(buf)
		body = append(body, buf[:n]...)
		if readErr != nil {
			break
		}
		if len(body) > 1<<20 {
			break
		}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, fmt.Errorf("glm-quota: upstream HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var payload quotaPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, http.StatusBadGateway, fmt.Errorf("glm-quota: upstream JSON decode failed: %w", err)
	}
	if payload.Code != 200 && payload.Code != 0 {
		return nil, http.StatusBadGateway, fmt.Errorf("glm-quota: upstream error: %s", truncate(payload.Message, 200))
	}

	view := &quotaView{
		Level:     strings.ToUpper(payload.Data.Level),
		FetchedAt: time.Now().Format("2006-01-02 15:04:05"),
		RawLimits: payload.Data.Limits,
	}

	var creditOrTokenLimits []limitEntry
	for _, l := range payload.Data.Limits {
		t := strings.ToUpper(l.Type)
		isUsageLimit := strings.Contains(t, "TOKEN") || strings.Contains(t, "CREDIT")

		switch {
		case isUsageLimit && l.Unit == 3:
			view.Window5h = window(&l)
		case isUsageLimit && l.Unit == 6:
			view.Weekly = window(&l)
		case (t == "TIME_LIMIT" || strings.Contains(t, "MCP")) && (l.Unit == 5 || l.Unit == 0):
			view.MCP = window(&l)
		case isUsageLimit:
			creditOrTokenLimits = append(creditOrTokenLimits, l)
		}
	}

	if (view.Window5h == nil || view.Weekly == nil) && len(creditOrTokenLimits) > 0 {
		sort.Slice(creditOrTokenLimits, func(i, j int) bool {
			return creditOrTokenLimits[i].NextResetTime < creditOrTokenLimits[j].NextResetTime
		})
		if view.Window5h == nil {
			view.Window5h = window(&creditOrTokenLimits[0])
		}
		if view.Weekly == nil && len(creditOrTokenLimits) > 1 {
			view.Weekly = window(&creditOrTokenLimits[1])
		}
	}

	if raw, err := json.Marshal(view); err == nil {
		cacheMu.Lock()
		cacheRaw = raw
		cacheAt = time.Now()
		cacheMu.Unlock()
	}
	return view, http.StatusOK, nil
}

func window(l *limitEntry) *windowView {
	sec := l.NextResetTime/1000 - time.Now().Unix()
	if sec < 0 {
		sec = 0
	}
	used := clamp(l.Percentage, 0, 100)
	return &windowView{
		UsedPct:      used,
		RemainingPct: 100 - used,
		ResetInSec:   sec,
		ResetAt:      time.Unix(l.NextResetTime/1000, 0).Format("01/02 15:04"),
		CurrentCount: l.CurrentValue,
		TotalCount:   l.Usage,
		Remaining:    l.Remaining,
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func jsonResponse(status int, v any) managementResponse {
	raw, _ := json.Marshal(v)
	return managementResponse{
		StatusCode: status,
		Headers:    http.Header{"Content-Type": []string{"application/json; charset=utf-8"}},
		Body:       raw,
	}
}

func htmlResponse(status int, body []byte) managementResponse {
	return managementResponse{
		StatusCode: status,
		Headers:    http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
		Body:       body,
	}
}

func renderError(msg string) []byte {
	var b strings.Builder
	b.WriteString("<!doctype html><html><head><meta charset=\"utf-8\"><title>GLM 配额 - 错误</title></head>")
	b.WriteString(`<body style="font-family:-apple-system,BlinkMacSystemFont,Segoe UI,Roboto,sans-serif;max-width:640px;margin:40px auto;padding:0 20px;color:var(--error-color,#c65746);background:var(--bg-primary,#f0eee8)">`)
	b.WriteString(`<h3>GLM Quota 插件错误</h3><p style="color:var(--text-secondary,#6d6760)">`)
	b.WriteString(html.EscapeString(msg))
	b.WriteString(`</p></body></html>`)
	return []byte(b.String())
}

func renderDashboardHTML(v *quotaView) []byte {
	var b strings.Builder
	b.WriteString(`<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>GLM Quota Dashboard</title>
<style>
/* CPAMC Exact Design System & Tokens */
:root {
  --bg-secondary: #faf9f5;
  --bg-primary: #f0eee8;
  --bg-tertiary: #e9e6df;
  --bg-hover: var(--bg-tertiary);
  --bg-quinary: #f6f4ee;
  --floating-surface: #fffdf9;
  --floating-shadow: 0 12px 26px #00000024;
  --text-primary: #2d2a26;
  --text-secondary: #6d6760;
  --text-tertiary: #a29c95;
  --text-quaternary: #c0bab3;
  --text-muted: var(--text-tertiary);
  --border-color: #e3e1db;
  --border-secondary: var(--border-color);
  --border-primary: #d5d2cb;
  --border-hover: #cecac4;
  --primary-color: #8b8680;
  --primary-hover: #7f7a74;
  --primary-active: #726d67;
  --primary-contrast: #fff;
  --success-color: #10b981;
  --quota-medium-color: #e0aa14;
  --warning-color: #c65746;
  --error-color: #c65746;
  --viz-success: #10b981;
  --viz-failure: #c65746;
  --badge-bg: #d1fae5;
  --badge-text: #065f46;
  --badge-border: #6ee7b7;
  --card-bg: color-mix(in srgb, var(--bg-primary) 82%, transparent);
}
[data-theme="white"] {
  --bg-secondary: #ffffff;
  --bg-primary: #ffffff;
  --bg-tertiary: #f6f6f6;
  --bg-hover: var(--bg-tertiary);
  --bg-quinary: #ffffff;
  --floating-surface: #ffffff;
  --floating-shadow: 0 12px 26px #0000001f;
  --text-primary: #2d2a26;
  --text-secondary: #6d6760;
  --text-tertiary: #a29c95;
  --text-quaternary: #c0bab3;
  --border-color: #e5e5e5;
  --border-primary: #e0e0e0;
  --border-hover: #d5d5d5;
  --card-bg: #ffffff;
}
[data-theme="dark"] {
  --bg-secondary: #151412;
  --bg-primary: #1d1b18;
  --bg-tertiary: #262320;
  --bg-hover: #2e2a26;
  --bg-quinary: #191714;
  --floating-surface: #2a2723;
  --floating-shadow: 0 14px 30px #0006;
  --text-primary: #f6f4f1;
  --text-secondary: #c9c3bb;
  --text-tertiary: #9c958d;
  --text-quaternary: #746e66;
  --border-color: #332f2b;
  --border-primary: #3d3934;
  --border-hover: #4d4741;
  --success-color: #10b981;
  --quota-medium-color: #e0aa14;
  --warning-color: #e06c5a;
  --error-color: #e06c5a;
  --viz-success: #10b981;
  --viz-failure: #e06c5a;
  --badge-bg: #064e3b;
  --badge-text: #a7f3d0;
  --badge-border: #047857;
  --card-bg: color-mix(in srgb, var(--bg-primary) 82%, transparent);
}

* { box-sizing: border-box; margin: 0; padding: 0; }
body {
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif;
  background: var(--bg-secondary);
  color: var(--text-primary);
  padding: 24px;
  line-height: 1.5;
  transition: background-color 0.2s ease, color 0.2s ease;
}
.container { max-width: 960px; margin: 0 auto; display: flex; flex-direction: column; gap: 20px; }

/* Page Header */
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding-bottom: 4px;
}
.header-left {
  display: flex;
  align-items: center;
  gap: 12px;
}
.header-left h1 {
  font-size: 18px;
  font-weight: 700;
  color: var(--text-primary);
  letter-spacing: -0.01em;
}
.tag-badge {
  font-family: ui-monospace, SF Mono, Cascadia Mono, monospace;
  font-size: 11px;
  font-weight: 700;
  background: var(--badge-bg);
  color: var(--badge-text);
  border: 1px solid var(--badge-border);
  padding: 2px 8px;
  border-radius: 9999px;
  text-transform: uppercase;
}
.header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
.action-btn {
  border: 1px solid var(--border-color);
  background: transparent;
  color: var(--text-secondary);
  font-size: 12px;
  font-weight: 600;
  padding: 5px 12px;
  border-radius: 9999px;
  cursor: pointer;
  text-decoration: none;
  display: inline-flex;
  align-items: center;
  gap: 5px;
  transition: border-color 0.2s, background-color 0.2s, color 0.2s;
}
.action-btn:hover {
  border-color: var(--border-hover);
  background: color-mix(in srgb, var(--bg-tertiary) 55%, transparent);
  color: var(--text-primary);
}

/* Cards Grid */
.cards-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 16px;
}
.cpa-card {
  border: 1px solid var(--border-color);
  background: var(--card-bg);
  border-radius: 14px;
  padding: 16px 18px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
  transition: transform 0.2s ease, border-color 0.2s ease, box-shadow 0.2s ease;
}
.cpa-card:hover {
  border-color: var(--border-hover);
  transform: translateY(-2px);
  box-shadow: 0 8px 20px -8px rgba(0, 0, 0, 0.08);
}
.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.card-title-group {
  display: flex;
  align-items: center;
  gap: 9px;
}
.icon-wrap {
  width: 28px;
  height: 28px;
  border-radius: 8px;
  background: color-mix(in srgb, var(--bg-tertiary) 60%, transparent);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 13px;
  font-weight: 700;
  color: var(--text-secondary);
}
.card-title {
  font-size: 13.5px;
  font-weight: 600;
  color: var(--text-primary);
}
.pct-badge {
  font-family: ui-monospace, SF Mono, Cascadia Mono, monospace;
  font-size: 12px;
  font-weight: 700;
}
.pct-badge.good { color: var(--viz-success); }
.pct-badge.warn { color: var(--quota-medium-color); }
.pct-badge.danger { color: var(--viz-failure); }

.stat-box {
  display: flex;
  align-items: baseline;
  gap: 6px;
}
.stat-main {
  font-family: ui-monospace, SF Mono, Cascadia Mono, monospace;
  font-size: 30px;
  font-weight: 800;
  letter-spacing: -0.02em;
  font-variant-numeric: tabular-nums;
}
.stat-main.good { color: var(--viz-success); }
.stat-main.warn { color: var(--quota-medium-color); }
.stat-main.danger { color: var(--viz-failure); }
.stat-unit {
  font-size: 13px;
  color: var(--text-secondary);
  font-weight: 500;
}

.cpa-bar-track {
  height: 6px;
  border-radius: 9999px;
  background: color-mix(in srgb, var(--text-primary) 8%, transparent);
  overflow: hidden;
}
.cpa-bar-fill {
  height: 100%;
  border-radius: 9999px;
  transition: width 0.4s cubic-bezier(.23, 1, .32, 1);
}
.cpa-bar-fill.good { background: var(--viz-success); }
.cpa-bar-fill.warn { background: var(--quota-medium-color); }
.cpa-bar-fill.danger { background: var(--viz-failure); }

.card-meta-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-family: ui-monospace, SF Mono, Cascadia Mono, monospace;
  font-size: 11.5px;
  color: var(--text-tertiary);
  font-variant-numeric: tabular-nums;
  line-height: 1.4;
}
.card-meta-list strong {
  color: var(--text-secondary);
}

/* Timeline Section */
.timeline-section {
  border: 1px solid var(--border-color);
  background: var(--card-bg);
  border-radius: 14px;
  padding: 20px 22px;
  display: flex;
  flex-direction: column;
  gap: 18px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}
.section-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.section-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
  display: flex;
  align-items: center;
  gap: 8px;
}
.section-badge {
  font-size: 11px;
  font-family: ui-monospace, SF Mono, monospace;
  color: var(--text-tertiary);
  background: color-mix(in srgb, var(--bg-tertiary) 70%, transparent);
  border: 1px solid var(--border-color);
  padding: 2px 8px;
  border-radius: 6px;
}

.timeline-rows {
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.timeline-item {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.timeline-item-meta {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 12.5px;
}
.item-name {
  font-weight: 600;
  color: var(--text-primary);
  display: flex;
  align-items: center;
  gap: 6px;
}
.item-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
}
.item-time {
  font-family: ui-monospace, SF Mono, monospace;
  font-size: 11.5px;
  color: var(--text-tertiary);
}
.item-time strong {
  color: var(--text-secondary);
}

.gantt-bar-shell {
  position: relative;
  height: 24px;
  border-radius: 6px;
  background: color-mix(in srgb, var(--text-primary) 5%, transparent);
  border: 1px solid var(--border-color);
  overflow: hidden;
  display: flex;
  align-items: center;
}
.gantt-bar-active {
  position: absolute;
  top: 0; bottom: 0; left: 0;
  border-radius: 5px;
  transition: width 0.4s ease;
}
.gantt-bar-active.h5 {
  background: color-mix(in srgb, #0ea5e9 20%, transparent);
  border: 1px solid #0ea5e9;
}
.gantt-bar-active.week {
  background: color-mix(in srgb, #6366f1 20%, transparent);
  border: 1px solid #6366f1;
}
.gantt-bar-labels {
  position: relative;
  z-index: 2;
  width: 100%;
  padding: 0 10px;
  display: flex;
  justify-content: space-between;
  font-family: ui-monospace, SF Mono, monospace;
  font-size: 11px;
  font-weight: 600;
  color: var(--text-primary);
}

.footer-info {
  display: flex;
  justify-content: space-between;
  font-size: 11.5px;
  color: var(--text-tertiary);
  padding: 0 4px;
}
</style>
<script>
// Automatically synchronize theme with CPAMC parent document
function syncTheme() {
  try {
    var parentHtml = window.parent.document.documentElement;
    var theme = parentHtml.getAttribute('data-theme') || '';
    if (theme) {
      document.documentElement.setAttribute('data-theme', theme);
    } else {
      document.documentElement.removeAttribute('data-theme');
    }
  } catch(e) {}
}
syncTheme();
try {
  var parentHtml = window.parent.document.documentElement;
  var observer = new MutationObserver(syncTheme);
  observer.observe(parentHtml, { attributes: true, attributeFilter: ['data-theme'] });
} catch(e) {}
</script>
</head>
<body>
<div class="container">
  <div class="page-header">
    <div class="header-left">
      <h1>GLM Coding Plan 配额监控</h1>
      <span class="tag-badge">` + html.EscapeString(v.Level) + `</span>
    </div>
    <div class="header-actions">
      <button type="button" class="action-btn" onclick="location.reload()">↻ 刷新数据</button>
      <a href="?format=json" target="_blank" class="action-btn">{ } JSON</a>
    </div>
  </div>

  <div class="cards-grid">`)

	if v.Window5h != nil {
		renderCPACard(&b, "5 小时配额", "5H", v.Window5h, "积分")
	}
	if v.Weekly != nil {
		renderCPACard(&b, "每周配额", "7D", v.Weekly, "积分")
	}
	if v.MCP != nil {
		renderCPACard(&b, "MCP 月度工具", "MCP", v.MCP, "次")
	}

	b.WriteString(`  </div>

  <div class="timeline-section">
    <div class="section-head">
      <div class="section-title">
        <span>配额调度窗口时间轴</span>
      </div>
      <span class="section-badge">实时滑动窗口</span>
    </div>

    <div class="timeline-rows">`)

	if v.Window5h != nil {
		renderGanttRow(&b, "5 小时滚动周期 (当前窗口)", "#0ea5e9", "h5", v.Window5h)
	}
	if v.Weekly != nil {
		renderGanttRow(&b, "自然周周期 (每周重置)", "#6366f1", "week", v.Weekly)
	}

	b.WriteString(`    </div>
  </div>

  <div class="footer-info">
    <span>同步时刻：` + html.EscapeString(v.FetchedAt) + ` · 60s 内存缓存</span>
    <span>智谱开放平台 / BigModel / Z.ai</span>
  </div>
</div>
</body>
</html>`)

	return []byte(b.String())
}

func renderCPACard(b *strings.Builder, title, iconText string, w *windowView, unit string) {
	statusCls := "good"
	if w.RemainingPct <= 15 {
		statusCls = "danger"
	} else if w.RemainingPct <= 35 {
		statusCls = "warn"
	}

	fmt.Fprintf(b, `
    <article class="cpa-card">
      <header class="card-header">
        <div class="card-title-group">
          <span class="icon-wrap">%s</span>
          <span class="card-title">%s</span>
        </div>
        <span class="pct-badge %s">余 %d%%</span>
      </header>

      <div class="stat-box">
        <span class="stat-main %s">%d</span>
        <span class="stat-unit">%% 剩余可用</span>
      </div>

      <div class="cpa-bar-track">
        <div class="cpa-bar-fill %s" style="width:%d%%;"></div>
      </div>

      <div class="card-meta-list">`,
		html.EscapeString(iconText),
		html.EscapeString(title),
		statusCls,
		w.RemainingPct,
		statusCls,
		w.RemainingPct,
		statusCls,
		w.RemainingPct,
	)

	if w.CurrentCount != nil && w.TotalCount != nil {
		fmt.Fprintf(b, `
        <div>消耗：<strong>%d</strong> / %d %s (剩余 %s)</div>`,
			*w.CurrentCount, *w.TotalCount, unit, formatRemaining(w))
	}

	fmt.Fprintf(b, `
        <div>重置时刻：<strong>%s</strong>（剩余 %s）</div>
      </div>
    </article>`,
		html.EscapeString(w.ResetAt),
		html.EscapeString(humanDuration(w.ResetInSec)),
	)
}

func renderGanttRow(b *strings.Builder, label, dotColor, fillClass string, w *windowView) {
	fmt.Fprintf(b, `
      <div class="timeline-item">
        <div class="timeline-item-meta">
          <span class="item-name">
            <span class="item-dot" style="background:%s;"></span>
            %s
          </span>
          <span class="item-time">
            重置于 <strong>%s</strong>（剩余 %s）
          </span>
        </div>
        <div class="gantt-bar-shell">
          <div class="gantt-bar-active %s" style="width:%d%%;"></div>
          <div class="gantt-bar-labels">
            <span>剩余可用 %d%%</span>
            <span>已消耗 %d%%</span>
          </div>
        </div>
      </div>`,
		dotColor,
		html.EscapeString(label),
		html.EscapeString(w.ResetAt),
		html.EscapeString(humanDuration(w.ResetInSec)),
		fillClass,
		w.RemainingPct,
		w.RemainingPct,
		w.UsedPct,
	)
}

func formatRemaining(w *windowView) string {
	if w.Remaining != nil {
		return fmt.Sprintf("%d", *w.Remaining)
	}
	if w.TotalCount != nil && w.CurrentCount != nil {
		return fmt.Sprintf("%d", *w.TotalCount-*w.CurrentCount)
	}
	return "-"
}

func humanDuration(sec int64) string {
	switch {
	case sec >= 86400:
		days := sec / 86400
		hours := (sec % 86400) / 3600
		return fmt.Sprintf("%d天%d小时", days, hours)
	case sec >= 3600:
		hours := sec / 3600
		mins := (sec % 3600) / 60
		return fmt.Sprintf("%d小时%02d分", hours, mins)
	default:
		mins := sec / 60
		return fmt.Sprintf("%d分钟", mins)
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
