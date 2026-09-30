package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

func writeCatalog(outputDir string, groups []Group) error {
	ordered := append([]Group(nil), groups...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Name < ordered[j].Name })
	type publicGroup struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	}
	public := make([]publicGroup, len(ordered))
	for i, group := range ordered {
		public[i] = publicGroup{Slug: group.Slug, Name: group.Name}
	}
	data, err := json.MarshalIndent(public, "", "  ")
	if err != nil {
		return fmt.Errorf("encode catalog: %w", err)
	}
	data = append(data, '\n')
	if err := writeAtomically(filepath.Join(outputDir, "groups.json"), data); err != nil {
		return fmt.Errorf("write catalog: %w", err)
	}
	return nil
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
