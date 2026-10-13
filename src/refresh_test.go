package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPublishGroupCalendars(t *testing.T) {
	for _, test := range []struct {
		name      string
		summaries []string
		kept      int
	}{
		{"mixed", []string{"Математика", "Военная подготовка (практика)", "Основы ВОЕННАЯ ПОДГОТОВКА", "Военная история"}, 2},
		{"all filtered", []string{"военная подготовка"}, 0},
		{"no matches", []string{"Математика"}, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			group := Group{Slug: "pro-401b", Name: "ПРО-401Б", ID: 12697}
			start := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
			var lessons []Lesson
			for i, summary := range test.summaries {
				lessons = append(lessons, Lesson{
					GroupID: group.ID, Slot: i + 1, Start: start, End: start.Add(time.Hour),
					Summary: summary, Occurrence: 1,
				})
			}
			if err := publishGroupCalendars(directory, group, lessons, start); err != nil {
				t.Fatal(err)
			}
			read := func(filename string) string {
				t.Helper()
				data, err := os.ReadFile(filepath.Join(directory, filename))
				if err != nil {
					t.Fatal(err)
				}
				return strings.ReplaceAll(string(data), "\r\n ", "")
			}
			full := read(group.Slug + ".ics")
			filtered := read(group.Slug + "-no-military-training.ics")
			if got := strings.Count(full, "BEGIN:VEVENT"); got != len(lessons) {
				t.Fatalf("full feed has %d events, want %d", got, len(lessons))
			}
			if got := strings.Count(filtered, "BEGIN:VEVENT"); got != test.kept {
				t.Fatalf("filtered feed has %d events, want %d", got, test.kept)
			}
			for _, lesson := range lessons {
				present := strings.Contains(filtered, "SUMMARY:"+lesson.Summary+"\r\n")
				want := !strings.Contains(strings.ToLower(lesson.Summary), "военная подготовка")
				if present != want {
					t.Errorf("filtered feed presence of %q = %v, want %v", lesson.Summary, present, want)
				}
			}
			if !strings.HasSuffix(filtered, "END:VCALENDAR\r\n") {
				t.Error("filtered feed is missing calendar ending")
			}
		})
	}
}
