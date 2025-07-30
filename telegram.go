package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type ServerState struct {
	Name        string
	IsUp        bool
	LastChecked time.Time
	LastChanged time.Time
	CheckCount  int
}

type ServerMonitor struct {
	states   map[string]*ServerState
	servers  []Server
	bot      *tgbotapi.BotAPI
	config   *Config
	mutex    sync.RWMutex
	interval time.Duration
}

func runTelegramBot(config *Config) {
	bot, err := tgbotapi.NewBotAPI(config.Telegram.BotToken)
	if err != nil {
		log.Fatalf("Failed to create Telegram bot: %v", err)
	}

	bot.Debug = false
	log.Printf("Authorized on account %s", bot.Self.UserName)

	monitor := NewServerMonitor(config.Servers, bot, config)
	monitor.Start()

	if config.Telegram.AdminChatID != 0 {
		uptime := getSystemUptime()
		startupMsg := fmt.Sprintf("🤖 WoT Bot started successfully!\n\n⏱️ System uptime: %s\n🔍 Monitoring %d servers every %v",
			uptime, len(config.Servers), monitor.interval)
		msg := tgbotapi.NewMessage(config.Telegram.AdminChatID, startupMsg)
		bot.Send(msg)
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		handleTelegramMessage(bot, update.Message, config)
	}
}

func handleTelegramMessage(bot *tgbotapi.BotAPI, message *tgbotapi.Message, config *Config) {
	if config.Telegram.AdminChatID != 0 && message.Chat.ID != config.Telegram.AdminChatID {
		log.Println("Unathorized access from:", message.Chat.ID)
		return
	}

	command := strings.ToLower(strings.TrimSpace(message.Text))

	log.Printf("%s command from %s(%s %s)\n", command, message.Chat.UserName, message.Chat.FirstName, message.Chat.LastName)

	switch {
	case command == "/start" || command == "/help":
		handleHelpCommand(bot, message)
	case command == "/list":
		handleListCommand(bot, message, config.Servers)
	case command == "/status":
		handleStatusCommand(bot, message, config.Servers)
	case command == "/uptime":
		handleUptimeCommand(bot, message)
	case strings.HasPrefix(command, "/wake"):
		handleWakeCommand(bot, message, config.Servers, command)
	case strings.HasPrefix(command, "/checkwake"):
		handleCheckWakeCommand(bot, message, config.Servers, command)
	default:
		reply := tgbotapi.NewMessage(message.Chat.ID, "❓ Unknown command. Use /help for available commands.")
		bot.Send(reply)
	}
}

func handleHelpCommand(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	helpText := `🤖 *WoT Bot Commands*

/help - Show this help message
/list - List all servers with status
/status - Check status of all servers
/uptime - Show system uptime
/wake [server] - Wake server(s)
  • /wake - Wake all servers
  • /wake servername - Wake specific server
/checkwake [server] - Check and wake if down
  • /checkwake - Check and wake all down servers
  • /checkwake servername - Check and wake specific server

Examples:
/wake k8s-master
/checkwake rpi`

	msg := tgbotapi.NewMessage(message.Chat.ID, helpText)
	msg.ParseMode = "Markdown"
	bot.Send(msg)
}

func handleListCommand(bot *tgbotapi.BotAPI, message *tgbotapi.Message, servers []Server) {
	if len(servers) == 0 {
		reply := tgbotapi.NewMessage(message.Chat.ID, "📝 No servers configured")
		bot.Send(reply)
		return
	}

	var response strings.Builder
	response.WriteString("🖥️ *Configured Servers:*\n\n")

	for _, server := range servers {
		status := "❌ DOWN"
		if server.IPAddress != "" && checkServerStatus(server) {
			status = "✅ UP"
		} else if server.IPAddress == "" {
			status = "❓ NO IP"
		}

		response.WriteString(fmt.Sprintf("• *%s* - %s\n", server.Name, status))
		if server.IPAddress != "" {
			response.WriteString(fmt.Sprintf("  IP: `%s`\n", server.IPAddress))
		}
		response.WriteString(fmt.Sprintf("  MAC: `%s`\n\n", server.MACAddress))
	}

	msg := tgbotapi.NewMessage(message.Chat.ID, response.String())
	msg.ParseMode = "Markdown"
	bot.Send(msg)
}

