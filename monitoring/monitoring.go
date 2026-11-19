package main

import (
	"fmt"
	"strings"
	"time"
)

const assignmentEligibilityWindow = 24 * time.Hour

type enrolmentRoleClassification int

const (
	roleAmbiguous enrolmentRoleClassification = iota
	roleStudent
	roleNonStudent
)

type reminderCandidate struct {
	CourseID      int64
	AssignmentID  int64
	UserID        int64
	Email         string
	Assignment    string
	AssignmentURL string
	DueDate       time.Time
}

type candidateIdentification struct {
	Candidates           []reminderCandidate
	SubmissionDataOK     bool
	UnavailableReason    string
	SubmissionCount      int
	CompletedStudents    int
	IncompleteStudents   int
	UnclassifiedStudents int
}

type resolvedSubmission struct {
	submission submission
	ambiguous  bool
}

// isAssignmentEligible applies the reminder window to the assignment's standard
// deadline. Individual extensions are intentionally outside the current scope.
func isAssignmentEligible(candidate assignment, now time.Time) bool {
	if candidate.DueDate <= 0 || candidate.TeamSubmission != 0 || candidate.NoSubmissions != 0 {
		return false
	}

	now = now.UTC()
	deadline := time.Unix(candidate.DueDate, 0).UTC()
	return deadline.After(now) && !deadline.After(now.Add(assignmentEligibilityWindow))
}

// identifyPersonalReminder only trusts a successful, owner-specific latest
// attempt. Unlike absence in a bulk list, an explicit editable lastattempt with
// no submission confirms that the authenticated student has not started yet.
func identifyPersonalReminder(courseID int64, candidate assignment, userID int64, email string, response submissionStatusResponse, moodleURL string) candidateIdentification {
	result := candidateIdentification{}
	if courseID <= 0 || candidate.ID <= 0 || candidate.CMID <= 0 || userID <= 0 || !validCalendarEmail(email) {
		result.UnavailableReason = "invalid reminder metadata"
		return result
	}
	if len(response.Warnings) != 0 || response.LastAttempt == nil {
		result.UnavailableReason = "missing or incomplete own submission status"
		return result
	}
	attempt := response.LastAttempt
	if attempt.Submission != nil {
		if attempt.Submission.UserID != userID {
			result.UnavailableReason = "submission owner does not match authenticated student"
			return result
		}
		result.SubmissionCount = 1
		if normalizedSubmissionStatus(attempt.Submission.Status) == "submitted" {
			result.SubmissionDataOK = true
			result.CompletedStudents = 1
			return result
		}
		switch normalizedSubmissionStatus(attempt.Submission.Status) {
		case "new", "draft", "reopened":
		default:
			result.UnavailableReason = "unrecognized own submission status"
			return result
		}
	}
	if attempt.SubmissionsEnabled == nil || attempt.CanEditOwner == nil || attempt.Locked == nil || attempt.Graded == nil {
		result.UnavailableReason = "incomplete own attempt permissions"
		return result
	}
	result.SubmissionDataOK = true
	// Moodle 4.5 returns null when no individual extension exists.
	hasExtension := attempt.ExtensionDueDate != nil && *attempt.ExtensionDueDate > 0
	if !*attempt.SubmissionsEnabled || !*attempt.CanEditOwner || *attempt.Locked || attempt.TeamSubmission != nil || hasExtension || (attempt.Submission == nil && *attempt.Graded) {
		return result
	}
	result.IncompleteStudents = 1
	result.Candidates = []reminderCandidate{{
		CourseID: courseID, AssignmentID: candidate.ID, UserID: userID,
		Email: email, Assignment: candidate.Name,
		AssignmentURL: fmt.Sprintf("%s/mod/assign/view.php?id=%d", strings.TrimRight(moodleURL, "/"), candidate.CMID),
		DueDate:       time.Unix(candidate.DueDate, 0).UTC(),
	}}
	return result
}

