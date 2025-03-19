package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/gosnmp/gosnmp"
	_ "github.com/mattn/go-sqlite3"
	"gopkg.in/ini.v1"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
        "flag"
)

// Config struct to hold settings
type Config struct {
	ZabbixToken string
	ZabbixURL   string
	GroupID     int
	Database    string
        Vendors     map[string]string
}

var config *Config

// 🔹 SNMP OIDs
var macTableOID = "1.3.6.1.2.1.17.4.3.1.2"

var interfaceOIDs = map[string]string{
	"Cisco":    "1.3.6.1.2.1.2.2.1.2",
	"Aruba":    "1.3.6.1.2.1.2.2.1.2",
	"ProCurve": "1.3.6.1.2.1.31.1.1.1.1",
}

// Vendor-specific VLAN OIDs
var vlanOIDs = map[string]string{
	"Cisco":    "1.3.6.1.4.1.9.9.68.1.2.2.1.2",
	"Aruba":    "1.3.6.1.2.1.17.7.1.4.5.1.1",
	"ProCurve": "1.3.6.1.2.1.17.7.1.4.5.1.1",
}

func dbpath() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "database.db", err
	}
	dbpath := filepath.Join(filepath.Dir(exePath), config.Database)
	return dbpath, nil
}

func apppath() string {
        exePath, err := os.Executable()
        if err != nil {
                return ""
        }
        return filepath.Dir(exePath)
}

// GetVendor returns the vendor name based on the template ID
func GetVendor(templateID string) string {
	if vendor, exists := config.Vendors[templateID]; exists {
		return vendor
	}
	return "Unknown"
}

// LoadSettings reads settings.ini from the program's directory
func LoadSettings() error {
	// Get the absolute path of the running program
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	configPath := filepath.Join(filepath.Dir(exePath), "settings.ini")

	// Load ini file
	cfg, err := ini.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to read settings.ini: %v", err)
	}

	// Load vendor mappings
	vendorMap := make(map[string]string)
	for _, key := range cfg.Section("Vendors").Keys() {
		vendorMap[key.Name()] = key.String()
	}


	// Parse values from [Global] section
	config = &Config{
		ZabbixToken: cfg.Section("Global").Key("ZabbixToken").String(),
		ZabbixURL:   cfg.Section("Global").Key("ZabbixURL").String(),
		GroupID:     cfg.Section("Global").Key("GroupID").MustInt(0),
		Database:    cfg.Section("Global").Key("Database").String(),
		Vendors:     vendorMap,
	}
	return nil
}

// Append to file
func AppendFile(filename, text string) error {
        f, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
        if err != nil {
               return err
        }
        defer f.Close()
        if _, err = f.WriteString(text); err != nil {
               return err
        }
        return nil
}

// Zabbix API Request Structs
type ZabbixRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
	Auth    string      `json:"auth,omitempty"`
	ID      int         `json:"id"`
}

type ZabbixAuthResponse struct {
	Result string `json:"result"`
}

type ZabbixHostResponse struct {
	Result []struct {
		Host       string `json:"host"`
		Interfaces []struct {
			IP      string `json:"ip"`
			Details struct {
				Community string `json:"community"`
			} `json:"details"`
		} `json:"interfaces"`
		ParentTemplates []struct {
			TemplateID string `json:"templateid"`
		} `json:"parentTemplates"`
	} `json:"result"`
}

// 📌 Zabbix API Request
func zabbixAPIRequest(method string, params interface{}) ([]byte, error) {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
		"auth":    config.ZabbixToken,
		"id":      1,
	}

	return zabbixPost(payload)
}

