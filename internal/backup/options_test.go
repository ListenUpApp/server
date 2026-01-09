package backup

import "testing"

func TestRestoreMode_Valid(t *testing.T) {
	tests := []struct {
		mode  RestoreMode
		valid bool
	}{
		{RestoreModeFull, true},
		{RestoreModeMerge, true},
		{RestoreModeEventsOnly, true},
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			if got := tt.mode.Valid(); got != tt.valid {
				t.Errorf("RestoreMode(%q).Valid() = %v, want %v", tt.mode, got, tt.valid)
			}
		})
	}
}

func TestMergeStrategy_Valid(t *testing.T) {
	tests := []struct {
		strategy MergeStrategy
		valid    bool
	}{
		{MergeKeepLocal, true},
		{MergeKeepBackup, true},
		{MergeNewest, true},
		{"", true}, // Empty is valid
		{"invalid", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.strategy), func(t *testing.T) {
			if got := tt.strategy.Valid(); got != tt.valid {
				t.Errorf("MergeStrategy(%q).Valid() = %v, want %v", tt.strategy, got, tt.valid)
			}
		})
	}
}
