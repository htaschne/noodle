package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"
)

const (
	defaultMonitorInterval = time.Hour
	defaultHTTPTimeout     = 30 * time.Second
	operationAssignments   = "get_assignments"
	operationSubmissions   = "get_submission_status"
)

type config struct {
	moodleURL         string
	moodleUsername    string
	moodlePassword    string
	moodleService     string
	moodleSource      string
	notificationEmail string
	moodleICalURL     string
	moodleICalEmail   string
	calendarZone      *time.Location
	monitorInterval   time.Duration
	logLevel          slog.Level
}

type cycleSummary struct {
	courses                  int
	assignments              int
	eligibleAssignments      int
	skippedAssignments       int
	enrolledStudents         int
	submissions              int
	candidates               int
	publishedReminders       int
	warningCount             int
	warningCountsByOperation map[string]int
	warningGroups            map[warningKey]*warningGroup
	partialCoverage          bool
}

type warningKey struct {
	operation string
	code      string
}

type warningGroup struct {
	count   int
	message string
}

type warningCounts map[string]int

type reminderPublisher interface {
	Publish(context.Context, reminderMessage) error
	Close() error
}

type reminderPublisherFactory func(context.Context) (reminderPublisher, error)

func (counts warningCounts) LogValue() slog.Value {
	operations := make([]string, 0, len(counts))
	for operation := range counts {
		operations = append(operations, operation)
	}
	sort.Strings(operations)

	attributes := make([]slog.Attr, 0, len(operations))
	for _, operation := range operations {
		attributes = append(attributes, slog.Int(operation, counts[operation]))
	}
	return slog.GroupValue(attributes...)
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		logger := newLogger(slog.LevelInfo)
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	logger := newLogger(cfg.logLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := newMonitoringSource(cfg, &http.Client{Timeout: defaultHTTPTimeout})
	logger.Info(
		"monitoring service started",
		"interval", cfg.monitorInterval,
		"log_level", cfg.logLevel.String(),
	)
	run(ctx, client, cfg.monitorInterval, logger)
	logger.Info("monitoring service stopped")
}

func newLogger(level slog.Level) *slog.Logger {
	levelVar := new(slog.LevelVar)
	levelVar.Set(level)
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: levelVar,
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			if attr.Key == slog.TimeKey {
				return slog.String(slog.TimeKey, attr.Value.Time().UTC().Format(time.RFC3339))
			}
			return attr
		},
	})
	return slog.New(handler)
}

