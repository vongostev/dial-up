/*
[2026-07-07] :: 🐛 :: Fixed stray closing-tag typo on RemoveLastProvider (was <|F:SaveLastProvider)
[2026-07-02] :: 🚀 :: Initial state package
*/

package controller

import (
	"encoding/json"
	"fmt"
	"os"

	"dial-up/internal/domain/logger"
	"dial-up/internal/provider"
)

// LoadLastProvider reads a provider from a JSON array file.
func LoadLastProvider(path string) (provider.Provider, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return provider.Provider{}, fmt.Errorf("cannot read %s: %w", path, err)
	}
	var parts [2]string
	if err = json.Unmarshal(data, &parts); err != nil {
		return provider.Provider{}, fmt.Errorf("cannot parse %s: %w", path, err)
	}
	return provider.Provider{Kind: parts[0], RoomID: parts[1]}, nil
}

// SaveLastProvider persists a provider as a JSON array to a file.
func SaveLastProvider(path string, p provider.Provider, l logger.Logger) {
	data, err := json.Marshal([2]string{p.Kind, p.RoomID})
	if err != nil {
		l.Error("controller", "Failed to marshal last provider", logger.Error(err))
		return
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		l.Error("controller", "Failed to write last provider", logger.Error(err))
	}
}

// RemoveLastProvider remove a persisted last provider file.
func RemoveLastProvider(path string, l logger.Logger) {
	if err := os.Remove(path); err != nil {
		l.Error("controller", "Failed to remove last provider persisted", logger.Error(err))
	}
}
