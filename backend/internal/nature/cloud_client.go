package nature

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

const defaultCloudBaseURL = "https://api.nature.global"
const defaultCloudCacheTTL = 10 * time.Second
const defaultCloudRateLimitBackoff = 60 * time.Second
const defaultCloudRateLimitCacheMaxAge = 5 * time.Minute

type CloudConfig struct {
	AccessToken          string
	ApplianceID          string
	BaseURL              string
	HTTPClient           *http.Client
	CacheTTL             time.Duration
	RateLimitBackoff     time.Duration
	RateLimitCacheMaxAge time.Duration
	Now                  func() time.Time
}

type CloudClient struct {
	accessToken          string
	applianceID          string
	baseURL              string
	httpClient           *http.Client
	cacheTTL             time.Duration
	rateLimitBackoff     time.Duration
	rateLimitCacheMaxAge time.Duration
	now                  func() time.Time
	mu                   sync.Mutex
	cachedPayload        *cloudAppliancesResponse
	cachedAt             time.Time
	rateLimitUntil       time.Time
	lastWarning          string
}

func NewCloudClient(cfg CloudConfig) *CloudClient {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultCloudBaseURL
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	cacheTTL := cfg.CacheTTL
	if cacheTTL <= 0 {
		cacheTTL = defaultCloudCacheTTL
	}
	rateLimitBackoff := cfg.RateLimitBackoff
	if rateLimitBackoff <= 0 {
		rateLimitBackoff = defaultCloudRateLimitBackoff
	}
	rateLimitCacheMaxAge := cfg.RateLimitCacheMaxAge
	if rateLimitCacheMaxAge <= 0 {
		rateLimitCacheMaxAge = defaultCloudRateLimitCacheMaxAge
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &CloudClient{
		accessToken:          cfg.AccessToken,
		applianceID:          cfg.ApplianceID,
		baseURL:              baseURL,
		httpClient:           httpClient,
		cacheTTL:             cacheTTL,
		rateLimitBackoff:     rateLimitBackoff,
		rateLimitCacheMaxAge: rateLimitCacheMaxAge,
		now:                  now,
	}
}

func (c *CloudClient) CurrentGridPower(ctx context.Context) (domain.GridPower, time.Time, error) {
	payload, err := c.fetchAppliances(ctx)
	if err != nil {
		return domain.GridPower{}, time.Time{}, err
	}
	return selectGridPower(payload.Appliances, c.applianceID)
}

func (c *CloudClient) CurrentEnergyMeterReading(ctx context.Context) (domain.EnergyMeterReading, error) {
	payload, err := c.fetchAppliances(ctx)
	if err != nil {
		return domain.EnergyMeterReading{}, err
	}
	return selectEnergyMeterReading(payload.Appliances, c.applianceID)
}

func (c *CloudClient) LastGridReadWarning() *string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lastWarning == "" {
		return nil
	}
	warning := c.lastWarning
	return &warning
}

