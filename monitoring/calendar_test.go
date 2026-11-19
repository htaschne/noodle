package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func calendarFeed(events ...string) string {
	return "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Noodle tests//EN\r\n" + strings.Join(events, "") + "END:VCALENDAR\r\n"
}

func calendarEvent(uid, properties string) string {
	return "BEGIN:VEVENT\r\nUID:" + uid + "\r\n" + strings.ReplaceAll(properties, "\n", "\r\n") + "\r\nEND:VEVENT\r\n"
}

func parseTestCalendar(t *testing.T, feed string, now time.Time) []calendarDeadline {
	t.Helper()
	deadlines, skipped, err := parseCalendar(context.Background(), []byte(feed), time.UTC, now)
	if err != nil || skipped != 0 {
		t.Fatalf("parse: skipped=%d err=%v", skipped, err)
	}
	return deadlines
}

func TestCalendarDeadlineExtraction(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	feed := calendarFeed(
		calendarEvent("due", "SUMMARY:Essay is due\nDESCRIPTION:Read chapter 1\\nSubmit essay\\, then review.\nDTSTART:20261120T025900Z\nDTEND:20261120T025900Z\nURL:https://moodle.example.test/mod/assign/view.php?id=12"),
		calendarEvent("pt", "SUMMARY:Trabalho está marcado(a) para esta data\nDTSTART:20261120T030000Z"),
		calendarEvent("quiz", "SUMMARY:Término de Teste Individual VI\nDTSTART:20261120T040000Z"),
		calendarEvent("meeting", "SUMMARY:Class meeting\nDTSTART:20261119T130000Z\nDESCRIPTION:Discuss the essay deadline."),
		calendarEvent("open", "SUMMARY:Início de Trabalho (Data limite)\nDTSTART:20261119T130000Z"),
		calendarEvent("plain-assignment", "SUMMARY:Essay\nDTSTART:20261119T130000Z\nURL:https://moodle.example.test/mod/assign/view.php?id=12"),
	)
	deadlines := parseTestCalendar(t, feed, now)
	if len(deadlines) != 3 {
		t.Fatalf("deadlines=%d, want 3", len(deadlines))
	}
	item := deadlines[0]
	if item.UID != "due" || item.Name != "Essay is due" || item.Description != "Read chapter 1\nSubmit essay, then review." || !strings.Contains(item.URL, "id=12") || item.DueDate != time.Date(2026, 11, 20, 2, 59, 0, 0, time.UTC).Unix() {
		t.Fatalf("metadata not preserved: %#v", item)
	}
}

func TestCalendarTimezonesAndAllDayEvents(t *testing.T) {
	tests := []struct{ name, calendarProps, properties, want string }{
		{"TZID", "", "DTSTART;TZID=America/Sao_Paulo:20261119T235900", "2026-11-20T02:59:00Z"},
		{"floating calendar timezone", "X-WR-TIMEZONE:America/Sao_Paulo\r\n", "DTSTART:20261119T235900", "2026-11-20T02:59:00Z"},
		{"all day", "X-WR-TIMEZONE:America/Sao_Paulo\r\n", "DTSTART;VALUE=DATE:20261119", "2026-11-20T02:59:59Z"},
		{"exclusive multi-day end", "X-WR-TIMEZONE:America/Sao_Paulo\r\n", "DTSTART;VALUE=DATE:20261118\nDTEND;VALUE=DATE:20261120", "2026-11-20T02:59:59Z"},
		{"DST day", "X-WR-TIMEZONE:America/New_York\r\n", "DTSTART;VALUE=DATE:20260308", "2026-03-09T03:59:59Z"},
		{"DST day duration", "X-WR-TIMEZONE:America/New_York\r\n", "DTSTART;VALUE=DATE:20260308\nDURATION:P1D", "2026-03-09T03:59:59Z"},
		{"duration", "", "DTSTART:20261119T220000Z\nDURATION:PT2H", "2026-11-20T00:00:00Z"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			want, _ := time.Parse(time.RFC3339, test.want)
			feed := strings.Replace(calendarFeed(calendarEvent("event", "SUMMARY:Essay is due\n"+test.properties)), "VERSION:2.0\r\n", "VERSION:2.0\r\n"+test.calendarProps, 1)
			deadlines := parseTestCalendar(t, feed, want.Add(-time.Hour))
			if len(deadlines) != 1 || deadlines[0].DueDate != want.Unix() {
				t.Fatalf("deadlines=%#v want=%s", deadlines, want)
			}
		})
	}
}