// 📌 Perform Zabbix API Request
func zabbixPost(payload map[string]interface{}) ([]byte, error) {
	body, _ := json.Marshal(payload)
	resp, err := http.Post(config.ZabbixURL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return ioutil.ReadAll(resp.Body)
}

// 📌 Get Hosts + SNMP Details
func getZabbixHosts() ([]map[string]string, error) {
	params := map[string]interface{}{
		"groupids": config.GroupID,
		"output":   []string{"host"},
		"selectInterfaces": []string{
			"ip", "details",
		},
		"selectParentTemplates": []string{
			"templateid",
		},
	}

	data, err := zabbixAPIRequest("host.get", params)
	if err != nil {
		return nil, err
	}

	var hostsResp struct {
		Result []struct {
			Host       string `json:"host"`
			HostID     string `json:"hostid"`
			Interfaces []struct {
				IP      string `json:"ip"`
				Details struct {
					Community string `json:"community"`
				} `json:"details"`
			} `json:"interfaces"`
			ParentTemplates []struct {
				TemplateID string `json:"templateid"`
			} `json:"parentTemplates"`
		} `json:"result"`
	}

	json.Unmarshal(data, &hostsResp)

	var hosts []map[string]string
	for _, host := range hostsResp.Result {
		if len(host.Interfaces) == 0 {
			continue
		}

		snmpInterface := host.Interfaces[0]
		ip := snmpInterface.IP
		snmpCommunity := snmpInterface.Details.Community

		// Check if SNMP community is a macro (e.g., "{$SNMP_COMMUNITY}")
		if strings.HasPrefix(snmpCommunity, "{$") && strings.HasSuffix(snmpCommunity, "}") {
			realCommunity, err := resolveZabbixMacro(host.HostID, snmpCommunity)
			if err == nil && realCommunity != "" {
				snmpCommunity = realCommunity
			}
		}

		// Detect Vendor
		vendor := "Unknown"
                for _, tpl := range host.ParentTemplates {
                        vnd := GetVendor(tpl.TemplateID)
                        if vnd != "" {
                            vendor = vnd 
                        }
                }


/*
		for _, tpl := range host.ParentTemplates {
			if tpl.TemplateID == "10250" {
				vendor = "ProCurve"
			} else if tpl.TemplateID == "10251" {
				vendor = "Cisco"
			} else if tpl.TemplateID == "10252" {
				vendor = "Aruba"
			}
		}
*/
		hosts = append(hosts, map[string]string{
			"hostname":  host.Host,
			"ip":        ip,
			"community": snmpCommunity,
			"vendor":    vendor,
		})
	}
	return hosts, nil
}

// 📌 Resolve Zabbix Macro to Get Real SNMP Community
func resolveZabbixMacro(hostID, macroName string) (string, error) {
	params := map[string]interface{}{
		"hostids":        hostID,
		"output":         []string{"macro", "value"},
		"globalmacro":    true,
		"templatemacros": true,
	}

	data, err := zabbixAPIRequest("usermacro.get", params)
	if err != nil {
		return "", err
	}

	var macroResp struct {
		Result []struct {
			Macro string `json:"macro"`
			Value string `json:"value"`
		} `json:"result"`
	}

	json.Unmarshal(data, &macroResp)

	for _, macro := range macroResp.Result {
		if macro.Macro == macroName {
			return macro.Value, nil
		}
	}

	return "", fmt.Errorf("❌ Macro not found: %s", macroName)
}

// 📌 3️⃣ Query SNMP for MAC-Port Mapping
func getMACPortData(ip, community, vendor string) []map[string]string {
	var results []map[string]string

	snmp := &gosnmp.GoSNMP{
		Target:    ip,
		Port:      161,
		Community: community,
		Version:   gosnmp.Version2c,
		Timeout:   time.Duration(5) * time.Second,
	}

	err := snmp.Connect()
	if err != nil {
		fmt.Println("❌ SNMP Connection Failed:", err)
		return results
	}
	defer snmp.Conn.Close()

	// Fetch MAC table
	macData, _ := snmp.BulkWalkAll(macTableOID)

	// Fetch Interface table
	ifData, _ := snmp.BulkWalkAll(interfaceOIDs[vendor])

    // Fetch VLAN Mode Table (Access/Trunk)
    vlanModeOID := "1.3.6.1.4.1.9.9.68.1.2.2.1.2" // Cisco OID for Port VLAN Mode
    vlanModeData, _ := snmp.BulkWalkAll(vlanModeOID)


	// Map ifIndex to Port Name
	ifIndexToPort := make(map[string]string)
	for _, pd := range ifData {
		// Extract ifIndex (last number in OID)
		oidParts := strings.Split(pd.Name, ".")
		ifIndex := oidParts[len(oidParts)-1] // Get last part

		switch v := pd.Value.(type) {
		case string:
			ifIndexToPort[ifIndex] = strings.Trim(v, `"`)
		case []uint8:
			ifIndexToPort[ifIndex] = strings.Trim(string(v), `"`)
		case int:
			ifIndexToPort[ifIndex] = fmt.Sprintf("%d", v) // Convert int to string
		default:
			fmt.Printf("⚠️ Unexpected data type for interface name: %T\n", pd.Value)
			continue
		}
	}

    // Map ifIndex to VLAN Mode (1 = Access, 2 = Trunk)
    accessPorts := make(map[string]bool)
    for _, pd := range vlanModeData {
        oidParts := strings.Split(pd.Name, ".")
        ifIndex := oidParts[len(oidParts)-1]

        if v, ok := pd.Value.(int); ok && v == 1 { // 1 = Access mode
            accessPorts[ifIndex] = true
        }
    }

	// Process MAC Addresses (Convert Decimal to Hex)
	for _, pd := range macData {
		oidParts := strings.Split(pd.Name, ".") // Extract OID elements
		if len(oidParts) < 6 {
			fmt.Println("⚠️ Skipping invalid MAC OID:", pd.Name)
			continue
		}

		// Convert last 6 elements from decimal to hex
		macParts := oidParts[len(oidParts)-6:]
		var hexMacParts []string
		for _, part := range macParts {
			decimalValue := 0
			fmt.Sscanf(part, "%d", &decimalValue) // Convert string to int
			hexMacParts = append(hexMacParts, fmt.Sprintf("%02X", decimalValue))
		}
		mac := strings.Join(hexMacParts, ":")

		ifIndex := fmt.Sprintf("%v", pd.Value) // Convert interface index to string
		port, exists := ifIndexToPort[ifIndex]
		fmt.Printf("%s -> %s\n", mac, port)
		if !exists {
			port = "Unknown Port"
		}

        // ❌ Skip trunk ports, keep only access ports
        if _, isAccess := accessPorts[ifIndex]; !isAccess {
            fmt.Printf("⏭️ Skipping trunk port: %s\n", port)
            continue
        }

		results = append(results, map[string]string{
			"mac":  mac,
			"port": port,
		})
	}

	return results
}

// 📌 4️⃣ Store in SQLite
func updateSQLite(hostname, ip, vendor string, data []map[string]string) error {
	dbp, _ := dbpath()
	db, _ := sql.Open("sqlite3", dbp)
	defer db.Close()

	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS network_inventory (
		switch_name TEXT,
		switch_ip TEXT,
		vendor TEXT,
		mac_address TEXT,
		port_name TEXT,
		created_at TEXT DEFAULT CURRENT_TIMESTAMP,
		updated_at TEXT DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(switch_name, mac_address, port_name)
	)`)
        if err != nil {
                   fmt.Printf("Unable to create database schema: %s\n",err)
                   return err
        }
	for _, entry := range data {
		_, err := db.Exec(`INSERT INTO network_inventory (switch_name, switch_ip, vendor, mac_address, port_name, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(switch_name, mac_address, port_name) DO UPDATE SET updated_at = CURRENT_TIMESTAMP`,
			hostname, ip, vendor, entry["mac"], entry["port"])
               if err != nil {
                    fmt.Printf("SQL Error: %s\n",err)
                    return err
               }
	}
        return nil
}

func main() {
	err := LoadSettings()
	if err != nil {
		fmt.Printf("%s\n", err)
		os.Exit(1)
	}
        single := flag.Bool("single",false,"Process single server, specify --host to pass the hostname")
        hst := flag.String("host","","Hostname")
        flag.Parse()
        faillog := filepath.Join(filepath.Dir(apppath()), "fail.log")
	hosts, _ := getZabbixHosts()
	for _, host := range hosts {
                if *single && *hst != host["hostnmame"] {
                       continue
                }
		if host["community"] == "" {
                        AppendFile(faillog,fmt.Sprintf("Host: %s community is empty, its either using not compliant snmp v1/v2 or there are another problems like invalid macros\n", host["hostname"]))
			continue
		}
		if host["vendor"] == "Unknown" {
                        AppendFile(faillog,fmt.Sprintf("Host: %s vendor is unknown, cannot validate it, assign valid templates to the host\n", host["hostname"]))
			continue
		}
		fmt.Println("🔄 Querying:", host["hostname"])
		data := getMACPortData(host["ip"], host["community"], host["vendor"])
                if *single {
                   fmt.Printf("Data: %s\n",data)
                } else {
		   updateSQLite(host["hostname"], host["ip"], host["vendor"], data)
                }
	}
}
