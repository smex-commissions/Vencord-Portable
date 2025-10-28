package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	path "path/filepath"
	"strings"
)

type PortableOptions struct {
	TargetDir   string
	BrowserPath string
}

type portableConfig struct {
	BrowserPath string `json:"browserPath"`
}

func SetupPortableEnvironment(source *DiscordInstall, opts PortableOptions) (*DiscordInstall, error) {
	opts.TargetDir = strings.TrimSpace(opts.TargetDir)
	opts.BrowserPath = strings.TrimSpace(opts.BrowserPath)

	if opts.TargetDir == "" {
		if err := ensureBaseDirExists(); err != nil {
			return nil, err
		}
		if err := WritePortableConfig(opts.BrowserPath); err != nil {
			return nil, err
		}
		return source, nil
	}

	absTarget, err := path.Abs(opts.TargetDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve portable target path: %w", err)
	}

	absSource, err := path.Abs(source.path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve source path: %w", err)
	}

	if absSource == absTarget {
		if err := ensureBaseDirExists(); err != nil {
			return nil, err
		}
		if err := WritePortableConfig(opts.BrowserPath); err != nil {
			return nil, err
		}
		return source, nil
	}

	if err := cloneDiscordInstall(absSource, absTarget); err != nil {
		return nil, err
	}

	cloned := ParseDiscord(absTarget, source.branch)
	if cloned == nil {
		return nil, fmt.Errorf("failed to parse cloned Discord install at %s", absTarget)
	}

	cloned.branch = source.branch
	cloned.isFlatpak = false

	vencordDataDir := path.Join(absTarget, "VencordData")
	ConfigureBaseDirs(vencordDataDir, path.Join(vencordDataDir, "vencord.asar"))

	if err := ensureBaseDirExists(); err != nil {
		return nil, err
	}
	if err := WritePortableConfig(opts.BrowserPath); err != nil {
		return nil, err
	}

	return cloned, nil
}

func ensureBaseDirExists() error {
	if BaseDir == "" {
		return errors.New("base directory is not configured")
	}
	if err := os.MkdirAll(BaseDir, 0o755); err != nil {
		return fmt.Errorf("failed to create base directory %s: %w", BaseDir, err)
	}
	return nil
}

func WritePortableConfig(browserPath string) error {
	if PortableConfigPath == "" {
		return errors.New("portable config path is not configured")
	}

	if browserPath == "" {
		if err := os.Remove(PortableConfigPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to remove portable config: %w", err)
		}
		return nil
	}

	cfg := portableConfig{BrowserPath: browserPath}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode portable config: %w", err)
	}

	if err := os.MkdirAll(path.Dir(PortableConfigPath), 0o755); err != nil {
		return fmt.Errorf("failed to create portable config directory: %w", err)
	}

	if err := os.WriteFile(PortableConfigPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write portable config: %w", err)
	}

	return nil
}

func cloneDiscordInstall(source, target string) error {
	if info, err := os.Stat(target); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("portable target %s exists and is not a directory", target)
		}
		empty, err := isDirEmpty(target)
		if err != nil {
			return err
		}
		if !empty {
			parsed := ParseDiscord(target, "")
			if parsed != nil {
				return nil
			}
			return fmt.Errorf("portable target %s already exists and is not empty", target)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to inspect portable target: %w", err)
	}

	if err := os.MkdirAll(target, 0o755); err != nil {
		return fmt.Errorf("failed to create portable target %s: %w", target, err)
	}

	return path.WalkDir(source, func(current string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := path.Rel(source, current)
		if err != nil {
			return err
		}

		if rel == "." {
			return nil
		}

		dest := path.Join(target, rel)

		if d.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}

		if d.Type()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(current)
			if err != nil {
				return err
			}
			return os.Symlink(linkTarget, dest)
		}

		return copyFile(current, dest)
	})
}

func isDirEmpty(pathname string) (bool, error) {
	f, err := os.Open(pathname)
	if err != nil {
		return false, fmt.Errorf("failed to open %s: %w", pathname, err)
	}
	defer f.Close()

	_, err = f.Readdirnames(1)
	if errors.Is(err, io.EOF) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to read directory %s: %w", pathname, err)
	}
	return false, nil
}

func copyFile(source, destination string) error {
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("failed to stat %s: %w", source, err)
	}

	if err := os.MkdirAll(path.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	srcFile, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", source, err)
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", destination, err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy %s to %s: %w", source, destination, err)
	}

	return nil
}
