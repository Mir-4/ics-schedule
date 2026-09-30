package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func main() {
	groupsPath := flag.String("groups", "groups.json", "group catalog path")
	outputDir := flag.String("output-dir", "public", "directory where static feeds are published")
	requestInterval := flag.Duration("request-interval", 2*time.Second, "delay between UUST requests")
	maxAttempts := flag.Int("max-attempts", 3, "maximum attempts for one UUST request")
	timezone := flag.String("timezone", "Asia/Yekaterinburg", "timezone for timetable times")
	weekOneMonday := flag.String("week-one-monday", "2026-08-31", "Monday that begins UUST week 1")
	flag.Parse()

	if strings.TrimSpace(*outputDir) == "" || *requestInterval <= 0 || *maxAttempts <= 0 {
		log.Fatal("output directory, request interval, and max attempts must be valid")
	}
	location, err := time.LoadLocation(*timezone)
	if err != nil {
		log.Fatal(err)
	}
	start, err := time.ParseInLocation("2006-01-02", *weekOneMonday, location)
	if err != nil || start.Weekday() != time.Monday {
		log.Fatal("week one must be a Monday in YYYY-MM-DD format")
	}
	groups, err := loadGroups(*groupsPath)
	if err != nil {
		log.Fatal(err)
	}
	client := NewClient(location)
	if err := os.MkdirAll(filepath.Join(*outputDir, "calendar"), 0o755); err != nil {
		log.Fatal(err)
	}
	if err := writeCatalog(*outputDir, groups); err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	firstRequest := true
	for _, group := range groups {
		var lessons []Lesson
		for attempt := 0; attempt < *maxAttempts; attempt++ {
			if !firstRequest {
				select {
				case <-ctx.Done():
					log.Fatal(ctx.Err())
				case <-time.After(*requestInterval):
				}
			}
			firstRequest = false
			lessons, err = client.FetchSemester(ctx, group.ID, start)
			if err == nil {
				break
			}
		}
		if err != nil {
			log.Fatalf("%s: %v", group.Slug, err)
		}
		calendar, err := Render(group.Name, lessons, time.Now())
		if err != nil {
			log.Fatalf("%s: %v", group.Slug, err)
		}
		path := filepath.Join(*outputDir, "calendar", group.Slug+".ics")
		if err := writeAtomically(path, calendar); err != nil {
			log.Fatalf("%s: %v", group.Slug, err)
		}
	}
	log.Printf("published %d group feeds", len(groups))
}

func loadGroups(path string) ([]Group, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open groups: %w", err)
	}
	defer file.Close()
	var groups []Group
	if err := json.NewDecoder(file).Decode(&groups); err != nil {
		return nil, fmt.Errorf("decode groups: %w", err)
	}
	if len(groups) == 0 {
		return nil, fmt.Errorf("at least one group is required")
	}
	seen := make(map[string]bool)
	for _, group := range groups {
		if !validPROGroup(group) {
			return nil, fmt.Errorf("invalid PRO group %+v", group)
		}
		if seen[group.Slug] {
			return nil, fmt.Errorf("duplicate group slug %q", group.Slug)
		}
		seen[group.Slug] = true
	}
	return groups, nil
}