func loadConfig() (config, error) {
	cfg := config{
		moodleURL:         strings.TrimRight(strings.TrimSpace(os.Getenv("MOODLE_URL")), "/"),
		moodleUsername:    os.Getenv("MOODLE_USERNAME"),
		moodlePassword:    os.Getenv("MOODLE_PASSWORD"),
		moodleService:     os.Getenv("MOODLE_SERVICE"),
		moodleSource:      strings.ToLower(strings.TrimSpace(os.Getenv("MOODLE_SOURCE"))),
		notificationEmail: strings.TrimSpace(os.Getenv("NOTIFICATION_EMAIL")),
		moodleICalURL:     strings.TrimSpace(os.Getenv("MOODLE_ICAL_URL")),
		moodleICalEmail:   strings.TrimSpace(os.Getenv("MOODLE_ICAL_EMAIL")),
		calendarZone:      time.UTC,
		monitorInterval:   defaultMonitorInterval,
		logLevel:          slog.LevelInfo,
	}

	if cfg.moodleSource == "" || cfg.moodleSource == "auto" {
		cfg.moodleSource = "api"
		if cfg.moodleICalURL != "" {
			cfg.moodleSource = "ical"
		}
	}
	if cfg.moodleSource != "api" && cfg.moodleSource != "ical" {
		return config{}, fmt.Errorf("MOODLE_SOURCE must be auto, api, or ical")
	}
	if cfg.notificationEmail == "" {
		cfg.notificationEmail = cfg.moodleICalEmail
	}
	if cfg.moodleSource == "ical" {
		parsed, err := url.Parse(cfg.moodleICalURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
			return config{}, fmt.Errorf("MOODLE_ICAL_URL must be a valid HTTP or HTTPS URL without user credentials or fragment")
		}
		if cfg.moodleICalEmail == "" {
			cfg.moodleICalEmail = cfg.notificationEmail
		}
		if !validCalendarEmail(cfg.moodleICalEmail) {
			return config{}, fmt.Errorf("MOODLE_ICAL_EMAIL must be a recipient email address when MOODLE_ICAL_URL is configured")
		}
		zone := strings.TrimSpace(os.Getenv("MOODLE_ICAL_TIMEZONE"))
		if zone != "" {
			cfg.calendarZone, err = time.LoadLocation(zone)
			if err != nil {
				return config{}, fmt.Errorf("MOODLE_ICAL_TIMEZONE must be an IANA timezone")
			}
		}
	} else {
		if !validCalendarEmail(cfg.notificationEmail) {
			return config{}, fmt.Errorf("NOTIFICATION_EMAIL must be a recipient email address in personal API mode")
		}
		if cfg.moodleURL == "" {
			return config{}, fmt.Errorf("MOODLE_URL is required")
		}
		parsedURL, err := url.Parse(cfg.moodleURL)
		if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
			return config{}, fmt.Errorf("MOODLE_URL must be a valid HTTP or HTTPS URL")
		}
		if parsedURL.User != nil {
			return config{}, fmt.Errorf("MOODLE_URL must not contain credentials")
		}
		if cfg.moodleUsername == "" {
			return config{}, fmt.Errorf("MOODLE_USERNAME is required")
		}
		if cfg.moodlePassword == "" {
			return config{}, fmt.Errorf("MOODLE_PASSWORD is required")
		}
		if cfg.moodleService == "" {
			cfg.moodleService = "moodle_mobile_app"
		}
	}

	if rawInterval := strings.TrimSpace(os.Getenv("MONITOR_INTERVAL")); rawInterval != "" {
		interval, err := time.ParseDuration(rawInterval)
		if err != nil || interval <= 0 {
			return config{}, fmt.Errorf("MONITOR_INTERVAL must be a positive duration")
		}
		cfg.monitorInterval = interval
	}
	if rawLogLevel := strings.TrimSpace(os.Getenv("LOG_LEVEL")); rawLogLevel != "" {
		switch strings.ToLower(rawLogLevel) {
		case "debug":
			cfg.logLevel = slog.LevelDebug
		case "info":
			cfg.logLevel = slog.LevelInfo
		case "warn":
			cfg.logLevel = slog.LevelWarn
		case "error":
			cfg.logLevel = slog.LevelError
		default:
			return config{}, fmt.Errorf("LOG_LEVEL must be one of debug, info, warn, error")
		}
	}
	return cfg, nil
}

type monitoringSource interface {
	monitor(context.Context, *slog.Logger) error
}

func newMonitoringSource(cfg config, httpClient *http.Client) monitoringSource {
	if cfg.moodleSource == "ical" {
		return &calendarClient{feedURL: cfg.moodleICalURL, email: cfg.moodleICalEmail, location: cfg.calendarZone, httpClient: httpClient, published: make(map[string]int64), newPublisher: newReminderPublisher}
	}
	client := newMoodleClient(cfg.moodleURL, cfg.moodleUsername, cfg.moodlePassword, cfg.moodleService, httpClient)
	client.email = cfg.notificationEmail
	return client
}

func (client *moodleClient) monitor(ctx context.Context, logger *slog.Logger) error {
	return monitorCycle(ctx, client, logger)
}

