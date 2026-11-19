package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	ical "github.com/emersion/go-ical"
	"github.com/teambition/rrule-go"
)

type calendarClient struct {
	feedURL      string
	email        string
	location     *time.Location
	httpClient   *http.Client
	published    map[string]int64
	newPublisher reminderPublisherFactory
}

// calendarDeadline retains calendar metadata while using the existing assignment
// model and eligibility rules. Recurrence identifies the original occurrence.
type calendarDeadline struct {
	assignment
	UID, Description, URL, Recurrence string
	Start, End                        time.Time
	AllDay                            bool
}

func validCalendarEmail(value string) bool {
	address, err := mail.ParseAddress(value)
	_, domain, _ := strings.Cut(value, "@")
	return err == nil && address.Address == value && strings.Contains(domain, ".")
}

func (client *calendarClient) fetch(ctx context.Context) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.feedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create calendar request: invalid URL")
	}
	request.Header.Set("Accept", "text/calendar")
	// Never forward the authenticated query to another origin or downgrade HTTPS.
	httpClient := *client.httpClient
	httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 || req.URL.Scheme != request.URL.Scheme || req.URL.Host != request.URL.Host {
			return fmt.Errorf("calendar redirect rejected")
		}
		return nil
	}
	response, err := httpClient.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// Transport and parser errors may contain the token-bearing URL.
		return nil, fmt.Errorf("calendar request failed (network or redirect error)")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("calendar returned HTTP status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxMoodleResponseSize+1))
	if err != nil {
		return nil, fmt.Errorf("could not read calendar response")
	}
	if len(body) > maxMoodleResponseSize {
		return nil, fmt.Errorf("calendar response exceeds size limit")
	}
	return body, nil
}

func (client *calendarClient) monitor(ctx context.Context, logger *slog.Logger) error {
	body, err := client.fetch(ctx)
	if err != nil {
		return err
	}
	now := time.Now()
	deadlines, skipped, err := parseCalendar(ctx, body, client.location, now)
	if err != nil {
		return err
	}
	if skipped > 0 {
		logger.Warn("calendar entries skipped", "count", skipped)
	}
	for id, due := range client.published {
		if due <= now.Unix() {
			delete(client.published, id)
		}
	}
	source, _ := url.Parse(client.feedURL)
	origin := source.Scheme + "://" + source.Host
	candidates := make([]reminderCandidate, 0, len(deadlines))
	ids := make([]string, 0, len(deadlines))
	seen := make(map[string]bool)
	for _, deadline := range deadlines {
		if !isAssignmentEligible(deadline.assignment, now) {
			continue
		}
		candidate := reminderCandidate{
			CourseID: stableCalendarID("calendar", origin),
			// Include the due time so a revised deadline passes the existing
			// notification service's durable delivered-ID check.
			AssignmentID: stableCalendarID("event", origin, deadline.UID, deadline.Recurrence, strconv.FormatInt(deadline.DueDate, 10)),
			UserID:       stableCalendarID("recipient", client.email), Email: client.email,
			Assignment: client.redact(deadline.Name), AssignmentURL: safeCalendarLink(client.redact(deadline.URL), origin),
			DueDate: time.Unix(deadline.DueDate, 0).UTC(),
		}
		id := reminderMessageID(candidate.CourseID, candidate.AssignmentID, candidate.UserID)
		if client.published[id] != 0 || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
		candidates = append(candidates, candidate)
	}
	published, err := publishReminderCandidates(ctx, client.newPublisher, candidates, logger)
	if client.published == nil {
		client.published = make(map[string]int64)
	}
	for index, id := range ids[:published] {
		client.published[id] = candidates[index].DueDate.Unix()
	}
	logger.Info("calendar cycle completed", "deadlines", len(deadlines), "published_reminders", published, "skipped_entries", skipped, "submission_status", "unavailable")
	return err
}

func stableCalendarID(parts ...string) int64 {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return int64(binary.BigEndian.Uint64(digest[:8])&0x7ffffffffffffffe) + 1
}

func (client *calendarClient) redact(value string) string {
	value = strings.ReplaceAll(value, client.feedURL, "[REDACTED]")
	parsed, _ := url.Parse(client.feedURL)
	for key, values := range parsed.Query() {
		if sensitiveCalendarParameter(key) {
			for _, secret := range values {
				value = redactSecrets(value, secret, url.QueryEscape(secret))
			}
		}
	}
	return value
}

func sensitiveCalendarParameter(key string) bool {
	key = strings.ToLower(key)
	return strings.Contains(key, "token") || strings.Contains(key, "auth") || strings.Contains(key, "password") || strings.Contains(key, "secret")
}