func TestCalendarRecurrenceExceptionsUpdatesAndCancellations(t *testing.T) {
	now := time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)
	master := calendarEvent("series", "SUMMARY:Essay is due\nDTSTART:20261118T120000Z\nRRULE:FREQ=DAILY;COUNT=4\nRDATE:20261119T140000Z,20261119T150000Z\nEXDATE:20261119T120000Z,20261119T140000Z")
	moved := calendarEvent("series", "RECURRENCE-ID:20261120T120000Z\nDTSTART:20261119T180000Z\nSEQUENCE:2")
	cancelled := calendarEvent("series", "RECURRENCE-ID:20261119T150000Z\nSTATUS:CANCELLED\nSEQUENCE:2")
	deadlines := parseTestCalendar(t, calendarFeed(master, moved, cancelled), now)
	if len(deadlines) != 1 || deadlines[0].Recurrence != "2026-11-20T12:00:00Z" || deadlines[0].DueDate != now.Add(18*time.Hour).Unix() {
		t.Fatalf("recurrence handling: %#v", deadlines)
	}
	// A master cancellation takes precedence over its previous version and instances.
	cancelMaster := calendarEvent("series", "STATUS:CANCELLED\nSEQUENCE:3")
	if got := parseTestCalendar(t, calendarFeed(master, moved, cancelMaster), now); len(got) != 0 {
		t.Fatalf("cancelled series produced deadlines: %#v", got)
	}
	newer := calendarEvent("single", "SUMMARY:Essay is due\nDTSTART:20261119T180000Z\nSEQUENCE:2")
	older := calendarEvent("single", "SUMMARY:Essay is due\nDTSTART:20261119T120000Z\nSEQUENCE:1")
	if got := parseTestCalendar(t, calendarFeed(newer, older), now); len(got) != 1 || got[0].DueDate != now.Add(18*time.Hour).Unix() {
		t.Fatalf("newest version not selected: %#v", got)
	}
}

func TestCalendarRecurrenceAcrossDST(t *testing.T) {
	now := time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)
	feed := calendarFeed(calendarEvent("dst", "SUMMARY:Essay is due\nDTSTART;TZID=America/New_York:20260307T090000\nRRULE:FREQ=DAILY;COUNT=3"))
	got := parseTestCalendar(t, feed, now)
	if len(got) != 1 || time.Unix(got[0].DueDate, 0).UTC().Format(time.RFC3339) != "2026-03-08T13:00:00Z" {
		t.Fatalf("DST recurrence: %#v", got)
	}
}

func TestCalendarDetachedInstanceInheritsDuration(t *testing.T) {
	now := time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)
	feed := calendarFeed(
		calendarEvent("series", "SUMMARY:Essay is due\nDTSTART:20261118T120000Z\nDURATION:PT2H\nRRULE:FREQ=DAILY;COUNT=3"),
		calendarEvent("series", "RECURRENCE-ID:20261119T120000Z\nDTSTART:20261119T160000Z"),
	)
	got := parseTestCalendar(t, feed, now)
	if len(got) != 1 || got[0].DueDate != now.Add(18*time.Hour).Unix() {
		t.Fatalf("detached instance lost duration: %#v", got)
	}
}

func TestCalendarInvalidEntriesAndFeeds(t *testing.T) {
	now := time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)
	for _, feed := range []string{"", "<html>Login</html>", "BEGIN:VCALENDAR\nBEGIN:VEVENT\n"} {
		if _, _, err := parseCalendar(context.Background(), []byte(feed), time.UTC, now); err == nil {
			t.Fatalf("accepted invalid feed %q", feed)
		}
	}
	feed := calendarFeed(
		calendarEvent("bad-time", "SUMMARY:Essay is due\nDTSTART:bad"),
		calendarEvent("bad-zone", "SUMMARY:Essay is due\nDTSTART;TZID=Unknown/Zone:20261119T120000"),
		calendarEvent("bad-rule", "SUMMARY:Essay is due\nDTSTART:20261119T120000Z\nRRULE:FREQ=NOTVALID"),
		calendarEvent("good", "SUMMARY:Essay is due\nDTSTART:20261119T120000Z"),
	)
	got, skipped, err := parseCalendar(context.Background(), []byte(feed), time.UTC, now)
	if err != nil || skipped != 3 || len(got) != 1 || got[0].UID != "good" {
		t.Fatalf("invalid entries: deadlines=%#v skipped=%d err=%v", got, skipped, err)
	}
}