func run(ctx context.Context, client monitoringSource, interval time.Duration, logger *slog.Logger) {
	for {
		if ctx.Err() != nil {
			return
		}

		if err := client.monitor(ctx, logger); err != nil && ctx.Err() == nil {
			logger.Error("monitoring cycle failed", "error", err)
		}

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func monitorCycle(ctx context.Context, client *moodleClient, logger *slog.Logger) error {
	startedAt := time.Now().UTC()
	logger.Info("monitoring cycle started")

	token, err := client.authenticate(ctx)
	if err != nil {
		return fmt.Errorf("authenticate with Moodle: %w", err)
	}
	logger.Info("authenticated with Moodle")

	siteInfo, err := client.getSiteInfo(ctx, token)
	if err != nil {
		return fmt.Errorf("get site info: %w", err)
	}

	courses, err := client.getMyCourses(ctx, token, siteInfo.UserID)
	if err != nil {
		return fmt.Errorf("get courses: %w", err)
	}
	summary := newCycleSummary(len(courses))
	summary.enrolledStudents = 1
	cycleCandidates := make([]reminderCandidate, 0)
	logger.Info("discovered courses", "count", summary.courses)

	for _, course := range courses {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		assignmentResponse, err := client.getAssignments(ctx, token, course.ID)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			summary.partialCoverage = true
			logger.Error("could not retrieve assignments", "course_id", course.ID, "course", displayCourseName(course), "error", err)
			continue
		}

		assignmentCount := 0
		for _, assignmentCourse := range assignmentResponse.Courses {
			assignmentCount += len(assignmentCourse.Assignments)
		}
		summary.assignments += assignmentCount
		logger.Debug("assignments retrieved", "course_id", course.ID, "count", assignmentCount)
		summary.recordWarnings(logger, operationAssignments, course.ID, 0, assignmentResponse.Warnings)

		now := time.Now().UTC()
		eligibleAssignments := make([]assignment, 0, assignmentCount)
		for _, assignmentCourse := range assignmentResponse.Courses {
			for _, candidate := range assignmentCourse.Assignments {
				if isAssignmentEligible(candidate, now) {
					eligibleAssignments = append(eligibleAssignments, candidate)
				}
			}
		}
		summary.eligibleAssignments += len(eligibleAssignments)
		summary.skippedAssignments += assignmentCount - len(eligibleAssignments)
		logger.Debug(
			"assignments filtered",
			"course_id", course.ID,
			"eligible", len(eligibleAssignments),
			"skipped", assignmentCount-len(eligibleAssignments),
		)
		if len(eligibleAssignments) == 0 {
			continue
		}

		for _, candidate := range eligibleAssignments {
			status, err := client.getOwnSubmissionStatus(ctx, token, candidate.ID)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				summary.partialCoverage = true
				logSubmissionFailure(logger, course.ID, candidate.ID, err)
				continue
			}
			summary.recordWarnings(logger, operationSubmissions, course.ID, candidate.ID, status.Warnings)
			identification := identifyPersonalReminder(course.ID, candidate, siteInfo.UserID, client.email, status, client.baseURL)
			if !identification.SubmissionDataOK {
				summary.partialCoverage = true
				logger.Warn("own submission status unavailable", "course_id", course.ID, "assignment_id", candidate.ID, "reason", identification.UnavailableReason)
				continue
			}
			summary.submissions += identification.SubmissionCount
			summary.candidates += len(identification.Candidates)
			cycleCandidates = append(cycleCandidates, identification.Candidates...)
			logger.Debug("own submission checked", "course_id", course.ID, "assignment_id", candidate.ID,
				"completed", identification.CompletedStudents, "pending", identification.IncompleteStudents)
		}
	}

	published, err := publishReminderCandidates(ctx, client.newPublisher, cycleCandidates, logger)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("publish reminder candidates: %w", err)
	}
	summary.publishedReminders = published
	summary.logAggregatedWarnings(logger)
	logger.Info(
		"monitoring cycle completed",
		"courses", summary.courses,
		"assignments", summary.assignments,
		"eligible_assignments", summary.eligibleAssignments,
		"skipped_assignments", summary.skippedAssignments,
		"enrolled_students", summary.enrolledStudents,
		"submissions", summary.submissions,
		"candidates", summary.candidates,
		"published_reminders", summary.publishedReminders,
		"moodle_warnings", summary.warningCount,
		"warning_counts_by_operation", warningCounts(summary.warningCountsByOperation),
		"coverage", summary.coverage(),
		"duration", time.Since(startedAt).Round(time.Millisecond),
	)
	return nil
}

func newCycleSummary(courses int) cycleSummary {
	return cycleSummary{
		courses:                  courses,
		warningCountsByOperation: make(map[string]int),
		warningGroups:            make(map[warningKey]*warningGroup),
	}
}

func (summary *cycleSummary) recordWarnings(logger *slog.Logger, operation string, courseID, assignmentID int64, warnings []moodleWarning) {
	for _, warning := range warnings {
		code := strings.TrimSpace(warning.WarningCode)
		if code == "" {
			code = "unknown"
		}

		summary.warningCount++
		summary.warningCountsByOperation[operation]++
		summary.partialCoverage = true

		key := warningKey{operation: operation, code: code}
		group := summary.warningGroups[key]
		if group == nil {
			group = &warningGroup{message: warning.Message}
			summary.warningGroups[key] = group
		}
		group.count++
		if group.message == "" && warning.Message != "" {
			group.message = warning.Message
		}

		attributes := []any{
			"operation", operation,
			"moodle_code", code,
			"moodle_message", warning.Message,
			"course_id", courseID,
		}
		if assignmentID > 0 {
			attributes = append(attributes, "assignment_id", assignmentID)
		}
		if warning.Item != "" {
			attributes = append(attributes, "item", warning.Item)
		}
		if warning.ItemID > 0 {
			attributes = append(attributes, "item_id", warning.ItemID)
		}
		logger.Debug("Moodle returned warning", attributes...)
	}
}

