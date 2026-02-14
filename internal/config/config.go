package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/joho/godotenv"
)

type Config struct {
	Port             int
	DbBridgeKey      string
	SupportedDrivers []string
}

func Load() (*Config, error) {
	// Try loading .env file, but don't fail if it doesn't exist
	_ = godotenv.Load()

	key := os.Getenv("DBBRIDGE_KEY")
	if len(key) < 32 {
		fmt.Println("DBBRIDGE_KEY not found or too short. Generating a new secure key...")
		newKey, err := generateRandomKey(32)
		if err != nil {
			return nil, fmt.Errorf("failed to generate key: %w", err)
		}

		if err := saveKeyToEnv(newKey); err != nil {
			fmt.Printf("Warning: Failed to save generated key to .env: %v\n", err)
		} else {
			fmt.Println("New DBBRIDGE_KEY saved to .env file.")
		}
		key = newKey
	}

	portStr := os.Getenv("PORT")
	port := 8080
	if portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err == nil {
			port = p
		}
	}

	driversStr := os.Getenv("SUPPORTED_DRIVERS")
	var drivers []string
	if driversStr != "" {
		drivers = strings.Split(driversStr, ",")
	} else {
		// Default drivers if not specified
		drivers = []string{"Sql Anywhere 10", "PostgreSQL", "MySQL", "SQLite", "SQL Server"}
	}

	return &Config{
		Port:             port,
		DbBridgeKey:      key,
		SupportedDrivers: drivers,
	}, nil
}

func generateRandomKey(length int) (string, error) {
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	// Return base64 encoded string to ensure it's printable and handles bytes correctly
	return base64.StdEncoding.EncodeToString(b), nil
}

func saveKeyToEnv(key string) error {
	filename := ".env"
	content, err := os.ReadFile(filename)
	if os.IsNotExist(err) {
		// Create new file
		return os.WriteFile(filename, []byte(fmt.Sprintf("DBBRIDGE_KEY=%s\nPORT=8080\n", key)), 0644)
	} else if err != nil {
		return err
	}

	// Handle UTF-16LE BOM or implicit UTF-16LE (lots of nulls)
	sContent := string(content)
	hasBOM := len(content) >= 2 && content[0] == 0xff && content[1] == 0xfe

	// Check for significant nulls (simple heuristic for UTF-16LE without BOM)
	// If more than 30% of bytes are null and we have enough data, assume encoding issue
	nullCount := 0
	if !hasBOM && len(content) > 10 {
		for _, b := range content {
			if b == 0 {
				nullCount++
			}
		}
	}
	isImplicitUTF16 := !hasBOM && len(content) > 0 && (float64(nullCount)/float64(len(content)) > 0.3)

	if hasBOM || isImplicitUTF16 {
		// Convert UTF-16LE to UTF-8
		// If implicit, we assume start at 0. If BOM, start at 2.
		start := 0
		if hasBOM {
			start = 2
		}

		// Ensure even length for decoding (truncate last byte if odd)
		data := content[start:]
		if len(data)%2 != 0 {
			data = data[:len(data)-1]
		}

		u16s := make([]uint16, len(data)/2)
		for i := 0; i < len(u16s); i++ {
			u16s[i] = binary.LittleEndian.Uint16(data[i*2:])
		}
		sContent = string(utf16.Decode(u16s))
	}

	lines := strings.Split(sContent, "\n")
	found := false
	newLines := []string{}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Clean up null bytes if any remain (paranoid check)
		trimmed = strings.Trim(trimmed, "\x00")

		if strings.HasPrefix(trimmed, "DBBRIDGE_KEY=") {
			newLines = append(newLines, fmt.Sprintf("DBBRIDGE_KEY=%s", key))
			found = true
		} else {
			// Careful not to double newline if original had CR
			// We'll just append cleaned lines and Join later.
			// But if line had \r, TrimSpace removed it.
			// So we can just append trimmed? No, that removes indentation or other relevant whitespace?
			// Env files usually key=value. TrimSpace is safe for checking key.
			// But for preserving other lines, we might want to keep original line but cleaned of UTF conversion artifacts.
			// Let's just use trimmed line for reconstruction to be safe and clean.
			// Also remove any remaining nulls from the string just in case
			trimmed = strings.ReplaceAll(trimmed, "\x00", "")
			if trimmed != "" {
				newLines = append(newLines, trimmed)
			}
		}
	}

	if !found {
		newLines = append(newLines, fmt.Sprintf("DBBRIDGE_KEY=%s", key))
	}

	output := strings.Join(newLines, "\n")
	return os.WriteFile(filename, []byte(output), 0644)
}
