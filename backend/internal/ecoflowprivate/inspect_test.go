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

func TestCycleFieldCandidatesFromFieldsReportsUnknownVarintsOnly(t *testing.T) {
	fields := []SnapshotField{
		{MessageIndex: 0, CmdFunc: 254, CmdID: 21, Field: 8, Wire: wireVarint, Value: "30"},
		{MessageIndex: 0, CmdFunc: 254, CmdID: 21, Field: 888, Wire: wireVarint, Value: "57"},
		{MessageIndex: 0, CmdFunc: 254, CmdID: 21, Field: 889, Wire: wireVarint, Value: "1"},
		{MessageIndex: 0, CmdFunc: 254, CmdID: 21, Field: 890, Wire: wireBytes, Value: "bytes:2"},
		{MessageIndex: 1, CmdFunc: 32, CmdID: 50, Field: 6, Wire: wireVarint, Value: "66"},
		{MessageIndex: 1, CmdFunc: 32, CmdID: 50, Field: 37, Wire: wireVarint, Value: "262"},
	}
	candidates := CycleFieldCandidatesFromFields(fields)
	if len(candidates) != 1 {
		t.Fatalf("candidates = %#v, want one unknown varint candidate", candidates)
	}
	got := candidates[0]
	if got.CmdFunc != 254 || got.CmdID != 21 || got.Field != 888 || got.Value != 57 {
		t.Fatalf("candidate = %#v, want field 888 value 57", got)
	}
}

func TestCycleFieldCandidatesFromSnapshotInspectsPayload(t *testing.T) {
	display := []byte{}
	display = appendIntField(display, 8, 30)
	display = appendIntField(display, 888, 57)
	payload := encodeHeaderMessage(delta3Header{PData: display, Src: 2, CmdFunc: 254, CmdID: 21, Seq: 1})

	candidates, err := CycleFieldCandidatesFromSnapshot(payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].Field != 888 || candidates[0].Value != 57 {
		t.Fatalf("candidates = %#v, want field 888 value 57", candidates)
	}
}

func TestSummarizeCycleFieldCandidatesSeparatesStableAndChangedValues(t *testing.T) {
	summary := SummarizeCycleFieldCandidates([][]SnapshotField{
		{
			{MessageIndex: 0, CmdFunc: 254, CmdID: 21, Field: 888, Wire: wireVarint, Value: "57"},
			{MessageIndex: 0, CmdFunc: 254, CmdID: 21, Field: 889, Wire: wireVarint, Value: "10"},
		},
		{
			{MessageIndex: 0, CmdFunc: 254, CmdID: 21, Field: 888, Wire: wireVarint, Value: "57"},
			{MessageIndex: 0, CmdFunc: 254, CmdID: 21, Field: 889, Wire: wireVarint, Value: "11"},
		},
	})
	if len(summary) != 2 {
		t.Fatalf("summary = %#v, want two fields", summary)
	}
	stable := findCycleObservation(summary, 888)
	if stable == nil || !stable.Stable || !stable.PresentInAll || len(stable.Values) != 1 || stable.Values[0] != 57 {
		t.Fatalf("stable observation = %#v, want stable value 57 in all samples", stable)
	}
	changed := findCycleObservation(summary, 889)
	if changed == nil || changed.Stable || !changed.PresentInAll || len(changed.Values) != 2 {
		t.Fatalf("changed observation = %#v, want changed values in all samples", changed)
	}
}

func findCycleObservation(observations []CycleFieldCandidateObservation, field int) *CycleFieldCandidateObservation {
	for i := range observations {
		if observations[i].Field == field {
			return &observations[i]
		}
	}
	return nil
}
