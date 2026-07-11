//go:build mage

package main

import (
	"os"
	"path/filepath"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

var ldFlags = "-s -w"

// Build builds the host binary.
func Build() error {
	return sh.RunV("go", "build", "-trimpath", "-ldflags="+ldFlags, "-o", "bin/dial-up", ".")
}

// BuildArm64 cross-compiles for linux/arm64 (OpenWrt).
func BuildArm64() error {
	return sh.RunWithV(map[string]string{"GOOS": "linux", "GOARCH": "arm64", "CGO_ENABLED": "0"},
		"go", "build", "-trimpath", "-gcflags=all=-l", "-ldflags="+ldFlags, "-o", "bin/dial-up-linux-arm64", ".")
}

// BuildAll builds both amd64 and arm64 binaries.
func BuildAll() error {
	mg.Deps(Build)
	return BuildArm64()
}

// Test runs go test ./....
func Test() error {
	return sh.RunV("go", "test", "./...")
}

// Vet runs go vet ./....
func Vet() error {
	return sh.RunV("go", "vet", "./...")
}

// Upx compresses the arm64 binary with upx (if available).
func Upx() error {
	binPath := "bin/dial-up-linux-arm64"
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		return nil
	}
	// upx may not be installed; skip silently
	_ = sh.RunV("upx", "--best", binPath)
	return nil
}

// Package builds the arm64 binary and creates the OpenWrt tarball.
func Package() error {
	mg.Deps(BuildArm64)

	dir := "bin/dial-up-openwrt-arm64"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	binaries := []struct{ src, dst string }{
		{"bin/dial-up-linux-arm64", "dial-up"},
	}
	for _, b := range binaries {
		if err := copyFile(b.src, filepath.Join(dir, b.dst)); err != nil {
			return err
		}
	}

	deployFiles := []struct{ src, dst string }{
		{"deploy/openwrt/init.d/dial-up", "init.d/dial-up"},
		{"deploy/openwrt/dial-up.env.sample", "dial-up.env.sample"},
	}
	for _, f := range deployFiles {
		dstDir := filepath.Dir(filepath.Join(dir, f.dst))
		if err := os.MkdirAll(dstDir, 0755); err != nil {
			return err
		}
		if err := copyFile(f.src, filepath.Join(dir, f.dst)); err != nil {
			return err
		}
	}

	tarball := "bin/dial-up-openwrt-arm64.tar.gz"
	if err := sh.RunV("tar", "-czf", tarball, "-C", "bin", "dial-up-openwrt-arm64"); err != nil {
		return err
	}

	os.RemoveAll(dir)
	return nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0755)
}

func init() {
	// Ensure bin/ exists
	os.MkdirAll("bin", 0755)
}
