package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type appPaths struct {
	dataDir  string
	vaultDir string
	dbPath   string
}

func defaultDataDir() (string, error) {
	if runtime.GOOS == "windows" {
		base, err := os.UserCacheDir() // %LOCALAPPDATA%
		if err != nil {
			return "", err
		}
		return filepath.Join(base, "SEFIROT"), nil
	}
	if runtime.GOOS == "darwin" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", "SEFIROT"), nil
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdg != "" {
		if !filepath.IsAbs(xdg) {
			return "", errors.New("XDG_DATA_HOME must be an absolute path")
		}
		return filepath.Join(xdg, "sefirot"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "sefirot"), nil
}

func resolveAppPaths(dataDir, vaultDir, dbPath string) (appPaths, error) {
	if dataDir == "" {
		dataDir = os.Getenv("SEFIROT_DATA_DIR")
	}
	if vaultDir == "" {
		vaultDir = os.Getenv("SEFIROT_VAULT_DIR")
	}
	if dbPath == "" {
		dbPath = os.Getenv("SEFIROT_DB_PATH")
	}

	// Preserve populated pre-1.0 installs that use ./vault + ./index.db.
	// A fresh source archive contains only vault/welcome.html and must use the OS data directory.
	if dataDir == "" && vaultDir == "" && dbPath == "" && hasLegacyWorkingDirectory() {
		dataDir, _ = filepath.Abs(".")
	}
	if dataDir == "" {
		var err error
		dataDir, err = defaultDataDir()
		if err != nil {
			return appPaths{}, err
		}
	}
	absData, err := filepath.Abs(dataDir)
	if err != nil {
		return appPaths{}, err
	}
	if vaultDir == "" {
		vaultDir = filepath.Join(absData, "vault")
	}
	if dbPath == "" {
		dbPath = filepath.Join(absData, "index.db")
	}
	absVault, err := filepath.Abs(vaultDir)
	if err != nil {
		return appPaths{}, err
	}
	absDB, err := filepath.Abs(dbPath)
	if err != nil {
		return appPaths{}, err
	}
	return appPaths{dataDir: absData, vaultDir: absVault, dbPath: absDB}, nil
}

func hasLegacyWorkingDirectory() bool {
	info, err := os.Stat("vault")
	if err != nil || !info.IsDir() {
		return false
	}
	if dbInfo, err := os.Stat("index.db"); err == nil && dbInfo.Mode().IsRegular() {
		return true
	}
	entries, err := os.ReadDir("vault")
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.Name() != "welcome.html" {
			return true
		}
	}
	return false
}