func (c *CloudClient) fetchAppliances(ctx context.Context) (cloudAppliancesResponse, error) {
	if c.accessToken == "" {
		return cloudAppliancesResponse{}, fmt.Errorf("nature access token is empty")
	}
	now := c.now()
	if payload, warning, ok, err := c.rateLimitedCachedAppliances(now); ok {
		if err != nil {
			c.setLastWarning("")
			return cloudAppliancesResponse{}, err
		}
		c.setLastWarning(warning)
		return payload, nil
	}
	if payload, ok := c.cachedAppliances(now); ok {
		c.setLastWarning("")
		return payload, nil
	}
	endpoint, err := url.JoinPath(c.baseURL, "/1/echonetlite/appliances")
	if err != nil {
		return cloudAppliancesResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return cloudAppliancesResponse{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.setLastWarning("")
		return cloudAppliancesResponse{}, fmt.Errorf("nature cloud request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return c.handleRateLimitedResponse(resp, now)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.setLastWarning("")
		return cloudAppliancesResponse{}, fmt.Errorf("nature cloud returned HTTP %d", resp.StatusCode)
	}

	var payload cloudAppliancesResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		c.setLastWarning("")
		return cloudAppliancesResponse{}, fmt.Errorf("decode nature cloud response: %w", err)
	}
	c.storeCachedAppliances(payload, now)
	return payload, nil
}

func (c *CloudClient) cachedAppliances(now time.Time) (cloudAppliancesResponse, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cachedPayload == nil || c.cachedAt.IsZero() {
		return cloudAppliancesResponse{}, false
	}
	if c.cacheTTL <= 0 || now.Sub(c.cachedAt) > c.cacheTTL {
		return cloudAppliancesResponse{}, false
	}
	return *c.cachedPayload, true
}

func (c *CloudClient) rateLimitedCachedAppliances(now time.Time) (cloudAppliancesResponse, string, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rateLimitUntil.IsZero() || !now.Before(c.rateLimitUntil) {
		return cloudAppliancesResponse{}, "", false, nil
	}
	if c.cachedPayload == nil || !c.cachedPayloadValidForRateLimitLocked(now) {
		return cloudAppliancesResponse{}, "", true, fmt.Errorf("nature cloud rate limited until %s and no recent cached value is available", c.rateLimitUntil.Format(time.RFC3339))
	}
	return *c.cachedPayload, c.rateLimitWarningLocked(), true, nil
}

func (c *CloudClient) handleRateLimitedResponse(resp *http.Response, now time.Time) (cloudAppliancesResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	backoff := parseRetryAfter(resp.Header.Get("Retry-After"), now, c.rateLimitBackoff)
	c.rateLimitUntil = now.Add(backoff)
	if c.cachedPayload != nil && c.cachedPayloadValidForRateLimitLocked(now) {
		c.lastWarning = c.rateLimitWarningLocked()
		return *c.cachedPayload, nil
	}
	c.lastWarning = ""
	return cloudAppliancesResponse{}, fmt.Errorf("nature cloud returned HTTP %d; retry after %s", resp.StatusCode, c.rateLimitUntil.Format(time.RFC3339))
}

func (c *CloudClient) storeCachedAppliances(payload cloudAppliancesResponse, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cached := payload
	c.cachedPayload = &cached
	c.cachedAt = now
	c.rateLimitUntil = time.Time{}
	c.lastWarning = ""
}

func (c *CloudClient) setLastWarning(warning string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastWarning = warning
}

func (c *CloudClient) rateLimitWarningLocked() string {
	return fmt.Sprintf("Nature Remo grid power used cached value because nature cloud is rate limited until %s", c.rateLimitUntil.Format(time.RFC3339))
}

func (c *CloudClient) cachedPayloadValidForRateLimitLocked(now time.Time) bool {
	return c.cachedPayload != nil && !c.cachedAt.IsZero() && now.Sub(c.cachedAt) <= c.rateLimitCacheMaxAge
}

func parseRetryAfter(value string, now time.Time, fallback time.Duration) time.Duration {
	if fallback <= 0 {
		fallback = defaultCloudRateLimitBackoff
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds <= 0 {
			return fallback
		}
		return time.Duration(seconds) * time.Second
	}
	retryAt, err := http.ParseTime(value)
	if err != nil {
		return fallback
	}
	if !retryAt.After(now) {
		return fallback
	}
	return retryAt.Sub(now)
}

type cloudAppliancesResponse struct {
	Appliances []cloudAppliance `json:"appliances"`
}

type cloudAppliance struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Nickname   string          `json:"nickname"`
	Properties []cloudProperty `json:"properties"`
}

type cloudProperty struct {
	EPC       string `json:"epc"`
	Value     string `json:"val"`
	UpdatedAt string `json:"updated_at"`
}

