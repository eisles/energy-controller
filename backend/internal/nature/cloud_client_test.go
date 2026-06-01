package nature

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCloudClientCurrentGridPower(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/1/echonetlite/appliances" {
			t.Fatalf("path = %s, want /1/echonetlite/appliances", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"appliances": [
				{
					"id": "meter-id",
					"type": "EL_SMART_METER",
					"nickname": "smart meter",
					"properties": [
						{"epc": "e7", "val": "fffff858", "updated_at": "2026-05-18T05:59:09Z"}
					]
				}
			]
		}`))
	}))
	defer server.Close()

	client := NewCloudClient(CloudConfig{
		AccessToken: "test-token",
		ApplianceID: "meter-id",
		BaseURL:     server.URL,
	})

	gridPower, updatedAt, err := client.CurrentGridPower(context.Background())
	if err != nil {
		t.Fatalf("CurrentGridPower failed: %v", err)
	}
	if gridPower.GridW != -1960 || gridPower.ExportW != 1960 || gridPower.ImportW != 0 {
		t.Fatalf("gridPower = %+v, want export 1960W", gridPower)
	}
	if updatedAt.IsZero() {
		t.Fatal("updatedAt is zero")
	}
}

func TestCloudClientCurrentEnergyMeterReading(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/1/echonetlite/appliances" {
			t.Fatalf("path = %s, want /1/echonetlite/appliances", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"appliances": [
				{
					"id": "meter-id",
					"type": "EL_SMART_METER",
					"nickname": "smart meter",
					"properties": [
						{"epc": "d3", "val": "00000001", "updated_at": "2026-05-18T05:59:09Z"},
						{"epc": "e1", "val": "01", "updated_at": "2026-05-18T05:59:09Z"},
						{"epc": "e0", "val": "00002710", "updated_at": "2026-05-18T05:59:09Z"},
						{"epc": "e3", "val": "00001388", "updated_at": "2026-05-18T06:00:09Z"}
					]
				}
			]
		}`))
	}))
	defer server.Close()

	client := NewCloudClient(CloudConfig{
		AccessToken: "test-token",
		ApplianceID: "meter-id",
		BaseURL:     server.URL,
	})

	reading, err := client.CurrentEnergyMeterReading(context.Background())
	if err != nil {
		t.Fatalf("CurrentEnergyMeterReading failed: %v", err)
	}
	if reading.ImportCumulativeKWh != 1000 || reading.ExportCumulativeKWh != 500 {
		t.Fatalf("reading = %+v, want import 1000 kWh and export 500 kWh", reading)
	}
	if reading.MeasuredAt.Format("15:04:05") != "06:00:09" {
		t.Fatalf("MeasuredAt = %s, want export updated_at", reading.MeasuredAt)
	}
}

func TestCloudClientReturnsErrorForMissingE7(t *testing.T) {
	_, _, err := selectGridPower([]cloudAppliance{
		{ID: "meter-id", Type: "EL_SMART_METER", Properties: []cloudProperty{{EPC: "e0", Value: "00000000", UpdatedAt: "2026-05-18T05:59:09Z"}}},
	}, "meter-id")
	if err == nil {
		t.Fatal("selectGridPower returned nil error for missing EPC e7")
	}
}

func TestCloudClientReusesAppliancePayloadCache(t *testing.T) {
	var requests int32
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(smartMeterPayload("fffff858")))
	}))
	defer server.Close()

	client := NewCloudClient(CloudConfig{
		AccessToken: "test-token",
		ApplianceID: "meter-id",
		BaseURL:     server.URL,
		CacheTTL:    10 * time.Second,
		Now: func() time.Time {
			return now
		},
	})

	if _, _, err := client.CurrentGridPower(context.Background()); err != nil {
		t.Fatalf("CurrentGridPower failed: %v", err)
	}
	if _, err := client.CurrentEnergyMeterReading(context.Background()); err != nil {
		t.Fatalf("CurrentEnergyMeterReading failed: %v", err)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("requests = %d, want 1 because grid and cumulative reads share cache", got)
	}
}

