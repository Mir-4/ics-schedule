package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type Lesson struct {
	GroupID    int
	Slot       int
	Start      time.Time
	End        time.Time
	Summary    string
	Teacher    string
	Location   string
	Occurrence int
}

const endpoint = "https://isu.uust.ru/module/schedule/schedule_2024_script.php"

type Client struct {
	httpClient *http.Client
	location   *time.Location
}

func NewClient(location *time.Location) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 20 * time.Second},
		location:   location,
	}
}

func (c *Client) FetchSemester(ctx context.Context, groupID int, weekOneMonday time.Time) ([]Lesson, error) {
	if groupID <= 0 {
		return nil, fmt.Errorf("uust: group ID must be positive")
	}
	form := url.Values{
		"funct":     {"group_semestr"},
		"sem":       {"осенний+семестр"},
		"group_id":  {fmt.Sprint(groupID)},
		"show_temp": {"0"},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("uust: create request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "text/html")
	request.Header.Set("User-Agent", "uust-calendar/0.1")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("uust: fetch group %d semester: %w", groupID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("uust: fetch group %d semester: HTTP %s", groupID, response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil {
		return nil, fmt.Errorf("uust: read group %d semester: %w", groupID, err)
	}
	if len(body) > 4<<20 {
		return nil, fmt.Errorf("uust: group %d semester response exceeds 4 MiB", groupID)
	}
	return ParseSemesterSchedule(groupID, string(body), weekOneMonday, c.location)
}

var (
	timeRangePattern = regexp.MustCompile(`^(\d{2}:\d{2})\s*-\s*(\d{2}:\d{2})$`)
	cellIDPattern    = regexp.MustCompile(`^(\d+)_(\d+)_group$`)
	appendPattern    = regexp.MustCompile(`(?s)\$\('#([^']+)'\)\.append\('((?:\\.|[^'])*)'\);`)
)

func dateTime(date time.Time, value string, location *time.Location) (time.Time, error) {
	return time.ParseInLocation("02.01.2006 15:04", date.Format("02.01.2006")+" "+value, location)
}

func firstElement(node *html.Node, name string) *html.Node {
	if node.Type == html.ElementNode && node.Data == name {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := firstElement(child, name); found != nil {
			return found
		}
	}
	return nil
}

func elements(node *html.Node, name string) []*html.Node {
	var result []*html.Node
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.ElementNode && current.Data == name {
			result = append(result, current)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return result
}

func directCells(row *html.Node) []*html.Node {
	var cells []*html.Node
	for child := row.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.Data == "td" {
			cells = append(cells, child)
		}
	}
	return cells
}

func attribute(node *html.Node, key string) (string, bool) {
	for _, attribute := range node.Attr {
		if attribute.Key == key {
			return attribute.Val, true
		}
	}
	return "", false
}

func text(node *html.Node) string {
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.TextNode {
			builder.WriteString(current.Data)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return builder.String()
}

func clean(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func unescapeJavaScriptString(value string) (string, error) {
	var builder strings.Builder
	for len(value) > 0 {
		character, size := utf8.DecodeRuneInString(value)
		value = value[size:]
		if character != '\\' {
			builder.WriteRune(character)
			continue
		}
		if value == "" {
			return "", errors.New("trailing escape")
		}
		escaped, escapedSize := utf8.DecodeRuneInString(value)
		value = value[escapedSize:]
		switch escaped {
		case '\\', '\'', '"':
			builder.WriteRune(escaped)
		case 'n':
			builder.WriteByte('\n')
		case 'r':
			builder.WriteByte('\r')
		case 't':
			builder.WriteByte('\t')
		default:
			builder.WriteRune(escaped)
		}
	}
	return builder.String(), nil
}

var weeksPattern = regexp.MustCompile(`^Недели:\s*(.+)$`)

type semesterSlot struct {
	number  int
	weekday int
	start   string
	end     string
}

func ParseSemesterSchedule(groupID int, fragment string, weekOneMonday time.Time, location *time.Location) ([]Lesson, error) {
	if location == nil || weekOneMonday.IsZero() || weekOneMonday.Weekday() != time.Monday {
		return nil, errors.New("uust: semester requires a Monday week-one date and location")
	}
	document, err := html.Parse(strings.NewReader(fragment))
	if err != nil {
		return nil, fmt.Errorf("uust: parse semester HTML: %w", err)
	}
	table := firstElement(document, "table")
	if table == nil {
		return nil, errors.New("uust: semester table is missing")
	}
	slots := semesterSlots(table)
	if len(slots) == 0 {
		return nil, errors.New("uust: no semester lesson slots")
	}

	lessons := make([]Lesson, 0)
	occurrences := make(map[string]int)
	for _, match := range appendPattern.FindAllStringSubmatch(fragment, -1) {
		cell, ok := slots[match[1]]
		if !ok {
			if cellIDPattern.MatchString(match[1]) {
				return nil, fmt.Errorf("uust: semester cell %s is missing from table", match[1])
			}
			continue
		}
		body, err := unescapeJavaScriptString(match[2])
		if err != nil {
			return nil, fmt.Errorf("uust: unescape semester cell %s: %w", match[1], err)
		}
		summary, teacher, lessonLocation, weeks, err := semesterDetails(body)
		if err != nil {
			return nil, fmt.Errorf("uust: parse semester cell %s: %w", match[1], err)
		}
		for _, week := range weeks {
			date := weekOneMonday.AddDate(0, 0, (week-1)*7+cell.weekday-1)
			start, err := dateTime(date, cell.start, location)
			if err != nil {
				return nil, err
			}
			end, err := dateTime(date, cell.end, location)
			if err != nil {
				return nil, err
			}
			key := fmt.Sprintf("%d:%s", week, match[1])
			occurrences[key]++
			lessons = append(lessons, Lesson{
				GroupID: groupID, Slot: cell.number, Start: start, End: end, Summary: summary,
				Teacher: teacher, Location: lessonLocation,
				Occurrence: occurrences[key],
			})
		}
	}
	if len(lessons) == 0 {
		return nil, errors.New("uust: no semester lessons found")
	}
	sort.Slice(lessons, func(i, j int) bool {
		if lessons[i].Start.Equal(lessons[j].Start) {
			return lessons[i].Occurrence < lessons[j].Occurrence
		}
		return lessons[i].Start.Before(lessons[j].Start)
	})
	return lessons, nil
}

func semesterSlots(table *html.Node) map[string]semesterSlot {
	slots := make(map[string]semesterSlot)
	for _, row := range elements(table, "tr") {
		cells := directCells(row)
		if len(cells) < 3 {
			continue
		}
		times := timeRangePattern.FindStringSubmatch(clean(text(cells[1])))
		if times == nil {
			continue
		}
		for _, cell := range cells[2:] {
			id, ok := attribute(cell, "id")
			if !ok {
				continue
			}
			parts := cellIDPattern.FindStringSubmatch(id)
			if parts == nil {
				continue
			}
			number, _ := strconv.Atoi(parts[1])
			weekday, _ := strconv.Atoi(parts[2])
			if weekday < 1 || weekday > 7 {
				continue
			}
			slots[id] = semesterSlot{number: number, weekday: weekday, start: times[1], end: times[2]}
		}
	}
	return slots
}

func semesterDetails(fragment string) (string, string, string, []int, error) {
	nodes, err := html.ParseFragment(strings.NewReader(fragment), &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div})
	if err != nil {
		return "", "", "", nil, err
	}
	var lines []string
	var part strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "br" {
			lines = append(lines, clean(part.String()))
			part.Reset()
			return
		}
		if node.Type == html.TextNode {
			part.WriteString(node.Data)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	for _, node := range nodes {
		walk(node)
	}
	lines = append(lines, clean(part.String()))
	if len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
	}
	if len(lines) != 4 || lines[0] == "" {
		return "", "", "", nil, errors.New("unexpected semester lesson fields")
	}
	match := weeksPattern.FindStringSubmatch(lines[3])
	if match == nil {
		return "", "", "", nil, fmt.Errorf("missing weeks in %q", lines[3])
	}
	weeks, err := parseWeeks(match[1])
	if err != nil {
		return "", "", "", nil, err
	}
	return lines[0], lines[1], strings.TrimSpace(strings.TrimPrefix(lines[2], "-")), weeks, nil
}

func parseWeeks(value string) ([]int, error) {
	var weeks []int
	seen := make(map[int]bool)
	for _, item := range strings.Split(value, ",") {
		bounds := strings.Split(strings.TrimSpace(item), "-")
		if len(bounds) > 2 {
			return nil, fmt.Errorf("uust: invalid semester weeks %q", value)
		}
		first, err := strconv.Atoi(strings.TrimSpace(bounds[0]))
		if err != nil || first < 1 || first > 60 {
			return nil, fmt.Errorf("uust: invalid semester weeks %q", value)
		}
		last := first
		if len(bounds) == 2 {
			last, err = strconv.Atoi(strings.TrimSpace(bounds[1]))
			if err != nil || last < first || last > 60 {
				return nil, fmt.Errorf("uust: invalid semester weeks %q", value)
			}
		}
		for week := first; week <= last; week++ {
			if !seen[week] {
				weeks = append(weeks, week)
				seen[week] = true
			}
		}
	}
	return weeks, nil
}