func handleStatusCommand(bot *tgbotapi.BotAPI, message *tgbotapi.Message, servers []Server) {
	if len(servers) == 0 {
		reply := tgbotapi.NewMessage(message.Chat.ID, "📝 No servers configured")
		bot.Send(reply)
		return
	}

	var response strings.Builder
	response.WriteString("📊 *Server Status:*\n\n")

	for _, server := range servers {
		if server.IPAddress == "" {
			response.WriteString(fmt.Sprintf("• *%s*: ❓ NO IP ADDRESS\n", server.Name))
			continue
		}

		status := "❌ DOWN"
		if checkServerStatus(server) {
			status = "✅ UP"
		}
		response.WriteString(fmt.Sprintf("• *%s* (%s): %s\n", server.Name, server.IPAddress, status))
	}

	msg := tgbotapi.NewMessage(message.Chat.ID, response.String())
	msg.ParseMode = "Markdown"
	bot.Send(msg)
}

func handleWakeCommand(bot *tgbotapi.BotAPI, message *tgbotapi.Message, servers []Server, command string) {
	parts := strings.Fields(command)

	if len(parts) == 1 {
		var response strings.Builder
		response.WriteString("🌟 *Waking all servers:*\n\n")

		for _, server := range servers {
			err := SendMagicPacket(server.MACAddress, config.BroadcastIP)
			if err != nil {
				response.WriteString(fmt.Sprintf("❌ *%s*: %v\n", server.Name, err))
			} else {
				response.WriteString(fmt.Sprintf("✅ *%s*: Magic packet sent\n", server.Name))
			}
		}

		msg := tgbotapi.NewMessage(message.Chat.ID, response.String())
		msg.ParseMode = "Markdown"
		bot.Send(msg)
		return
	}

	serverName := parts[1]
	for _, server := range servers {
		if strings.EqualFold(server.Name, serverName) {
			err := SendMagicPacket(server.MACAddress, config.BroadcastIP)
			var responseText string
			if err != nil {
				responseText = fmt.Sprintf("❌ Failed to wake *%s*: %v", server.Name, err)
			} else {
				responseText = fmt.Sprintf("✅ Magic packet sent to *%s* (%s)", server.Name, server.MACAddress)
			}

			msg := tgbotapi.NewMessage(message.Chat.ID, responseText)
			msg.ParseMode = "Markdown"
			bot.Send(msg)
			return
		}
	}

	reply := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("❌ Server '%s' not found", serverName))
	bot.Send(reply)
}

func handleUptimeCommand(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	uptime := getSystemUptime()
	responseText := fmt.Sprintf("⏱️ *System Uptime:* %s", uptime)

	msg := tgbotapi.NewMessage(message.Chat.ID, responseText)
	msg.ParseMode = "Markdown"
	bot.Send(msg)
}

func handleCheckWakeCommand(bot *tgbotapi.BotAPI, message *tgbotapi.Message, servers []Server, command string) {
	parts := strings.Fields(command)

	if len(parts) == 1 {
		var response strings.Builder
		response.WriteString("🔍 *Check and Wake Results:*\n\n")

		for _, server := range servers {
			if server.IPAddress == "" {
				err := SendMagicPacket(server.MACAddress, config.BroadcastIP)
				if err != nil {
					response.WriteString(fmt.Sprintf("❌ *%s*: No IP, wake failed - %v\n", server.Name, err))
				} else {
					response.WriteString(fmt.Sprintf("📡 *%s*: No IP, sent wake packet\n", server.Name))
				}
				continue
			}

			if checkServerStatus(server) {
				response.WriteString(fmt.Sprintf("✅ *%s*: Already UP\n", server.Name))
			} else {
				err := SendMagicPacket(server.MACAddress, config.BroadcastIP)
				if err != nil {
					response.WriteString(fmt.Sprintf("❌ *%s*: DOWN, wake failed - %v\n", server.Name, err))
				} else {
					response.WriteString(fmt.Sprintf("🌟 *%s*: DOWN, sent wake packet\n", server.Name))
				}
			}
		}

		msg := tgbotapi.NewMessage(message.Chat.ID, response.String())
		msg.ParseMode = "Markdown"
		bot.Send(msg)
		return
	}

	serverName := parts[1]
	for _, server := range servers {
		if strings.EqualFold(server.Name, serverName) {
			var responseText string

			if server.IPAddress == "" {
				err := SendMagicPacket(server.MACAddress, config.BroadcastIP)
				if err != nil {
					responseText = fmt.Sprintf("❌ *%s*: No IP address, wake failed - %v", server.Name, err)
				} else {
					responseText = fmt.Sprintf("📡 *%s*: No IP address, sent wake packet", server.Name)
				}
			} else if checkServerStatus(server) {
				responseText = fmt.Sprintf("✅ *%s* is already UP", server.Name)
			} else {
				err := SendMagicPacket(server.MACAddress, config.BroadcastIP)
				if err != nil {
					responseText = fmt.Sprintf("❌ *%s* is DOWN, wake failed: %v", server.Name, err)
				} else {
					responseText = fmt.Sprintf("🌟 *%s* was DOWN, sent wake packet", server.Name)
				}
			}

			msg := tgbotapi.NewMessage(message.Chat.ID, responseText)
			msg.ParseMode = "Markdown"
			bot.Send(msg)
			return
		}
	}

	reply := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("❌ Server '%s' not found", serverName))
	bot.Send(reply)
}