func (summary *cycleSummary) logAggregatedWarnings(logger *slog.Logger) {
	keys := make([]warningKey, 0, len(summary.warningGroups))
	for key := range summary.warningGroups {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool {
		if keys[left].operation == keys[right].operation {
			return keys[left].code < keys[right].code
		}
		return keys[left].operation < keys[right].operation
	})

	for _, key := range keys {
		group := summary.warningGroups[key]
		logger.Warn(
			"Moodle warnings reduced monitoring coverage",
			"operation", key.operation,
			"moodle_code", key.code,
			"count", group.count,
			"moodle_message", group.message,
		)
	}
}

func (summary *cycleSummary) coverage() string {
	if summary.partialCoverage {
		return "partial"
	}
	return "complete"
}

func countEnrolmentRoles(users []enrolledUser) (students, ambiguous int) {
	for _, user := range users {
		switch classifyEnrolmentRole(user.Roles) {
		case roleStudent:
			students++
		case roleAmbiguous:
			ambiguous++
		}
	}
	return students, ambiguous
}

func displayCourseName(course moodleCourse) string {
	if course.FullName != "" {
		return course.FullName
	}
	if course.ShortName != "" {
		return course.ShortName
	}
	return "unknown"
}

func formatDueDate(timestamp int64) string {
	if timestamp <= 0 {
		return "none"
	}
	return time.Unix(timestamp, 0).UTC().Format(time.RFC3339)
}

func summarizeSubmissions(response submissionsResponse) (int, string) {
	counts := make(map[string]int)
	total := 0
	for _, assignment := range response.Assignments {
		for _, submission := range assignment.Submissions {
			status := strings.TrimSpace(submission.Status)
			if status == "" {
				status = "unknown"
			}
			counts[status]++
			total++
		}
	}

	if total == 0 {
		return 0, "none"
	}

	statuses := make([]string, 0, len(counts))
	for status, count := range counts {
		statuses = append(statuses, fmt.Sprintf("%s=%d", status, count))
	}
	sort.Strings(statuses)
	return total, strings.Join(statuses, ",")
}

func logSubmissionFailure(logger *slog.Logger, courseID, assignmentID int64, err error) {
	attributes := []any{
		"course_id", courseID,
		"assignment_id", assignmentID,
		"reason", "request failed",
		"error", err,
	}
	var apiErr *moodleAPIError
	if errors.As(err, &apiErr) {
		attributes = append(attributes, "moodle_code", apiErr.ErrorCode, "moodle_message", apiErr.Message)
	}
	logger.Error("could not retrieve submissions", attributes...)
}

func publishReminderCandidates(
	ctx context.Context,
	newPublisher reminderPublisherFactory,
	candidates []reminderCandidate,
	logger *slog.Logger,
) (int, error) {
	if len(candidates) == 0 {
		return 0, nil
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	publisher, err := newPublisher(ctx)
	if err != nil {
		logger.Error("could not initialize RabbitMQ publisher", "error", err)
		return 0, fmt.Errorf("initialize RabbitMQ publisher: %w", err)
	}

	published := 0
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return published, errors.Join(err, publisher.Close())
		}
		message := newReminderMessage(
			candidate.CourseID,
			candidate.AssignmentID,
			candidate.UserID,
			candidate.Email,
			candidate.Assignment,
			candidate.AssignmentURL,
			candidate.DueDate,
		)
		if err := publisher.Publish(ctx, message); err != nil {
			if ctx.Err() != nil {
				return published, errors.Join(ctx.Err(), publisher.Close())
			}
			publishErr := errors.Join(err, publisher.Close())
			logger.Error(
				"could not publish reminder candidate",
				"course_id", candidate.CourseID,
				"assignment_id", candidate.AssignmentID,
				"published", published,
				"error", publishErr,
			)
			return published, fmt.Errorf("publish reminder candidate: %w", publishErr)
		}
		published++
	}
	if err := publisher.Close(); err != nil {
		logger.Error("could not close RabbitMQ publisher", "published", published, "error", err)
		return published, fmt.Errorf("close RabbitMQ publisher: %w", err)
	}
	return published, nil
}

func newReminderPublisher(ctx context.Context) (reminderPublisher, error) {
	return newRabbitMQPublisher(ctx)
}
