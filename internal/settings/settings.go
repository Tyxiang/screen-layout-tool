// Package settings 管理当前进程固定使用的设置位置。
package settings

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/ini.v1"
)

type Values struct {
	Layout    string
	AutoStart bool
}

func Default() Values { return Values{Layout: "Basic"} }

type Store struct {
	Path string
}

// Open 只选择一次目录；后续保存失败时不切换位置。
func Open(programDir, appData string) (*Store, error) {
	var failures []error
	paths := []string{programDir}
	if appData != "" {
		paths = append(paths, filepath.Join(appData, "screen-layout-tool"))
	}
	for i, directory := range paths {
		if i > 0 {
			if err := os.MkdirAll(directory, 0700); err != nil {
				failures = append(failures, err)
				continue
			}
		}
		if err := accessible(directory); err != nil {
			failures = append(failures, err)
			continue
		}
		return &Store{Path: filepath.Join(directory, "config.ini")}, nil
	}
	return &Store{}, fmt.Errorf("settings cannot be saved: %w", errors.Join(failures...))
}

func accessible(directory string) error {
	file, err := os.OpenFile(filepath.Join(directory, "config.ini"), os.O_RDWR, 0600)
	if err == nil {
		if err = file.Close(); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// 设置通过同目录临时文件替换，目录本身也必须允许写入。
	probe, err := os.CreateTemp(directory, ".settings-probe-*")
	if err != nil {
		return err
	}
	closeErr := probe.Close()
	removeErr := os.Remove(probe.Name())
	return errors.Join(closeErr, removeErr)
}

func (s *Store) Load() (Values, error) {
	values := Default()
	if s.Path == "" {
		return values, nil
	}
	data, err := ini.LoadSources(ini.LoadOptions{Insensitive: true}, s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return values, nil
	}
	if err != nil {
		return values, fmt.Errorf("cannot read settings: %w", err)
	}
	values.Layout = data.Section("Running").Key("Layout").MustString("Basic")
	key := data.Section("Setting").Key("AutoStart")
	if key.String() != "" {
		values.AutoStart, err = key.Bool()
		if err != nil {
			return Default(), fmt.Errorf("invalid AutoStart setting: %w", err)
		}
	}
	return values, nil
}

func (s *Store) Save(values Values) error {
	if s.Path == "" {
		return errors.New("settings cannot be saved")
	}
	data := ini.Empty()
	data.Section("Running").Key("Layout").SetValue(values.Layout)
	data.Section("Setting").Key("AutoStart").SetValue(fmt.Sprint(values.AutoStart))
	temp, err := os.CreateTemp(filepath.Dir(s.Path), ".config-*.ini")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := data.WriteTo(temp); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), s.Path)
}
