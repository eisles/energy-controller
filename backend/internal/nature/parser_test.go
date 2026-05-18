package nature

import "testing"

func TestParseInstantaneousPower(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		gridW   int
		importW int
		exportW int
	}{
		{name: "import", value: "00000320", gridW: 800, importW: 800, exportW: 0},
		{name: "export", value: "fffff858", gridW: -1960, importW: 0, exportW: 1960},
		{name: "zero", value: "00000000", gridW: 0, importW: 0, exportW: 0},
		{name: "with prefix", value: "0xfffff830", gridW: -2000, importW: 0, exportW: 2000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseInstantaneousPower(tt.value)
			if err != nil {
				t.Fatalf("ParseInstantaneousPower failed: %v", err)
			}
			if got.GridW != tt.gridW || got.ImportW != tt.importW || got.ExportW != tt.exportW {
				t.Fatalf("ParseInstantaneousPower(%q) = %+v, want grid=%d import=%d export=%d", tt.value, got, tt.gridW, tt.importW, tt.exportW)
			}
		})
	}
}

func TestParseInstantaneousPowerRejectsInvalidHex(t *testing.T) {
	if _, err := ParseInstantaneousPower("not-hex"); err == nil {
		t.Fatal("ParseInstantaneousPower returned nil error for invalid hex")
	}
}
