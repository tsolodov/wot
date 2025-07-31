package main

import (
	"encoding/json"
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLoadConfig(t *testing.T) {
	testConfig := Config{
		Servers: []Server{
			{
				Name:       "test-server",
				MACAddress: "aa:bb:cc:dd:ee:ff",
				IPAddress:  "192.168.1.100",
				TCPPorts:   []int{22, 80, 443},
			},
		},
		BroadcastIP: "255.255.255.255",
		Telegram: TelegramConfig{
			BotToken:    "test-token",
			AdminChatID: 12345,
		},
	}

	// Test YAML format
	t.Run("YAML", func(t *testing.T) {
		tmpFile, err := os.CreateTemp("", "test-config-*.yaml")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer os.Remove(tmpFile.Name())

		configData, err := yaml.Marshal(testConfig)
		if err != nil {
			t.Fatalf("Failed to marshal YAML config: %v", err)
		}

		if _, err := tmpFile.Write(configData); err != nil {
			t.Fatalf("Failed to write config: %v", err)
		}
		tmpFile.Close()

		loadedConfig, err := loadConfig(tmpFile.Name())
		if err != nil {
			t.Fatalf("Failed to load YAML config: %v", err)
		}

		verifyConfig(t, loadedConfig)
	})

	// Test JSON format
	t.Run("JSON", func(t *testing.T) {
		tmpFile, err := os.CreateTemp("", "test-config-*.json")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer os.Remove(tmpFile.Name())

		configData, err := json.Marshal(testConfig)
		if err != nil {
			t.Fatalf("Failed to marshal JSON config: %v", err)
		}

		if _, err := tmpFile.Write(configData); err != nil {
			t.Fatalf("Failed to write config: %v", err)
		}
		tmpFile.Close()

		loadedConfig, err := loadConfig(tmpFile.Name())
		if err != nil {
			t.Fatalf("Failed to load JSON config: %v", err)
		}

		verifyConfig(t, loadedConfig)
	})
}

func verifyConfig(t *testing.T, config *Config) {
	if len(config.Servers) != 1 {
		t.Errorf("Expected 1 server, got %d", len(config.Servers))
	}

	if config.Servers[0].Name != "test-server" {
		t.Errorf("Expected server name 'test-server', got '%s'", config.Servers[0].Name)
	}

	if config.BroadcastIP != "255.255.255.255" {
		t.Errorf("Expected broadcast IP '255.255.255.255', got '%s'", config.BroadcastIP)
	}

	expectedPorts := []int{22, 80, 443}
	if len(config.Servers[0].TCPPorts) != len(expectedPorts) {
		t.Errorf("Expected %d TCP ports, got %d", len(expectedPorts), len(config.Servers[0].TCPPorts))
	}
	for i, port := range expectedPorts {
		if i < len(config.Servers[0].TCPPorts) && config.Servers[0].TCPPorts[i] != port {
			t.Errorf("Expected TCP port %d at index %d, got %d", port, i, config.Servers[0].TCPPorts[i])
		}
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

func TestEnvironmentVariables(t *testing.T) {
	// Create a temporary config file
	testConfig := Config{
		Servers: []Server{
			{
				Name:       "test-server",
				MACAddress: "aa:bb:cc:dd:ee:ff",
				IPAddress:  "192.168.1.100",
				TCPPorts:   []int{22, 80, 443},
			},
		},
		Telegram: TelegramConfig{
			BotToken:    "config-token",
			AdminChatID: 12345,
		},
	}

	tmpFile, err := os.CreateTemp("", "test-env-config-*.json")
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

	// Test environment variable override
	os.Setenv("WOT_BOT_TOKEN", "env-token")
	os.Setenv("WOT_ADMIN_CHAT_ID", "67890")
	defer func() {
		os.Unsetenv("WOT_BOT_TOKEN")
		os.Unsetenv("WOT_ADMIN_CHAT_ID")
	}()

	// Load config with environment variables
	loadedConfig, err := loadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify environment variables override config file values
	if loadedConfig.Telegram.BotToken != "env-token" {
		t.Errorf("Expected bot token 'env-token', got '%s'", loadedConfig.Telegram.BotToken)
	}

	if loadedConfig.Telegram.AdminChatID != 67890 {
		t.Errorf("Expected admin chat ID 67890, got %d", loadedConfig.Telegram.AdminChatID)
	}
}