func selectGridPower(appliances []cloudAppliance, applianceID string) (domain.GridPower, time.Time, error) {
	appliance, err := selectSmartMeter(appliances, applianceID)
	if err != nil {
		return domain.GridPower{}, time.Time{}, err
	}
	for _, property := range appliance.Properties {
		if strings.EqualFold(property.EPC, "e7") {
			gridPower, err := ParseInstantaneousPower(property.Value)
			if err != nil {
				return domain.GridPower{}, time.Time{}, err
			}
			updatedAt, err := time.Parse(time.RFC3339, property.UpdatedAt)
			if err != nil {
				return domain.GridPower{}, time.Time{}, fmt.Errorf("parse Nature Remo E7 updated_at %q: %w", property.UpdatedAt, err)
			}
			return gridPower, updatedAt, nil
		}
	}
	return domain.GridPower{}, time.Time{}, fmt.Errorf("Nature Remo appliance %s does not include EPC e7", appliance.ID)
}

func selectEnergyMeterReading(appliances []cloudAppliance, applianceID string) (domain.EnergyMeterReading, error) {
	appliance, err := selectSmartMeter(appliances, applianceID)
	if err != nil {
		return domain.EnergyMeterReading{}, err
	}
	properties := map[string]cloudProperty{}
	for _, property := range appliance.Properties {
		properties[strings.ToLower(property.EPC)] = property
	}
	coefficient, ok := properties["d3"]
	if !ok {
		return domain.EnergyMeterReading{}, fmt.Errorf("Nature Remo appliance %s does not include EPC d3", appliance.ID)
	}
	unit, ok := properties["e1"]
	if !ok {
		return domain.EnergyMeterReading{}, fmt.Errorf("Nature Remo appliance %s does not include EPC e1", appliance.ID)
	}
	importProperty, ok := properties["e0"]
	if !ok {
		return domain.EnergyMeterReading{}, fmt.Errorf("Nature Remo appliance %s does not include EPC e0", appliance.ID)
	}
	exportProperty, ok := properties["e3"]
	if !ok {
		return domain.EnergyMeterReading{}, fmt.Errorf("Nature Remo appliance %s does not include EPC e3", appliance.ID)
	}
	importKWh, parsedCoefficient, parsedUnit, err := ParseCumulativeEnergyKWh(importProperty.Value, coefficient.Value, unit.Value)
	if err != nil {
		return domain.EnergyMeterReading{}, err
	}
	exportKWh, _, _, err := ParseCumulativeEnergyKWh(exportProperty.Value, coefficient.Value, unit.Value)
	if err != nil {
		return domain.EnergyMeterReading{}, err
	}
	importUpdatedAt, err := time.Parse(time.RFC3339, importProperty.UpdatedAt)
	if err != nil {
		return domain.EnergyMeterReading{}, fmt.Errorf("parse Nature Remo E0 updated_at %q: %w", importProperty.UpdatedAt, err)
	}
	exportUpdatedAt, err := time.Parse(time.RFC3339, exportProperty.UpdatedAt)
	if err != nil {
		return domain.EnergyMeterReading{}, fmt.Errorf("parse Nature Remo E3 updated_at %q: %w", exportProperty.UpdatedAt, err)
	}
	measuredAt := importUpdatedAt
	if exportUpdatedAt.After(measuredAt) {
		measuredAt = exportUpdatedAt
	}
	return domain.EnergyMeterReading{
		MeasuredAt:           measuredAt,
		ImportCumulativeKWh:  importKWh,
		ExportCumulativeKWh:  exportKWh,
		Coefficient:          parsedCoefficient,
		CumulativeUnit:       parsedUnit,
		RawImportCumulative:  importProperty.Value,
		RawExportCumulative:  exportProperty.Value,
		ImportValueUpdatedAt: importUpdatedAt,
		ExportValueUpdatedAt: exportUpdatedAt,
	}, nil
}

func selectSmartMeter(appliances []cloudAppliance, applianceID string) (cloudAppliance, error) {
	if applianceID != "" {
		for _, appliance := range appliances {
			if appliance.ID == applianceID {
				return appliance, nil
			}
		}
		return cloudAppliance{}, fmt.Errorf("Nature Remo appliance id %s was not found", applianceID)
	}
	for _, appliance := range appliances {
		if appliance.Type == "EL_SMART_METER" {
			return appliance, nil
		}
	}
	return cloudAppliance{}, fmt.Errorf("Nature Remo smart meter was not found")
}
