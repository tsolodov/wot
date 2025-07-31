package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"gopkg.in/yaml.v3"
)

var (
	config Config
)

type Server struct {
	Name       string `json:"name" yaml:"name"`
	MACAddress string `json:"mac_address" yaml:"mac_address"`
	IPAddress  string `json:"ip_address,omitempty" yaml:"ip_address,omitempty"`
	TCPPorts   []int  `json:"tcp_ports,omitempty" yaml:"tcp_ports,omitempty"`
}

type TelegramConfig struct {
	BotToken    string `json:"bot_token" yaml:"bot_token"`
	AdminChatID int64  `json:"admin_chat_id" yaml:"admin_chat_id"`
}

type Config struct {
	Servers            []Server       `json:"servers" yaml:"servers"`
	Telegram           TelegramConfig `json:"telegram,omitempty" yaml:"telegram,omitempty"`
	BroadcastIP        string         `json:"broadcast_ip,omitempty" yaml:"broadcast_ip,omitempty"`
	MonitoringInterval int            `json:"monitoring_interval,omitempty" yaml:"monitoring_interval,omitempty"`
}

func main() {
	var configFile = flag.String("config", "config.yaml", "Configuration file path")

	flag.Parse()

	config, err := loadConfig(*configFile)
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	runTelegramBot(config)
	return
}

func loadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Try YAML first, then JSON as fallback
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		// Try JSON as fallback
		err = json.Unmarshal(data, &config)
		if err != nil {
			return nil, fmt.Errorf("failed to parse config file as YAML or JSON: %w", err)
		}
	}

	// Override with environment variables if present
	if botToken := os.Getenv("WOT_BOT_TOKEN"); botToken != "" {
		config.Telegram.BotToken = botToken
		log.Println("Using bot token from WOT_BOT_TOKEN environment variable")
	}

	if adminChatID := os.Getenv("WOT_ADMIN_CHAT_ID"); adminChatID != "" {
		if chatID, err := strconv.ParseInt(adminChatID, 10, 64); err == nil {
			config.Telegram.AdminChatID = chatID
			log.Println("Using admin chat ID from WOT_ADMIN_CHAT_ID environment variable")
		} else {
			log.Printf("Warning: Invalid WOT_ADMIN_CHAT_ID format: %s (must be a number)", adminChatID)
		}
	}

	return &config, nil
}

func listAllServers(servers []Server) {
	fmt.Println("Configured servers:")
	for _, server := range servers {
		fmt.Printf("  %s - %s", server.Name, server.MACAddress)
		if server.IPAddress != "" {
			status := checkServerStatus(server)
			statusText := "DOWN"
			if status {
				statusText = "UP"
			}
			fmt.Printf(" (%s) [%s]", server.IPAddress, statusText)
		}
		fmt.Println()
	}
}

func wakeServer(servers []Server, name string) error {
	for _, server := range servers {
		if strings.EqualFold(server.Name, name) {
			return SendMagicPacket(server.MACAddress, config.BroadcastIP)
		}
	}
	return fmt.Errorf("server '%s' not found in configuration", name)
}

func wakeAllServers(servers []Server) error {
	for _, server := range servers {
		err := SendMagicPacket(server.MACAddress, config.BroadcastIP)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to wake %s: %v\n", server.Name, err)
		}
	}
	return nil
}

