//go:build windows

package main

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The unpacked upstream build avoids extracting Python into TEMP for every video.
func resolverPath() string {
	candidates := []string{}
	if data, err := os.ReadFile(filepath.Join(dataDirPath, "resolver-version.txt")); err == nil {
		version := strings.TrimSpace(string(data))
		if validResolverVersion(version) {
			candidates = append(candidates, filepath.Join(dataDirPath, "resolver-updates", version, "yt-dlp.exe"))
		}
	}
	candidates = append(candidates, filepath.Join(dataDirPath, "resolver", "yt-dlp.exe"), bundledPath("resolver/yt-dlp.exe"), bundledPath("yt-dlp.exe"))
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return bundledPath("resolver/yt-dlp.exe")
}

func downloadFile(ctx context.Context, url, path string, limit int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "YTFrame/"+appVersion)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	n, err := io.Copy(f, io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return err
	}
	if n > limit {
		return fmt.Errorf("download exceeds %d bytes", limit)
	}
	return nil
}

// Keep extraction bounded and reject traversal/symlink entries before writing.
func unpackResolver(archive, dest string) error {
	z, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer z.Close()
	if len(z.File) > 4096 {
		return fmt.Errorf("too many files in resolver archive")
	}
	var total uint64
	for _, entry := range z.File {
		name := strings.ReplaceAll(entry.Name, "\\", "/")
		clean := filepath.Clean(filepath.FromSlash(name))
		if strings.HasPrefix(name, "/") || filepath.IsAbs(clean) || filepath.VolumeName(clean) != "" || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || strings.Contains(name, ":") || entry.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("invalid resolver archive path %q", entry.Name)
		}
		if entry.UncompressedSize64 > (256<<20)-total {
			return fmt.Errorf("resolver archive is too large")
		}
		total += entry.UncompressedSize64
	}
	for _, entry := range z.File {
		target := filepath.Join(dest, filepath.FromSlash(strings.ReplaceAll(entry.Name, "\\", "/")))
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		src, err := entry.Open()
		if err != nil {
			return err
		}
		dst, err := os.Create(target)
		if err != nil {
			src.Close()
			return err
		}
		_, copyErr := io.Copy(dst, io.LimitReader(src, int64(entry.UncompressedSize64)+1))
		closeErr := dst.Close()
		src.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	if _, err := os.Stat(filepath.Join(dest, "yt-dlp.exe")); err != nil {
		return fmt.Errorf("resolver executable missing: %w", err)
	}
	return nil
}

func updateResolver() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	stage, err := os.MkdirTemp(dataDirPath, ".resolver-update-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	// Resolve latest once, so the checksum and archive always use the same release.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://github.com/yt-dlp/yt-dlp/releases/latest", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("release lookup returned HTTP %d", resp.StatusCode)
	}
	release := strings.TrimPrefix(resp.Request.URL.Path, "/yt-dlp/yt-dlp/releases/tag/")
	if !validResolverVersion(release) {
		return fmt.Errorf("invalid resolver release")
	}
	base := "https://github.com/yt-dlp/yt-dlp/releases/download/" + release + "/"
	sums, archive := filepath.Join(stage, "sums.txt"), filepath.Join(stage, "resolver.zip")
	if err := downloadFile(ctx, base+"SHA2-256SUMS", sums, 1<<20); err != nil {
		return err
	}
	data, err := os.ReadFile(sums)
	if err != nil {
		return err
	}
	expected := ""
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == "yt-dlp_win.zip" {
			expected = strings.ToLower(fields[0])
			break
		}
	}
	if len(expected) != 64 {
		return fmt.Errorf("release does not contain a resolver checksum")
	}
	if err := downloadFile(ctx, base+"yt-dlp_win.zip", archive, 64<<20); err != nil {
		return err
	}
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	hash := sha256.New()
	_, err = io.Copy(hash, f)
	f.Close()
	if err != nil {
		return err
	}
	if hex.EncodeToString(hash.Sum(nil)) != expected {
		return fmt.Errorf("resolver checksum mismatch")
	}
	fresh := filepath.Join(stage, "resolver")
	if err := unpackResolver(archive, fresh); err != nil {
		return err
	}
	// Install side by side: an active resolver's DLLs are never renamed or deleted.
	versions := filepath.Join(dataDirPath, "resolver-updates")
	if err := os.MkdirAll(versions, 0o755); err != nil {
		return err
	}
	target := filepath.Join(versions, release)
	if _, err := os.Stat(filepath.Join(target, "yt-dlp.exe")); os.IsNotExist(err) {
		if err := os.Rename(fresh, target); err != nil {
			return err
		}
	}
	manifest := filepath.Join(dataDirPath, "resolver-version.txt")
	tmp := manifest + ".tmp"
	if err := os.WriteFile(tmp, []byte(release+"\n"), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, manifest); err != nil {
		return err
	}
	return nil
}

func validResolverVersion(version string) bool {
	if len(version) < 1 || len(version) > 80 || version[0] < '0' || version[0] > '9' {
		return false
	}
	for _, r := range version {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '.' || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}
