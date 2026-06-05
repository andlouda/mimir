package session

import (
	"os"
	"testing"
)

func setTestConfigHome(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
}

func TestSaveAndLoadSession(t *testing.T) {
	setTestConfigHome(t)

	filePath, err := getSessionFilePath()
	if err != nil {
		t.Fatalf("getSessionFilePath failed: %v", err)
	}

	testData := SessionData{
		Terminals: []TerminalState{
			{Type: "cmd", Name: "Terminal 1", Minimized: false},
			{Type: "bash", Name: "Terminal 2", Minimized: true},
		},
	}

	// Test Save
	err = SaveSession(testData)
	if err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	// Verify file was created
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("Session file not created: %v", err)
	}
	// Verify file permissions (owner read/write only)
	if info.Mode().Perm()&0077 != 0 {
		t.Errorf("Session file permissions too permissive: %v", info.Mode().Perm())
	}

	// Test Load
	loadedData, err := LoadSession()
	if err != nil {
		t.Fatalf("LoadSession failed: %v", err)
	}
	if len(loadedData.Terminals) != 2 {
		t.Fatalf("Expected 2 terminals, got %d", len(loadedData.Terminals))
	}
	if loadedData.Terminals[0].Type != "cmd" {
		t.Errorf("Expected type 'cmd', got '%s'", loadedData.Terminals[0].Type)
	}
	if loadedData.Terminals[0].Name != "Terminal 1" {
		t.Errorf("Expected name 'Terminal 1', got '%s'", loadedData.Terminals[0].Name)
	}
	if loadedData.Terminals[1].Minimized != true {
		t.Errorf("Expected terminal 2 to be minimized")
	}
}

func TestLoadSessionFileNotExists(t *testing.T) {
	setTestConfigHome(t)

	filePath, err := getSessionFilePath()
	if err != nil {
		t.Fatalf("getSessionFilePath failed: %v", err)
	}

	// Remove session file
	os.Remove(filePath)

	data, err := LoadSession()
	if err != nil {
		t.Fatalf("LoadSession with missing file should not error: %v", err)
	}
	if data.Terminals != nil {
		t.Errorf("Expected nil terminals for missing session file, got %v", data.Terminals)
	}
}

func TestLoadSessionCorruptedFile(t *testing.T) {
	setTestConfigHome(t)

	filePath, err := getSessionFilePath()
	if err != nil {
		t.Fatalf("getSessionFilePath failed: %v", err)
	}

	// Write corrupted data
	err = os.WriteFile(filePath, []byte("not valid json{{{"), 0600)
	if err != nil {
		t.Fatalf("Failed to write corrupted file: %v", err)
	}

	_, err = LoadSession()
	if err == nil {
		t.Fatal("LoadSession should fail for corrupted JSON")
	}
}

func TestSaveEmptySession(t *testing.T) {
	setTestConfigHome(t)

	emptyData := SessionData{}
	if err := SaveSession(emptyData); err != nil {
		t.Fatalf("SaveSession with empty data failed: %v", err)
	}

	loaded, err := LoadSession()
	if err != nil {
		t.Fatalf("LoadSession after empty save failed: %v", err)
	}
	if loaded.Terminals != nil {
		t.Errorf("Expected nil terminals for empty session, got %v", loaded.Terminals)
	}
}
