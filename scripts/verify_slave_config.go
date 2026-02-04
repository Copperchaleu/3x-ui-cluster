package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Database models
type Slave struct {
	Id      int    `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
	Port    int    `json:"port"`
	Secret  string `json:"secret"`
	Status  string `json:"status"`
}

type Inbound struct {
	Id             int    `json:"id"`
	SlaveId        int    `json:"slaveId"`
	Tag            string `json:"tag"`
	Listen         string `json:"listen"`
	Port           int    `json:"port"`
	Protocol       string `json:"protocol"`
	Settings       string `json:"settings"`
	StreamSettings string `json:"streamSettings"`
	Sniffing       string `json:"sniffing"`
	Enable         bool   `json:"enable"`
	Remark         string `json:"remark"`
}

type XrayOutbound struct {
	Id             int    `json:"id"`
	SlaveId        int    `json:"slaveId"`
	Tag            string `json:"tag"`
	Protocol       string `json:"protocol"`
	Settings       string `json:"settings"`
	StreamSettings string `json:"streamSettings"`
	Enable         bool   `json:"enable"`
}

type XrayRoutingRule struct {
	Id          int    `json:"id"`
	SlaveId     int    `json:"slaveId"`
	Type        string `json:"type"`
	OutboundTag string `json:"outboundTag"`
	Domain      string `json:"domain"`
	Ip          string `json:"ip"`
	Port        string `json:"port"`
}

// Xray config structures
type XrayConfig struct {
	Inbounds []map[string]interface{} `json:"inbounds"`
	Outbounds []map[string]interface{} `json:"outbounds"`
	Routing map[string]interface{} `json:"routing"`
}

type ConfigDiff struct {
	MissingInbounds []string
	ExtraInbounds   []string
	MismatchedInbounds []string
	MissingOutbounds []string
	ExtraOutbounds   []string
	MismatchedOutbounds []string
	MissingRoutes    []string
	ExtraRoutes      []string
	Details          []string
}

func main() {
	dbPath := flag.String("db", "./db/x-ui.db", "Path to Master database")
	slaveId := flag.Int("slave", 0, "Slave ID to verify (0 for all)")
	containerName := flag.String("container", "", "Docker container name (e.g. slave1, slave2)")
	configPath := flag.String("config", "/app/bin/config.json", "Xray config path in container")
	verbose := flag.Bool("v", false, "Verbose output")
	listSlaves := flag.Bool("list", false, "List all slaves in database")
	flag.Parse()

	// Open database
	db, err := gorm.Open(sqlite.Open(*dbPath), &gorm.Config{})
	if err != nil {
		fmt.Printf("❌ Failed to open database: %v\n", err)
		fmt.Printf("   Database path: %s\n", *dbPath)
		os.Exit(1)
	}

	// List slaves if requested
	if *listSlaves {
		listAllSlaves(db)
		os.Exit(0)
	}

	if *slaveId == 0 {
		fmt.Println("❌ Error: Must specify -slave ID")
		fmt.Println()
		fmt.Println("💡 Tip: Use --list to see all available slaves")
		fmt.Println("   Example: ./scripts/verify_slave_config.sh --list")
		fmt.Println()
		flag.Usage()
		os.Exit(1)
	}

	// Get slave info from database
	var slave Slave
	if err := db.First(&slave, *slaveId).Error; err != nil {
		fmt.Printf("❌ Slave ID %d not found in database\n\n", *slaveId)
		
		// Show available slaves
		var slaves []Slave
		db.Find(&slaves)
		if len(slaves) > 0 {
			fmt.Println("💡 Available slaves:")
			for _, s := range slaves {
				fmt.Printf("   - ID: %d, Name: %s, Status: %s\n", s.Id, s.Name, s.Status)
			}
			fmt.Println("\n   Use --list for more details")
		} else {
			fmt.Println("💡 No slaves configured in database yet")
			fmt.Println("   Add a slave in the web panel first")
		}
		os.Exit(1)
	}

	// Use container name from flag or database
	if *containerName == "" && slave.Name != "" {
		*containerName = slave.Name
	}
	
	if *containerName == "" {
		fmt.Println("❌ Error: Container name not specified")
		fmt.Println()
		fmt.Println("💡 Please specify container name:")
		fmt.Println("   Example: ./scripts/verify_slave_config.sh --slave-id 4 --container slave1")
		os.Exit(1)
	}

	fmt.Printf("📋 Verifying Slave: %s (ID: %d)\n", slave.Name, slave.Id)
	fmt.Printf("   Container: %s\n", *containerName)
	fmt.Printf("   Config Path: %s\n", *configPath)
	fmt.Printf("   Status: %s\n\n", slave.Status)

	// Get expected config from Master database
	fmt.Println("🔍 Reading Master database configuration...")
	expectedConfig, err := buildExpectedConfig(db, slave.Id)
	if err != nil {
		fmt.Printf("❌ Failed to build expected config: %v\n", err)
		os.Exit(1)
	}

	// Get actual config from Slave container
	fmt.Println("🔍 Reading Slave configuration from Docker container...")
	actualConfig, err := readSlaveConfigFromDocker(*containerName, *configPath)
	if err != nil {
		fmt.Printf("❌ Failed to read Slave config: %v\n", err)
		os.Exit(1)
	}

	// Compare configurations
	fmt.Println("🔍 Comparing configurations...\n")
	diff := compareConfigs(expectedConfig, actualConfig, *verbose)

	// Print results
	printResults(diff)

	// Exit with appropriate code
	if hasErrors(diff) {
		os.Exit(1)
	}
	fmt.Println("\n✅ All configurations match!")
}

func listAllSlaves(db *gorm.DB) {
	var slaves []Slave
	if err := db.Find(&slaves).Error; err != nil {
		fmt.Printf("❌ Failed to query slaves: %v\n", err)
		return
	}
	
	if len(slaves) == 0 {
		fmt.Println("ℹ️  No slaves configured in database")
		return
	}
	
	fmt.Println("📋 Available Slaves:")
	fmt.Println(strings.Repeat("=", 90))
	fmt.Printf("  %-4s | %-20s | %-30s | %-10s\n", "ID", "Name", "Address", "Status")
	fmt.Println(strings.Repeat("=", 90))
	for _, s := range slaves {
		addr := s.Address
		if addr == "" {
			addr = "(not configured)"
		}
		fmt.Printf("  %-4d | %-20s | %-30s | %-10s\n", 
			s.Id, s.Name, addr, s.Status)
	}
	fmt.Println(strings.Repeat("=", 90))
	fmt.Printf("\nTotal: %d slave(s)\n", len(slaves))
	fmt.Println("\nUsage:")
	fmt.Println("  ./verify_slave_config.sh --slave-id <ID> --container <container-name>")
}

func buildExpectedConfig(db *gorm.DB, slaveId int) (*XrayConfig, error) {
	config := &XrayConfig{
		Inbounds:  []map[string]interface{}{},
		Outbounds: []map[string]interface{}{},
		Routing:   map[string]interface{}{"rules": []interface{}{}},
	}

	// Get inbounds
	var inbounds []Inbound
	if err := db.Where("slave_id = ? AND enable = ?", slaveId, true).Find(&inbounds).Error; err != nil {
		return nil, err
	}

	for _, inbound := range inbounds {
		inboundConfig := map[string]interface{}{
			"tag":      inbound.Tag,
			"port":     inbound.Port,
			"protocol": inbound.Protocol,
		}
		
		if inbound.Listen != "" {
			inboundConfig["listen"] = inbound.Listen
		}
		
		if inbound.Settings != "" {
			var settings map[string]interface{}
			json.Unmarshal([]byte(inbound.Settings), &settings)
			inboundConfig["settings"] = settings
		}
		
		if inbound.StreamSettings != "" {
			var streamSettings map[string]interface{}
			json.Unmarshal([]byte(inbound.StreamSettings), &streamSettings)
			inboundConfig["streamSettings"] = streamSettings
		}
		
		config.Inbounds = append(config.Inbounds, inboundConfig)
	}

	// Get outbounds
	var outbounds []XrayOutbound
	if err := db.Where("slave_id = ? AND enable = ?", slaveId, true).Find(&outbounds).Error; err != nil {
		return nil, err
	}

	for _, outbound := range outbounds {
		outboundConfig := map[string]interface{}{
			"tag":      outbound.Tag,
			"protocol": outbound.Protocol,
		}
		
		if outbound.Settings != "" {
			var settings map[string]interface{}
			json.Unmarshal([]byte(outbound.Settings), &settings)
			outboundConfig["settings"] = settings
		}
		
		if outbound.StreamSettings != "" {
			var streamSettings map[string]interface{}
			json.Unmarshal([]byte(outbound.StreamSettings), &streamSettings)
			outboundConfig["streamSettings"] = streamSettings
		}
		
		config.Outbounds = append(config.Outbounds, outboundConfig)
	}

	return config, nil
}

func readSlaveConfigFromDocker(containerName, configPath string) (*XrayConfig, error) {
	// Build docker exec command
	cmd := exec.Command("docker", "exec", containerName, "cat", configPath)
	
	// Execute command
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("docker exec error: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("failed to execute docker exec: %v", err)
	}

	// Parse config
	var config XrayConfig
	if err := json.Unmarshal(output, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %v", err)
	}

	return &config, nil
}

func compareConfigs(expected, actual *XrayConfig, verbose bool) ConfigDiff {
	diff := ConfigDiff{}

	// Build tag maps for comparison
	expectedInboundTags := make(map[string]map[string]interface{})
	for _, inbound := range expected.Inbounds {
		if tag, ok := inbound["tag"].(string); ok {
			expectedInboundTags[tag] = inbound
		}
	}

	actualInboundTags := make(map[string]map[string]interface{})
	for _, inbound := range actual.Inbounds {
		if tag, ok := inbound["tag"].(string); ok {
			actualInboundTags[tag] = inbound
		}
	}

	// Check for missing inbounds
	for tag, expected := range expectedInboundTags {
		if actual, exists := actualInboundTags[tag]; !exists {
			diff.MissingInbounds = append(diff.MissingInbounds, tag)
			port := ""
			if p, ok := expected["port"]; ok {
				port = fmt.Sprintf(" Port: %v,", p)
			}
			protocol := ""
			if p, ok := expected["protocol"].(string); ok {
				protocol = fmt.Sprintf(" Protocol: %s", p)
			}
			diff.Details = append(diff.Details, fmt.Sprintf("  ❌ Inbound '%s' (%s%s) not found on Slave", 
				tag, port, protocol))
		} else if verbose {
			// Compare details
			if port, ok := actual["port"].(float64); ok {
				if expectedPort, ok := expected["port"].(int); ok && int(port) != expectedPort {
					diff.MismatchedInbounds = append(diff.MismatchedInbounds, tag)
					diff.Details = append(diff.Details, fmt.Sprintf("  ⚠️  Inbound '%s' port mismatch: expected %d, got %d",
						tag, expectedPort, int(port)))
				}
			}
		}
	}

	// Check for extra inbounds
	for tag := range actualInboundTags {
		if _, exists := expectedInboundTags[tag]; !exists {
			diff.ExtraInbounds = append(diff.ExtraInbounds, tag)
			diff.Details = append(diff.Details, fmt.Sprintf("  ⚠️  Extra inbound '%s' found on Slave (not in Master database)", tag))
		}
	}

	// Similar checks for outbounds
	expectedOutboundTags := make(map[string]map[string]interface{})
	for _, outbound := range expected.Outbounds {
		if tag, ok := outbound["tag"].(string); ok {
			expectedOutboundTags[tag] = outbound
		}
	}

	actualOutboundTags := make(map[string]map[string]interface{})
	for _, outbound := range actual.Outbounds {
		if tag, ok := outbound["tag"].(string); ok {
			actualOutboundTags[tag] = outbound
		}
	}

	for tag := range expectedOutboundTags {
		if _, exists := actualOutboundTags[tag]; !exists {
			diff.MissingOutbounds = append(diff.MissingOutbounds, tag)
			diff.Details = append(diff.Details, fmt.Sprintf("  ❌ Outbound '%s' not found on Slave", tag))
		}
	}

	for tag := range actualOutboundTags {
		if _, exists := expectedOutboundTags[tag]; !exists {
			// Skip built-in outbounds
			if tag != "direct" && tag != "block" && tag != "blackhole" {
				diff.ExtraOutbounds = append(diff.ExtraOutbounds, tag)
				diff.Details = append(diff.Details, fmt.Sprintf("  ⚠️  Extra outbound '%s' found on Slave", tag))
			}
		}
	}

	return diff
}

func printResults(diff ConfigDiff) {
	fmt.Println("=" + strings.Repeat("=", 79))
	fmt.Println("  VERIFICATION RESULTS")
	fmt.Println("=" + strings.Repeat("=", 79))
	
	// Inbounds
	fmt.Printf("\n📥 INBOUNDS:\n")
	if len(diff.MissingInbounds) == 0 && len(diff.ExtraInbounds) == 0 && len(diff.MismatchedInbounds) == 0 {
		fmt.Println("  ✅ All inbounds match")
	} else {
		if len(diff.MissingInbounds) > 0 {
			fmt.Printf("  ❌ Missing on Slave: %d\n", len(diff.MissingInbounds))
		}
		if len(diff.ExtraInbounds) > 0 {
			fmt.Printf("  ⚠️  Extra on Slave: %d\n", len(diff.ExtraInbounds))
		}
		if len(diff.MismatchedInbounds) > 0 {
			fmt.Printf("  ⚠️  Mismatched: %d\n", len(diff.MismatchedInbounds))
		}
	}
	
	// Outbounds
	fmt.Printf("\n📤 OUTBOUNDS:\n")
	if len(diff.MissingOutbounds) == 0 && len(diff.ExtraOutbounds) == 0 && len(diff.MismatchedOutbounds) == 0 {
		fmt.Println("  ✅ All outbounds match")
	} else {
		if len(diff.MissingOutbounds) > 0 {
			fmt.Printf("  ❌ Missing on Slave: %d\n", len(diff.MissingOutbounds))
		}
		if len(diff.ExtraOutbounds) > 0 {
			fmt.Printf("  ⚠️  Extra on Slave: %d\n", len(diff.ExtraOutbounds))
		}
	}
	
	// Details
	if len(diff.Details) > 0 {
		fmt.Println("\n📝 DETAILS:")
		for _, detail := range diff.Details {
			fmt.Println(detail)
		}
	}
}

func hasErrors(diff ConfigDiff) bool {
	return len(diff.MissingInbounds) > 0 || 
		   len(diff.MissingOutbounds) > 0 || 
		   len(diff.MismatchedInbounds) > 0
}