func safeCalendarLink(raw, origin string) string {
	parsed, err := url.Parse(html.UnescapeString(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || strings.Contains(parsed.Path, "export") {
		return origin + "/calendar/view.php"
	}
	for key := range parsed.Query() {
		if sensitiveCalendarParameter(key) {
			return origin + "/calendar/view.php"
		}
	}
	parsed.Fragment = ""
	return parsed.String()
}

var calendarURLPattern = regexp.MustCompile(`https?://[^\s<>"']+`)
var deadlinePattern = regexp.MustCompile(`(?i)(\bis due\b|\bdue date\b|\bdeadline\b|\bdata limite\b|\bdata de entrega\b|\bprazo de entrega\b|\bdeve ser entregue\b|está marcado\(a\) para esta data|^término de\b|^encerramento de\b|\bcloses\b)`)
var openingPattern = regexp.MustCompile(`(?i)^(início de\b|abertura de\b|opens\b|start of\b)`)

func calendarText(event ical.Event, name string) string {
	text, _ := event.Props.Text(name)
	return text
}

func calendarValue(event ical.Event, name string) string {
	if prop := event.Props.Get(name); prop != nil {
		return prop.Value
	}
	return ""
}

// parseCalendar rejects structurally broken feeds and skips invalid individual
// events. No returned error includes untrusted calendar content.
func parseCalendar(ctx context.Context, body []byte, location *time.Location, now time.Time) ([]calendarDeadline, int, error) {
	calendar, err := ical.NewDecoder(strings.NewReader(string(body))).Decode()
	if err != nil {
		return nil, 0, fmt.Errorf("invalid iCalendar feed")
	}
	if prop := calendar.Props.Get("METHOD"); prop != nil && strings.EqualFold(prop.Value, "CANCEL") {
		return nil, 0, nil
	}
	if location == nil {
		location = time.UTC
	}
	if prop := calendar.Props.Get("X-WR-TIMEZONE"); prop != nil {
		location, err = time.LoadLocation(prop.Value)
		if err != nil {
			return nil, 0, fmt.Errorf("unsupported calendar timezone")
		}
	}
	groups := make(map[string]map[string]ical.Event)
	skipped := 0
	for _, event := range calendar.Events() {
		uid := calendarText(event, ical.PropUID)
		if uid == "" {
			skipped++
			continue
		}
		rid := ""
		if prop := event.Props.Get(ical.PropRecurrenceID); prop != nil {
			if prop.Params.Get("RANGE") != "" {
				// THISANDFUTURE needs series-wide rewriting; fail this feed
				// rather than sending stale occurrences from its master.
				return nil, skipped, fmt.Errorf("unsupported calendar recurrence range")
			}
			value, err := prop.DateTime(location)
			if err != nil {
				return nil, skipped, fmt.Errorf("invalid calendar recurrence identity")
			}
			rid = value.UTC().Format(time.RFC3339)
		}
		if groups[uid] == nil {
			groups[uid] = make(map[string]ical.Event)
		}
		previous, ok := groups[uid][rid]
		if !ok || newerCalendarEvent(event, previous) {
			groups[uid][rid] = event
		}
	}
	var deadlines []calendarDeadline
	for _, events := range groups {
		master, hasMaster := events[""]
		if hasMaster && strings.EqualFold(calendarValue(master, ical.PropStatus), "CANCELLED") {
			continue
		}
		if hasMaster {
			deadline, err := extractCalendarDeadline(master, location)
			if err != nil {
				skipped++
			} else if deadline != nil {
				occurrences, err := calendarOccurrences(ctx, master, deadline.Start, now.Add(-deadline.End.Sub(deadline.Start)-24*time.Hour), now.Add(assignmentEligibilityWindow))
				if err != nil {
					skipped++
				} else {
					for _, start := range occurrences {
						rid := start.UTC().Format(time.RFC3339)
						if _, overridden := events[rid]; overridden {
							continue
						}
						item := *deadline
						item.Start = start
						if item.AllDay {
							days := calendarDays(deadline.Start, deadline.End)
							item.End = start.AddDate(0, 0, days)
							item.DueDate = item.End.Add(-time.Second).Unix()
						} else {
							item.End = start.Add(deadline.End.Sub(deadline.Start))
							item.DueDate = item.End.Unix()
						}
						if master.Props.Get(ical.PropRecurrenceRule) != nil || master.Props.Get(ical.PropRecurrenceDates) != nil {
							item.Recurrence = rid
						}
						deadlines = append(deadlines, item)
					}
				}
			}
		}
		for rid, event := range events {
			if rid == "" || strings.EqualFold(calendarValue(event, ical.PropStatus), "CANCELLED") {
				continue
			}
			// Detached instances can omit the master's display fields.
			for _, name := range []string{ical.PropSummary, ical.PropDescription, ical.PropURL} {
				if event.Props.Get(name) == nil && hasMaster {
					event.Props[name] = master.Props[name]
				}
			}
			deadline, err := extractCalendarDeadline(event, location)
			if err != nil {
				skipped++
			} else if deadline != nil {
				if hasMaster && event.Props.Get(ical.PropDateTimeEnd) == nil && event.Props.Get(ical.PropDuration) == nil {
					original, err := extractCalendarDeadline(master, location)
					if err == nil && original != nil {
						if deadline.AllDay {
							deadline.End = deadline.Start.AddDate(0, 0, calendarDays(original.Start, original.End))
							deadline.DueDate = deadline.End.Add(-time.Second).Unix()
						} else {
							deadline.End = deadline.Start.Add(original.End.Sub(original.Start))
							deadline.DueDate = deadline.End.Unix()
						}
					}
				}
				deadline.Recurrence = rid
				deadlines = append(deadlines, *deadline)
			}
		}
	}
	eligible := deadlines[:0]
	for _, deadline := range deadlines {
		if isAssignmentEligible(deadline.assignment, now) {
			eligible = append(eligible, deadline)
		}
	}
	deadlines = eligible
	sort.Slice(deadlines, func(i, j int) bool {
		if deadlines[i].DueDate == deadlines[j].DueDate {
			return deadlines[i].UID < deadlines[j].UID
		}
		return deadlines[i].DueDate < deadlines[j].DueDate
	})
	return deadlines, skipped, nil
}

func newerCalendarEvent(left, right ical.Event) bool {
	a, _ := strconv.Atoi(calendarValue(left, ical.PropSequence))
	b, _ := strconv.Atoi(calendarValue(right, ical.PropSequence))
	if a != b {
		return a > b
	}
	for _, name := range []string{ical.PropLastModified, ical.PropDateTimeStamp} {
		a, _ := left.Props.DateTime(name, time.UTC)
		b, _ := right.Props.DateTime(name, time.UTC)
		if !a.Equal(b) {
			return a.After(b)
		}
	}
	return strings.EqualFold(calendarValue(left, ical.PropStatus), "CANCELLED")
}

func extractCalendarDeadline(event ical.Event, location *time.Location) (*calendarDeadline, error) {
	name := strings.TrimSpace(calendarText(event, ical.PropSummary))
	if name == "" || openingPattern.MatchString(name) || !deadlinePattern.MatchString(name) || strings.EqualFold(calendarValue(event, ical.PropStatus), "CANCELLED") {
		return nil, nil
	}
	start, err := event.DateTimeStart(location)
	if err != nil || start.IsZero() {
		return nil, fmt.Errorf("invalid event start")
	}
	allDay := len(calendarValue(event, ical.PropDateTimeStart)) == 8
	if prop := event.Props.Get(ical.PropDateTimeEnd); prop != nil && (len(prop.Value) == 8) != allDay {
		return nil, fmt.Errorf("event start and end have different value types")
	}
	end, err := event.DateTimeEnd(location)
	if err != nil || end.Before(start) {
		return nil, fmt.Errorf("invalid event end")
	}
	if allDay && event.Props.Get(ical.PropDateTimeEnd) == nil {
		days := 1
		if prop := event.Props.Get(ical.PropDuration); prop != nil {
			duration, err := prop.Duration()
			if err != nil || duration <= 0 || duration%(24*time.Hour) != 0 {
				return nil, fmt.Errorf("invalid all-day duration")
			}
			days = int(duration / (24 * time.Hour))
		}
		end = start.AddDate(0, 0, days)
	}
	if allDay && (!end.After(start) || end.Hour() != 0 || end.Minute() != 0 || end.Second() != 0) {
		return nil, fmt.Errorf("invalid all-day event end")
	}
	due := end
	if allDay {
		due = end.Add(-time.Second)
	}
	description := calendarText(event, ical.PropDescription)
	link := calendarValue(event, ical.PropURL)
	if link == "" {
		link = calendarURLPattern.FindString(description)
	}
	return &calendarDeadline{
		assignment: assignment{Name: name, DueDate: due.Unix()},
		UID:        calendarText(event, ical.PropUID), Description: description, URL: link,
		Start: start, End: end, AllDay: allDay,
	}, nil
}

func calendarDays(start, end time.Time) int {
	a := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	b := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
	return int(b.Sub(a) / (24 * time.Hour))
}

func calendarOccurrences(ctx context.Context, event ical.Event, start time.Time, from, until time.Time) ([]time.Time, error) {
	set := &rrule.Set{}
	set.DTStart(start)
	set.RDate(start)
	if prop := event.Props.Get(ical.PropRecurrenceRule); prop != nil {
		option, err := rrule.StrToROptionInLocation(prop.Value, start.Location())
		if err != nil {
			return nil, fmt.Errorf("invalid recurrence rule")
		}
		option.Dtstart = start
		rule, err := rrule.NewRRule(*option)
		if err != nil {
			return nil, fmt.Errorf("invalid recurrence rule")
		}
		set.RRule(rule)
	}
	for _, name := range []string{ical.PropRecurrenceDates, ical.PropExceptionDates} {
		for _, prop := range event.Props[name] {
			for _, value := range strings.Split(prop.Value, ",") {
				item := prop
				item.Value = value
				date, err := item.DateTime(start.Location())
				if err != nil {
					return nil, fmt.Errorf("invalid recurrence date")
				}
				if name == ical.PropRecurrenceDates {
					set.RDate(date)
				} else {
					set.ExDate(date)
				}
			}
		}
	}
	next := set.Iterator()
	var dates []time.Time
	// Bound expansion even for feeds with old, very frequent rules.
	for count := 0; count < 100000; count++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		date, ok := next()
		if !ok || date.After(until) {
			return dates, nil
		}
		if !date.Before(from) {
			dates = append(dates, date)
		}
	}
	return nil, fmt.Errorf("calendar recurrence expansion limit exceeded")
}
