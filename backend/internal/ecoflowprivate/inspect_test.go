package ecoflowprivate

import "testing"

func TestInspectSnapshotFieldsReturnsPayloadFields(t *testing.T) {
	payload := displayPayload(t, 30, true)
	fields, err := InspectSnapshotFields(payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) == 0 {
		t.Fatal("fields is empty")
	}
	foundReserve := false
	for _, field := range fields {
		if field.CmdFunc == 254 && field.CmdID == 21 && field.Field == 8 && field.Value == "30" {
			foundReserve = true
		}
	}
	if !foundReserve {
		t.Fatalf("fields = %#v, want backup reserve field 8 value 30", fields)
	}
}