func TestCalendarPollingDeduplicationUpdatesAndRestart(t *testing.T) {
	due := time.Now().UTC().Truncate(time.Second).Add(2 * time.Hour)
	feed := calendarFeed(calendarEvent("stable", "SUMMARY:Essay is due\nDTSTART:"+due.Format("20060102T150405Z")))
	status := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Accept") != "text/calendar" {
			t.Error("unexpected calendar request")
		}
		w.WriteHeader(status)
		fmt.Fprint(w, feed)
	}))
	defer server.Close()
	publisher := &recordingReminderPublisher{}
	factory := &recordingPublisherFactory{publisher: publisher}
	client := &calendarClient{feedURL: server.URL + "/calendar?authtoken=SECRET", email: "student@example.test", location: time.UTC, httpClient: server.Client(), newPublisher: factory.New}
	for range 2 {
		if err := client.monitor(context.Background(), discardLogger()); err != nil {
			t.Fatal(err)
		}
	}
	if len(publisher.messages) != 1 {
		t.Fatalf("unchanged polling published %d messages", len(publisher.messages))
	}
	first := publisher.messages[0]
	if first.CourseID <= 0 || first.AssignmentID <= 0 || first.StudentID <= 0 || first.AssignmentURL != server.URL+"/calendar/view.php" {
		t.Fatalf("incompatible reminder: %#v", first)
	}
	// Simulate a fresh monitoring process: it emits the same ID, which the
	// notification service's persistent history already knows how to suppress.
	client.published = nil
	client.feedURL = server.URL + "/calendar?authtoken=ROTATED"
	if err := client.monitor(context.Background(), discardLogger()); err != nil {
		t.Fatal(err)
	}
	if publisher.messages[1].ID != first.ID {
		t.Fatal("restart/token rotation changed reminder identity")
	}
	feed = strings.ReplaceAll(feed, due.Format("20060102T150405Z"), due.Add(time.Hour).Format("20060102T150405Z"))
	if err := client.monitor(context.Background(), discardLogger()); err != nil {
		t.Fatal(err)
	}
	if len(publisher.messages) != 3 || publisher.messages[2].ID == first.ID || !publisher.messages[2].DueAt.Equal(due.Add(time.Hour)) {
		t.Fatalf("updated deadline: %#v", publisher.messages)
	}
	status = http.StatusServiceUnavailable
	if err := client.monitor(context.Background(), discardLogger()); err == nil || strings.Contains(err.Error(), "ROTATED") {
		t.Fatalf("unavailable feed: %v", err)
	}
	status = http.StatusOK
	if err := client.monitor(context.Background(), discardLogger()); err != nil || len(publisher.messages) != 3 {
		t.Fatalf("recovery duplicated reminders: %v", err)
	}
	feed = calendarFeed(calendarEvent("stable", "STATUS:CANCELLED"))
	if err := client.monitor(context.Background(), discardLogger()); err != nil || len(publisher.messages) != 3 {
		t.Fatalf("cancellation published reminder: %v", err)
	}
}

func TestCalendarPublishFailureRetries(t *testing.T) {
	due := time.Now().UTC().Add(time.Hour).Format("20060102T150405Z")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, calendarFeed(calendarEvent("retry", "SUMMARY:Essay is due\nDTSTART:"+due)))
	}))
	defer server.Close()
	publisher := &recordingReminderPublisher{err: errors.New("broker unavailable")}
	factory := &recordingPublisherFactory{publisher: publisher}
	client := &calendarClient{feedURL: server.URL, email: "student@example.test", httpClient: server.Client(), newPublisher: factory.New}
	if err := client.monitor(context.Background(), discardLogger()); err == nil {
		t.Fatal("expected publication error")
	}
	publisher.err = nil
	if err := client.monitor(context.Background(), discardLogger()); err != nil || len(publisher.messages) != 1 {
		t.Fatalf("publication not retried: %v", err)
	}
}

