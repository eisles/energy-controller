package nature

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

const defaultCloudBaseURL = "https://api.nature.global"

type CloudConfig struct {
	AccessToken string
	ApplianceID string
	BaseURL     string
	HTTPClient  *http.Client
}

type CloudClient struct {
	accessToken string
	applianceID string
	baseURL     string
	httpClient  *http.Client
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
	return &CloudClient{
		accessToken: cfg.AccessToken,
		applianceID: cfg.ApplianceID,
		baseURL:     baseURL,
		httpClient:  httpClient,
	}
}

func (c *CloudClient) CurrentGridPower(ctx context.Context) (domain.GridPower, time.Time, error) {
	if c.accessToken == "" {
		return domain.GridPower{}, time.Time{}, fmt.Errorf("nature access token is empty")
	}
	endpoint, err := url.JoinPath(c.baseURL, "/1/echonetlite/appliances")
	if err != nil {
		return domain.GridPower{}, time.Time{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return domain.GridPower{}, time.Time{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return domain.GridPower{}, time.Time{}, fmt.Errorf("nature cloud request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return domain.GridPower{}, time.Time{}, fmt.Errorf("nature cloud returned HTTP %d", resp.StatusCode)
	}

	var payload cloudAppliancesResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return domain.GridPower{}, time.Time{}, fmt.Errorf("decode nature cloud response: %w", err)
	}
	return selectGridPower(payload.Appliances, c.applianceID)
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
