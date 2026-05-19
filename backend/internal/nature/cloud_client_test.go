package nature

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
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
