package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func Render(groupName string, lessons []Lesson, stampedAt time.Time) ([]byte, error) {
	if strings.TrimSpace(groupName) == "" {
		return nil, fmt.Errorf("ical: group name is required")
	}
	ordered := append([]Lesson(nil), lessons...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Start.Equal(ordered[j].Start) {
			if ordered[i].Slot == ordered[j].Slot {
				return ordered[i].Occurrence < ordered[j].Occurrence
			}
			return ordered[i].Slot < ordered[j].Slot
		}
		return ordered[i].Start.Before(ordered[j].Start)
	})

	lines := []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//uust-calendar//Schedule Feed//RU",
		"CALSCALE:GREGORIAN",
		"X-WR-CALNAME:" + escape(groupName),
	}
	stamp := stampedAt.UTC().Format("20060102T150405Z")
	for _, lesson := range ordered {
		if !lesson.End.After(lesson.Start) {
			return nil, fmt.Errorf("ical: %s has an invalid time range", lesson.Summary)
		}
		if strings.TrimSpace(lesson.Summary) == "" {
			return nil, fmt.Errorf("ical: lesson summary is required")
		}
		date := lesson.Start.In(lesson.Start.Location()).Format("20060102")
		uid := fmt.Sprintf("uust-%d-%s-%d-%d@uust-calendar", lesson.GroupID, date, lesson.Slot, lesson.Occurrence)
		lines = append(lines,
			"BEGIN:VEVENT",
			"UID:"+uid,
			"DTSTAMP:"+stamp,
			"DTSTART:"+lesson.Start.UTC().Format("20060102T150405Z"),
			"DTEND:"+lesson.End.UTC().Format("20060102T150405Z"),
			"SUMMARY:"+escape(lesson.Summary),
		)
		if lesson.Teacher != "" {
			lines = append(lines, "DESCRIPTION:"+escape("Преподаватель: "+lesson.Teacher))
		}
		if lesson.Location != "" {
			lines = append(lines, "LOCATION:"+escape(lesson.Location))
		}
		lines = append(lines, "END:VEVENT")
	}
	lines = append(lines, "END:VCALENDAR")

	var calendar strings.Builder
	for _, line := range lines {
		writeFolded(&calendar, line)
	}
	return []byte(calendar.String()), nil
}

func escape(value string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		";", "\\;",
		",", "\\,",
		"\r\n", "\\n",
		"\n", "\\n",
		"\r", "\\n",
	)
	return replacer.Replace(value)
}

func writeFolded(builder *strings.Builder, line string) {
	const limit = 75
	used := 0
	for _, character := range line {
		size := len(string(character))
		if used+size > limit {
			builder.WriteString("\r\n ")
			used = 1
		}
		builder.WriteRune(character)
		used += size
	}
	builder.WriteString("\r\n")
}
