package main

import (
	"os"
	"path/filepath"
	"strings"
)

type Group struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
	ID   int    `json:"id"`
}

func validPROGroup(group Group) bool {
	return strings.HasPrefix(group.Slug, "pro-") && strings.HasPrefix(group.Name, "ПРО-") && group.ID > 0
}

func writeAtomically(path string, data []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".uust-calendar-*")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporary.Name(), path)
}
