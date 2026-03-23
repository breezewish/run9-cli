package config

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Identity caches the last verified remote identity for the current credentials.
type Identity struct {
	AuthKind       string `toml:"auth_kind"`
	UserID         string `toml:"user_id"`
	PrimaryEmail   string `toml:"primary_email"`
	DisplayName    string `toml:"display_name,omitempty"`
	OrgID          string `toml:"org_id"`
	OrgDisplayName string `toml:"org_display_name"`
	OrgKind        string `toml:"org_kind"`
	Role           string `toml:"role"`
}

// File is the persisted CLI login state.
type File struct {
	Endpoint string   `toml:"endpoint"`
	AK       string   `toml:"ak"`
	SK       string   `toml:"sk"`
	Identity Identity `toml:"identity"`
}

// DefaultPath returns the default CLI config path.
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".run9-cli.toml"
	}
	return filepath.Join(home, ".run9", "cli.toml")
}

// Load reads one config file from disk.
func Load(path string) (File, error) {
	var cfg File
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return File{}, err
	}
	return cfg, nil
}

// LoadOptional reads one config file if it exists.
func LoadOptional(path string) (File, bool, error) {
	cfg, err := Load(path)
	if err == nil {
		return cfg, true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return File{}, false, nil
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) && errors.Is(pathErr.Err, os.ErrNotExist) {
		return File{}, false, nil
	}
	return File{}, false, err
}

// Save writes one config file atomically with 0600 permissions.
func Save(path string, cfg File) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".run9-cli-*.toml")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// Remove deletes one config file if it exists.
func Remove(path string) error {
	err := os.Remove(path)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
