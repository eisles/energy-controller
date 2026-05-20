package ecoflow

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestManualSetSelfPoweredMode(t *testing.T) {
	if os.Getenv("RUN_ECOFLOW_MANUAL_WRITE") != "1" {
		t.Skip("set RUN_ECOFLOW_MANUAL_WRITE=1 to send a real EcoFlow write")
	}
	if os.Getenv("ECOFLOW_MANUAL_SELF_POWERED") != "true" {
		t.Skip("set ECOFLOW_MANUAL_SELF_POWERED=true to confirm self-powered mode write")
	}

	client := NewSignedWriteClient(Config{
		AccessKey: os.Getenv("ECOFLOW_ACCESS_KEY"),
		SecretKey: os.Getenv("ECOFLOW_SECRET_KEY"),
		DeviceSN:  os.Getenv("ECOFLOW_DEVICE_SN"),
		BaseURL:   firstNonEmpty(os.Getenv("ECOFLOW_BASE_URL"), "https://api-e.ecoflow.com"),
	}, WriteGuards{
		MockMode:           false,
		SimulationMode:     false,
		EnableRealControl:  true,
		AutoControlEnabled: false,
		ManualOneShot:      true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.SetSelfPoweredMode(ctx, true); err != nil {
		t.Fatalf("SetSelfPoweredMode failed: %v", err)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