func NewServerMonitor(servers []Server, bot *tgbotapi.BotAPI, config *Config) *ServerMonitor {
	interval := 5 * time.Minute
	if config.MonitoringInterval > 0 {
		interval = time.Duration(config.MonitoringInterval) * time.Minute
	}

	monitor := &ServerMonitor{
		states:   make(map[string]*ServerState),
		servers:  servers,
		bot:      bot,
		config:   config,
		interval: interval,
	}

	now := time.Now()
	for _, server := range servers {
		if server.IPAddress == "" {
			continue
		}

		initialState := checkServerStatus(server)
		monitor.states[server.Name] = &ServerState{
			Name:        server.Name,
			IsUp:        initialState,
			LastChecked: now,
			LastChanged: now,
			CheckCount:  1,
		}
	}

	return monitor
}

func (sm *ServerMonitor) Start() {
	log.Printf("Starting server monitoring with %v interval", sm.interval)

	sm.checkAllServers()

	go func() {
		ticker := time.NewTicker(sm.interval)
		defer ticker.Stop()

		for range ticker.C {
			sm.checkAllServers()
		}
	}()
}

func (sm *ServerMonitor) checkAllServers() {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	now := time.Now()

	for _, server := range sm.servers {
		if server.IPAddress == "" {
			continue
		}

		state, exists := sm.states[server.Name]
		if !exists {
			state = &ServerState{
				Name:        server.Name,
				IsUp:        false,
				LastChecked: now,
				LastChanged: now,
				CheckCount:  0,
			}
			sm.states[server.Name] = state
		}

		currentStatus := checkServerStatus(server)
		state.LastChecked = now
		state.CheckCount++

		if currentStatus != state.IsUp {
			log.Printf("Server %s status changed: %v -> %v", server.Name, state.IsUp, currentStatus)

			state.IsUp = currentStatus
			state.LastChanged = now

			sm.sendStatusNotification(server, currentStatus, now)
		}
	}
}

func (sm *ServerMonitor) sendStatusNotification(server Server, isUp bool, timestamp time.Time) {
	if sm.bot == nil || sm.config.Telegram.AdminChatID == 0 {
		return
	}

	var status, emoji string
	if isUp {
		status = "UP"
		emoji = "🟢"
	} else {
		status = "DOWN"
		emoji = "🔴"
	}

	message := fmt.Sprintf("%s *%s* is now *%s*\n\n📍 IP: `%s`\n⏰ Time: %s",
		emoji, server.Name, status, server.IPAddress, timestamp.Format("15:04:05"))

	msg := tgbotapi.NewMessage(sm.config.Telegram.AdminChatID, message)
	msg.ParseMode = "Markdown"

	if _, err := sm.bot.Send(msg); err != nil {
		log.Printf("Failed to send status notification: %v", err)
	}
}

func (sm *ServerMonitor) GetServerStates() map[string]*ServerState {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	states := make(map[string]*ServerState)
	for name, state := range sm.states {
		stateCopy := *state
		states[name] = &stateCopy
	}
	return states
}

func getSystemUptime() string {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return "Unknown"
	}

	uptimeStr := strings.Fields(string(data))[0]
	uptimeSeconds, err := strconv.ParseFloat(uptimeStr, 64)
	if err != nil {
		return "Unknown"
	}

	duration := time.Duration(uptimeSeconds) * time.Second

	days := int(duration.Hours()) / 24
	hours := int(duration.Hours()) % 24
	minutes := int(duration.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	} else if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	} else {
		return fmt.Sprintf("%dm", minutes)
	}
}