func TestCloudClientUsesCachedPayloadDuringRateLimit(t *testing.T) {
	var requests int32
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&requests, 1) == 1 {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(smartMeterPayload("fffff858")))
			return
		}
		w.Header().Set("Retry-After", "12")
		http.Error(w, "too many requests", http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := NewCloudClient(CloudConfig{
		AccessToken:          "test-token",
		ApplianceID:          "meter-id",
		BaseURL:              server.URL,
		CacheTTL:             time.Second,
		RateLimitBackoff:     time.Minute,
		RateLimitCacheMaxAge: time.Minute,
		Now: func() time.Time {
			return now
		},
	})

	if _, _, err := client.CurrentGridPower(context.Background()); err != nil {
		t.Fatalf("first CurrentGridPower failed: %v", err)
	}
	now = now.Add(2 * time.Second)
	gridPower, _, err := client.CurrentGridPower(context.Background())
	if err != nil {
		t.Fatalf("rate-limited CurrentGridPower failed: %v", err)
	}
	if gridPower.ExportW != 1960 {
		t.Fatalf("gridPower = %+v, want cached export 1960W", gridPower)
	}
	warning := client.LastGridReadWarning()
	if warning == nil || !strings.Contains(*warning, "rate limited") || !strings.Contains(*warning, "cached") {
		t.Fatalf("warning = %v, want cached rate-limit warning", warning)
	}
	now = now.Add(time.Second)
	if _, _, err := client.CurrentGridPower(context.Background()); err != nil {
		t.Fatalf("backoff CurrentGridPower failed: %v", err)
	}
	if got := atomic.LoadInt32(&requests); got != 2 {
		t.Fatalf("requests = %d, want no extra request during backoff", got)
	}
}

func TestCloudClientReturnsErrorForRateLimitWithoutCache(t *testing.T) {
	var requests int32
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.Header().Set("Retry-After", "12")
		http.Error(w, "too many requests", http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := NewCloudClient(CloudConfig{
		AccessToken: "test-token",
		ApplianceID: "meter-id",
		BaseURL:     server.URL,
		Now: func() time.Time {
			return now
		},
	})

	_, _, err := client.CurrentGridPower(context.Background())
	if err == nil || !strings.Contains(err.Error(), "HTTP 429") {
		t.Fatalf("err = %v, want HTTP 429", err)
	}
	now = now.Add(time.Second)
	_, _, err = client.CurrentGridPower(context.Background())
	if err == nil || !strings.Contains(err.Error(), "rate limited until") {
		t.Fatalf("err = %v, want active backoff without request", err)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("requests = %d, want no retry during active backoff without cache", got)
	}
	if warning := client.LastGridReadWarning(); warning != nil {
		t.Fatalf("warning = %q, want nil when no cached value was used", *warning)
	}
}

func TestCloudClientDoesNotUseExpiredCacheDuringRateLimit(t *testing.T) {
	var requests int32
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&requests, 1) == 1 {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(smartMeterPayload("fffff858")))
			return
		}
		http.Error(w, "too many requests", http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := NewCloudClient(CloudConfig{
		AccessToken:          "test-token",
		ApplianceID:          "meter-id",
		BaseURL:              server.URL,
		CacheTTL:             time.Second,
		RateLimitBackoff:     time.Minute,
		RateLimitCacheMaxAge: time.Minute,
		Now: func() time.Time {
			return now
		},
	})

	if _, _, err := client.CurrentGridPower(context.Background()); err != nil {
		t.Fatalf("first CurrentGridPower failed: %v", err)
	}
	now = now.Add(2 * time.Minute)
	_, _, err := client.CurrentGridPower(context.Background())
	if err == nil || !strings.Contains(err.Error(), "HTTP 429") {
		t.Fatalf("err = %v, want HTTP 429 instead of expired cache fallback", err)
	}
	if warning := client.LastGridReadWarning(); warning != nil {
		t.Fatalf("warning = %q, want nil for expired cache", *warning)
	}
}

func smartMeterPayload(gridHex string) string {
	return `{
		"appliances": [
			{
				"id": "meter-id",
				"type": "EL_SMART_METER",
				"nickname": "smart meter",
				"properties": [
					{"epc": "e7", "val": "` + gridHex + `", "updated_at": "2026-05-18T05:59:09Z"},
					{"epc": "d3", "val": "00000001", "updated_at": "2026-05-18T05:59:09Z"},
					{"epc": "e1", "val": "01", "updated_at": "2026-05-18T05:59:09Z"},
					{"epc": "e0", "val": "00002710", "updated_at": "2026-05-18T05:59:09Z"},
					{"epc": "e3", "val": "00001388", "updated_at": "2026-05-18T06:00:09Z"}
				]
			}
		]
	}`
}