func identifyReminderCandidates(courseID int64, candidate assignment, users []enrolledUser, response submissionsResponse, moodleURL string) candidateIdentification {
	result := candidateIdentification{}
	if candidate.CMID <= 0 {
		result.UnavailableReason = "assignment course module ID missing"
		return result
	}
	if len(response.Warnings) > 0 {
		result.UnavailableReason = "Moodle returned submission warnings"
		return result
	}

	assignmentResult, found := findAssignmentSubmissions(candidate.ID, response.Assignments)
	if !found {
		result.UnavailableReason = "assignment submission data missing from response"
		return result
	}

	result.SubmissionDataOK = true
	result.SubmissionCount = len(assignmentResult.Submissions)
	latestByUser := latestSubmissionsByUser(assignmentResult.Submissions)
	deadline := time.Unix(candidate.DueDate, 0).UTC()

	for _, user := range users {
		if !hasStudentRole(user.Roles) {
			continue
		}

		resolved, exists := latestByUser[user.ID]
		if !exists {
			// Moodle does not guarantee that a missing record means the user has
			// not submitted, so absence is never used as evidence of pending work.
			result.UnclassifiedStudents++
			continue
		}
		if resolved.ambiguous {
			// Conflicting statuses for the same attempt cannot be resolved from
			// the response alone, so they are not evidence for a reminder.
			result.UnclassifiedStudents++
			continue
		}

		switch normalizedSubmissionStatus(resolved.submission.Status) {
		case "submitted":
			result.CompletedStudents++
			continue
		case "new", "draft", "reopened":
			result.IncompleteStudents++
		default:
			// Unknown and empty statuses are not sufficient evidence to notify.
			result.UnclassifiedStudents++
			continue
		}

		email := strings.TrimSpace(user.Email)
		if email == "" {
			continue
		}

		result.Candidates = append(result.Candidates, reminderCandidate{
			CourseID:      courseID,
			AssignmentID:  candidate.ID,
			UserID:        user.ID,
			Email:         email,
			Assignment:    candidate.Name,
			AssignmentURL: fmt.Sprintf("%s/mod/assign/view.php?id=%d", strings.TrimRight(moodleURL, "/"), candidate.CMID),
			DueDate:       deadline,
		})
	}

	return result
}

func findAssignmentSubmissions(assignmentID int64, assignments []assignmentSubmissions) (assignmentSubmissions, bool) {
	for _, candidate := range assignments {
		if candidate.AssignmentID == assignmentID {
			return candidate, true
		}
	}
	return assignmentSubmissions{}, false
}

func latestSubmissionsByUser(submissions []submission) map[int64]resolvedSubmission {
	latest := make(map[int64]resolvedSubmission)
	for _, candidate := range submissions {
		if candidate.UserID <= 0 {
			continue
		}

		current, found := latest[candidate.UserID]
		if !found {
			latest[candidate.UserID] = resolvedSubmission{submission: candidate}
			continue
		}
		if sameSubmissionAttempt(candidate, current.submission) {
			if normalizedSubmissionStatus(candidate.Status) != normalizedSubmissionStatus(current.submission.Status) {
				current.ambiguous = true
			}
			if current.submission.Latest < candidate.Latest {
				current.submission = candidate
			}
			latest[candidate.UserID] = current
			continue
		}
		if submissionPrecedes(current.submission, candidate) {
			latest[candidate.UserID] = resolvedSubmission{submission: candidate}
		}
	}
	return latest
}

func submissionPrecedes(current, candidate submission) bool {
	if current.Latest != candidate.Latest {
		return current.Latest < candidate.Latest
	}
	return current.AttemptNumber < candidate.AttemptNumber
}

func sameSubmissionAttempt(left, right submission) bool {
	return left.AttemptNumber == right.AttemptNumber
}

func normalizedSubmissionStatus(status string) string {
	return strings.ToLower(strings.TrimSpace(status))
}

func hasStudentRole(roles []moodleRole) bool {
	return classifyEnrolmentRole(roles) == roleStudent
}

func classifyEnrolmentRole(roles []moodleRole) enrolmentRoleClassification {
	hasStudent := false
	hasNonStudent := false
	hasUnknown := false
	for _, role := range roles {
		shortName := strings.ToLower(strings.TrimSpace(role.ShortName))
		name := strings.ToLower(strings.TrimSpace(role.Name))

		// Moodle role IDs vary by installation. Standard semantic role names are
		// used instead, and unrecognized/custom roles remain ambiguous.
		switch shortName {
		case "manager", "coursecreator", "editingteacher", "teacher":
			hasNonStudent = true
		case "student":
			hasStudent = true
		case "":
			if name == "student" {
				hasStudent = true
			} else {
				hasUnknown = true
			}
		default:
			hasUnknown = true
		}
	}
	if hasNonStudent {
		return roleNonStudent
	}
	if hasUnknown {
		return roleAmbiguous
	}
	if hasStudent {
		return roleStudent
	}
	return roleAmbiguous
}