func TestCalendarHTTPFailuresNeverExposeURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, "https://example.test/?authtoken=SECRET", http.StatusFound)
		case "/large":
			fmt.Fprint(w, strings.Repeat("x", maxMoodleResponseSize+1))
		default:
			http.Error(w, "SECRET", http.StatusForbidden)
		}
	}))
	for _, path := range []string{"/error", "/redirect", "/large"} {
		client := &calendarClient{feedURL: server.URL + path + "?authtoken=SECRET", httpClient: server.Client()}
		_, err := client.fetch(context.Background())
		if err == nil || strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), server.URL) {
			t.Fatalf("unsafe fetch error: %v", err)
		}
	}
	server.Close()
	client := &calendarClient{feedURL: server.URL + "?authtoken=SECRET", httpClient: server.Client()}
	if _, err := client.fetch(context.Background()); err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("unsafe network error: %v", err)
	}
}

func TestCalendarSourceSelection(t *testing.T) {
	setValidConfigEnvironment(t)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := newMonitoringSource(cfg, http.DefaultClient).(*moodleClient); !ok {
		t.Fatal("empty calendar URL did not select API")
	}
	t.Setenv("MOODLE_ICAL_URL", "https://moodle.example.test/calendar/export_execute.php?authtoken=SECRET")
	t.Setenv("MOODLE_ICAL_EMAIL", "student@example.test")
	t.Setenv("MOODLE_ICAL_TIMEZONE", "America/Sao_Paulo")
	for _, name := range []string{"MOODLE_URL", "MOODLE_USERNAME", "MOODLE_PASSWORD", "MOODLE_SERVICE"} {
		t.Setenv(name, "")
	}
	cfg, err = loadConfig()
	if err != nil {
		t.Fatalf("calendar required API credentials: %v", err)
	}
	if _, ok := newMonitoringSource(cfg, http.DefaultClient).(*calendarClient); !ok {
		t.Fatal("configured calendar did not take precedence")
	}
	for _, pair := range [][2]string{{"MOODLE_ICAL_URL", "://SECRET"}, {"MOODLE_ICAL_EMAIL", "invalid"}, {"MOODLE_ICAL_TIMEZONE", "Invalid/Zone"}} {
		t.Run(pair[0], func(t *testing.T) {
			t.Setenv(pair[0], pair[1])
			if _, err := loadConfig(); err == nil || strings.Contains(err.Error(), "SECRET") {
				t.Fatalf("invalid calendar setting: %v", err)
			}
		})
	}
}

func TestCalendarSecretLinksAndText(t *testing.T) {
	client := &calendarClient{feedURL: "https://moodle.example.test/calendar/export_execute.php?authtoken=SECRET"}
	if got := client.redact("Reminder SECRET " + client.feedURL); strings.Contains(got, "SECRET") || strings.Contains(got, "export_execute") {
		t.Fatalf("secret in reminder name: %s", got)
	}
	for _, link := range []string{client.feedURL, "javascript:alert(1)", "https://user:password@example.test", "https://example.test/?wstoken=SECRET"} {
		if got := safeCalendarLink(link, "https://moodle.example.test"); got != "https://moodle.example.test/calendar/view.php" {
			t.Fatalf("unsafe link accepted: %s", got)
		}
	}
}

// Opt-in read-only validation of a real export; never sends notifications.
func TestLiveCalendarIntegration(t *testing.T) {
	if os.Getenv("TEST_MOODLE_ICAL_LIVE") != "1" {
		t.Skip("set TEST_MOODLE_ICAL_LIVE=1 and MOODLE_ICAL_URL for live read-only validation")
	}
	client := &calendarClient{feedURL: os.Getenv("MOODLE_ICAL_URL"), httpClient: &http.Client{Timeout: 30 * time.Second}}
	body, err := client.fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	zone, _ := time.LoadLocation("America/Sao_Paulo")
	deadlines, skipped, err := parseCalendar(context.Background(), body, zone, time.Now())
	if err != nil || skipped != 0 {
		t.Fatalf("live parsing: skipped=%d err=%v", skipped, err)
	}
	t.Logf("live feed parsed; deadlines in expansion window: %d", len(deadlines))
}