func SendMagicPacket(macAddr, broadcastIP string) error {

	log.Printf("SendMagicPacket to %s broadcast: %s\n", macAddr, broadcastIP)
	// Parse MAC address
	macBytes, err := hex.DecodeString(strings.ReplaceAll(macAddr, ":", ""))
	if err != nil {
		return fmt.Errorf("invalid MAC address: %w", err)
	}
	if len(macBytes) != 6 {
		return fmt.Errorf("MAC address must be 6 bytes long")
	}

	// Construct magic packet
	magicPacket := make([]byte, 102)
	// First 6 bytes are all 0xFF
	for i := 0; i < 6; i++ {
		magicPacket[i] = 0xFF
	}
	// Subsequent 16 repetitions of the MAC address
	for i := 0; i < 16; i++ {
		copy(magicPacket[6+(i*6):6+(i*6)+6], macBytes)
	}

	// Resolve UDP address
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", broadcastIP, 9)) // Port 9 is commonly used for WoL
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	// Establish UDP connection
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return fmt.Errorf("failed to dial UDP: %w", err)
	}
	defer conn.Close()

	// Send magic packet
	_, err = conn.Write(magicPacket)
	if err != nil {
		return fmt.Errorf("failed to send magic packet: %w", err)
	}

	return nil
}

func checkServerStatus(server Server) bool {
	if server.IPAddress == "" {
		return false
	}

	return pingHost(server.IPAddress, server.TCPPorts)
}

func pingHost(host string, tcpPorts []int) bool {
	// Try privileged ping first
	if pingHostPrivileged(host) {
		return true
	}
	// Fallback to unprivileged ping
	return pingHostUnprivileged(host, tcpPorts)
}

func pingHostPrivileged(host string) bool {
	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		return false
	}
	defer conn.Close()

	dst, err := net.ResolveIPAddr("ip4", host)
	if err != nil {
		return false
	}

	// Create ICMP message
	msg := &icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &icmp.Echo{
			ID:   os.Getpid() & 0xffff,
			Seq:  1,
			Data: []byte("WoT"),
		},
	}

	msgBytes, err := msg.Marshal(nil)
	if err != nil {
		return false
	}

	// Send ping with timeout
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	_, err = conn.WriteTo(msgBytes, dst)
	if err != nil {
		return false
	}

	// Wait for reply
	reply := make([]byte, 1500)
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	n, _, err := conn.ReadFrom(reply)
	if err != nil {
		return false
	}

	// Simple check - if we got any ICMP reply, consider it success
	if n > 8 { // Basic ICMP header is 8 bytes
		return true
	}

	return false
}

func pingHostUnprivileged(host string, tcpPorts []int) bool {
	// Use custom TCP ports or default to common ports
	var ports []int
	if len(tcpPorts) > 0 {
		ports = tcpPorts
	} else {
		ports = []int{22, 80, 443}
	}

	for _, port := range ports {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), 1*time.Second)
		if err == nil {
			conn.Close()
			return true
		}
	}

	return false
}

func checkAllServersStatus(servers []Server) {
	fmt.Println("Server Status:")
	for _, server := range servers {
		if server.IPAddress == "" {
			fmt.Printf("  %s: NO IP ADDRESS\n", server.Name)
			continue
		}

		status := checkServerStatus(server)
		statusText := "DOWN"
		if status {
			statusText = "UP"
		}
		fmt.Printf("  %s (%s): %s\n", server.Name, server.IPAddress, statusText)
	}
}

func checkAndWakeServers(servers []Server, serverName string) error {
	if serverName != "" {
		for _, server := range servers {
			if strings.EqualFold(server.Name, serverName) {
				return checkAndWakeServer(server)
			}
		}
		return fmt.Errorf("server '%s' not found in configuration", serverName)
	}

	for _, server := range servers {
		err := checkAndWakeServer(server)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error with %s: %v\n", server.Name, err)
		}
	}
	return nil
}

func checkAndWakeServer(server Server) error {
	if server.IPAddress == "" {
		fmt.Printf("%s: No IP address configured, sending wake packet\n", server.Name)
		return SendMagicPacket(server.MACAddress, config.BroadcastIP)
	}

	fmt.Printf("Checking %s (%s)... ", server.Name, server.IPAddress)
	if pingHost(server.IPAddress, server.TCPPorts) {
		fmt.Println("UP - no wake needed")
		return nil
	}

	fmt.Println("DOWN - sending wake packet")
	return SendMagicPacket(server.MACAddress, config.BroadcastIP)
}
