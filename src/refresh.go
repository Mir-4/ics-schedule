package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Group struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
	ID   int    `json:"id"`
}

func validPROGroup(group Group) bool {
	return strings.HasPrefix(group.Slug, "pro-") && strings.HasPrefix(group.Name, "ПРО-") && group.ID > 0
}

func publishGroupCalendars(directory string, group Group, lessons []Lesson, stampedAt time.Time) error {
	filtered := make([]Lesson, 0, len(lessons))
	for _, lesson := range lessons {
		if !strings.Contains(strings.ToLower(lesson.Summary), "военная подготовка") {
			filtered = append(filtered, lesson)
		}
	}
	feeds := []struct {
		slug    string
		name    string
		lessons []Lesson
	}{
		{group.Slug, group.Name, lessons},
		{group.Slug + "-no-military-training", group.Name + " без ВУЦа", filtered},
	}
	for _, feed := range feeds {
		calendar, err := Render(feed.name, feed.lessons, stampedAt)
		if err != nil {
			return err
		}
		if err := writeAtomically(filepath.Join(directory, feed.slug+".ics"), calendar); err != nil {
			return err
		}
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
