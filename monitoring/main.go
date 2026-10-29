package main

import (
	"context"
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
)

type config struct {
	moodleURL       string
	moodleUsername  string
	moodlePassword  string
	moodleService   string
	monitorInterval time.Duration
}

type cycleSummary struct {
	courses     int
	assignments int
	submissions int
}

func main() {
	logger := newLogger()
	cfg, err := loadConfig()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := newMoodleClient(
		cfg.moodleURL,
		cfg.moodleUsername,
		cfg.moodlePassword,
		cfg.moodleService,
		&http.Client{Timeout: defaultHTTPTimeout},
	)

	logger.Info("monitoring service started", "interval", cfg.monitorInterval)
	run(ctx, client, cfg.monitorInterval, logger)
	logger.Info("monitoring service stopped")
}

func newLogger() *slog.Logger {
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
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
		moodleURL:       strings.TrimRight(strings.TrimSpace(os.Getenv("MOODLE_URL")), "/"),
		moodleUsername:  os.Getenv("MOODLE_USERNAME"),
		moodlePassword:  os.Getenv("MOODLE_PASSWORD"),
		moodleService:   os.Getenv("MOODLE_SERVICE"),
		monitorInterval: defaultMonitorInterval,
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

	if rawInterval := strings.TrimSpace(os.Getenv("MONITOR_INTERVAL")); rawInterval != "" {
		interval, err := time.ParseDuration(rawInterval)
		if err != nil || interval <= 0 {
			return config{}, fmt.Errorf("MONITOR_INTERVAL must be a positive duration")
		}
		cfg.monitorInterval = interval
	}

	return cfg, nil
}

func run(ctx context.Context, client *moodleClient, interval time.Duration, logger *slog.Logger) {
	for {
		if ctx.Err() != nil {
			return
		}

		if err := monitorCycle(ctx, client, logger); err != nil && ctx.Err() == nil {
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
	startedAt := time.Now()
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

	summary := cycleSummary{courses: len(courses)}
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
			logger.Error("could not retrieve assignments", "course_id", course.ID, "course", displayCourseName(course), "error", err)
			continue
		}

		assignmentCount := 0
		for _, assignmentCourse := range assignmentResponse.Courses {
			assignmentCount += len(assignmentCourse.Assignments)
		}
		logger.Info("course assignments discovered", "course_id", course.ID, "course", displayCourseName(course), "assignments", assignmentCount)
		logWarnings(logger, "get assignments", assignmentResponse.Warnings)

		for _, assignmentCourse := range assignmentResponse.Courses {
			courseName := assignmentCourse.FullName
			if courseName == "" {
				courseName = displayCourseName(course)
			}

			for _, assignment := range assignmentCourse.Assignments {
				summary.assignments++
				submissionResponse, err := client.getSubmissions(ctx, token, assignment.ID)
				if err != nil {
					if ctx.Err() != nil {
						return ctx.Err()
					}
					logger.Error(
						"could not retrieve submissions",
						"course_id", assignmentCourse.ID,
						"course", courseName,
						"assignment_id", assignment.ID,
						"assignment", assignment.Name,
						"error", err,
					)
					continue
				}

				submissionCount, statuses := summarizeSubmissions(submissionResponse)
				summary.submissions += submissionCount
				logger.Info(
					"assignment submissions retrieved",
					"course_id", assignmentCourse.ID,
					"course", courseName,
					"assignment_id", assignment.ID,
					"assignment", assignment.Name,
					"due", formatDueDate(assignment.DueDate),
					"team_submission", assignment.TeamSubmission != 0,
					"no_submissions", assignment.NoSubmissions != 0,
					"submissions", submissionCount,
					"statuses", statuses,
				)
				logWarnings(logger, "get submissions", submissionResponse.Warnings)
			}
		}
	}

	logger.Info(
		"monitoring cycle completed",
		"courses", summary.courses,
		"assignments", summary.assignments,
		"submissions", summary.submissions,
		"duration", time.Since(startedAt).Round(time.Millisecond),
	)
	return nil
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

func logWarnings(logger *slog.Logger, operation string, warnings []moodleWarning) {
	if len(warnings) == 0 {
		return
	}

	codes := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		if warning.WarningCode != "" {
			codes = append(codes, warning.WarningCode)
		}
	}
	sort.Strings(codes)
	logger.Warn("Moodle returned warnings", "operation", operation, "count", len(warnings), "codes", strings.Join(codes, ","))
}
