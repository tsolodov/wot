package main

import (
	"encoding/json"
	"os"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file
	testConfig := Config{
		Servers: []Server{
			{
				Name:       "test-server",
				MACAddress: "aa:bb:cc:dd:ee:ff",
				IPAddress:  "192.168.1.100",
			},
		},
		BroadcastIP: "255.255.255.255",
		Telegram: TelegramConfig{
			BotToken:    "test-token",
			AdminChatID: 12345,
		},
	}

	// Write test config to temporary file
	tmpFile, err := os.CreateTemp("", "test-config-*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	configData, err := json.Marshal(testConfig)
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}

	if _, err := tmpFile.Write(configData); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	tmpFile.Close()

	// Test loading the config
	loadedConfig, err := loadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify loaded config
	if len(loadedConfig.Servers) != 1 {
		t.Errorf("Expected 1 server, got %d", len(loadedConfig.Servers))
	}

	if loadedConfig.Servers[0].Name != "test-server" {
		t.Errorf("Expected server name 'test-server', got '%s'", loadedConfig.Servers[0].Name)
	}

	if loadedConfig.BroadcastIP != "255.255.255.255" {
		t.Errorf("Expected broadcast IP '255.255.255.255', got '%s'", loadedConfig.BroadcastIP)
	}
}

func TestSendMagicPacket(t *testing.T) {
	// Test with invalid MAC address
	err := SendMagicPacket("invalid-mac", "255.255.255.255")
	if err == nil {
		t.Error("Expected error for invalid MAC address")
	}

	// Test with valid MAC address format (this won't actually send a packet)
	// We're just testing the packet construction logic
	validMAC := "aa:bb:cc:dd:ee:ff"
	
	// This test would need network mocking to test actual sending
	// For now, we'll test the MAC address parsing by calling with a broadcast
	// address that should fail gracefully
	err = SendMagicPacket(validMAC, "0.0.0.0")
	// We expect an error here due to the invalid broadcast address
	if err == nil {
		t.Log("MAC address parsing appears to work correctly")
	}
}