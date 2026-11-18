package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const maxMoodleResponseSize = 10 << 20

type moodleClient struct {
	baseURL    string
	username   string
	password   string
	service    string
	httpClient *http.Client
}

type moodleAPIError struct {
	Exception string
	ErrorCode string
	Message   string
}

func (err *moodleAPIError) Error() string {
	return fmt.Sprintf("Moodle API error exception=%q code=%q", err.Exception, err.ErrorCode)
}

type apiErrorEnvelope struct {
	Exception string `json:"exception"`
	ErrorCode string `json:"errorcode"`
	Message   string `json:"message"`
	ErrorText string `json:"error"`
}

type tokenResponse struct {
	Token string `json:"token"`
}

type siteInfoResponse struct {
	UserID int64 `json:"userid"`
}

type moodleCourse struct {
	ID        int64  `json:"id"`
	FullName  string `json:"fullname"`
	ShortName string `json:"shortname"`
}

type assignmentsResponse struct {
	Courses  []assignmentCourse `json:"courses"`
	Warnings []moodleWarning    `json:"warnings"`
}

type assignmentCourse struct {
	ID          int64        `json:"id"`
	FullName    string       `json:"fullname"`
	Assignments []assignment `json:"assignments"`
}

type assignment struct {
	ID             int64  `json:"id"`
	CMID           int64  `json:"cmid"`
	Course         int64  `json:"course"`
	Name           string `json:"name"`
	DueDate        int64  `json:"duedate"`
	TeamSubmission int    `json:"teamsubmission"`
	NoSubmissions  int    `json:"nosubmissions"`
}

type enrolledUser struct {
	ID       int64        `json:"id"`
	FullName string       `json:"fullname"`
	Email    string       `json:"email"`
	Roles    []moodleRole `json:"roles"`
}

type moodleRole struct {
	ID        int64  `json:"roleid"`
	Name      string `json:"name"`
	ShortName string `json:"shortname"`
}

type submissionsResponse struct {
	Assignments []assignmentSubmissions `json:"assignments"`
	Warnings    []moodleWarning         `json:"warnings"`
}

type assignmentSubmissions struct {
	AssignmentID int64        `json:"assignmentid"`
	Submissions  []submission `json:"submissions"`
}

type submission struct {
	UserID        int64  `json:"userid"`
	Status        string `json:"status"`
	AttemptNumber int64  `json:"attemptnumber"`
	Latest        int    `json:"latest"`
}

type moodleWarning struct {
	Item        string `json:"item"`
	ItemID      int64  `json:"itemid"`
	WarningCode string `json:"warningcode"`
	Message     string `json:"message"`
}

func newMoodleClient(baseURL, username, password, service string, httpClient *http.Client) *moodleClient {
	return &moodleClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		username:   username,
		password:   password,
		service:    service,
		httpClient: httpClient,
	}
}

func (client *moodleClient) authenticate(ctx context.Context) (string, error) {
	form := url.Values{
		"username": {client.username},
		"password": {client.password},
		"service":  {client.service},
	}
	var response tokenResponse
	if err := client.postForm(ctx, "/login/token.php", form, &response); err != nil {
		return "", err
	}
	if strings.TrimSpace(response.Token) == "" {
		return "", fmt.Errorf("Moodle authentication response did not contain a token")
	}
	return response.Token, nil
}

func (client *moodleClient) getSiteInfo(ctx context.Context, token string) (siteInfoResponse, error) {
	form := webServiceForm(token, "core_webservice_get_site_info")
	var response siteInfoResponse
	if err := client.postForm(ctx, "/webservice/rest/server.php", form, &response); err != nil {
		return siteInfoResponse{}, err
	}
	if response.UserID <= 0 {
		return siteInfoResponse{}, fmt.Errorf("Moodle site info response did not contain a user ID")
	}
	return response, nil
}

func (client *moodleClient) getMyCourses(ctx context.Context, token string, userID int64) ([]moodleCourse, error) {
	form := webServiceForm(token, "core_enrol_get_users_courses")
	form.Set("userid", fmt.Sprintf("%d", userID))
	var courses []moodleCourse
	if err := client.postForm(ctx, "/webservice/rest/server.php", form, &courses); err != nil {
		return nil, err
	}
	return courses, nil
}

func (client *moodleClient) getAssignments(ctx context.Context, token string, courseID int64) (assignmentsResponse, error) {
	form := webServiceForm(token, "mod_assign_get_assignments")
	form.Set("courseids[0]", fmt.Sprintf("%d", courseID))
	var response assignmentsResponse
	if err := client.postForm(ctx, "/webservice/rest/server.php", form, &response); err != nil {
		return assignmentsResponse{}, err
	}
	client.sanitizeWarnings(response.Warnings, token)
	return response, nil
}

func (client *moodleClient) getEnrolledUsers(ctx context.Context, token string, courseID int64) ([]enrolledUser, error) {
	form := webServiceForm(token, "core_enrol_get_enrolled_users")
	form.Set("courseid", fmt.Sprintf("%d", courseID))
	var users []enrolledUser
	if err := client.postForm(ctx, "/webservice/rest/server.php", form, &users); err != nil {
		return nil, err
	}
	return users, nil
}

func (client *moodleClient) getSubmissions(ctx context.Context, token string, assignmentID int64) (submissionsResponse, error) {
	form := webServiceForm(token, "mod_assign_get_submissions")
	form.Set("assignmentids[0]", fmt.Sprintf("%d", assignmentID))
	var response submissionsResponse
	if err := client.postForm(ctx, "/webservice/rest/server.php", form, &response); err != nil {
		return submissionsResponse{}, err
	}
	client.sanitizeWarnings(response.Warnings, token)
	return response, nil
}

func (client *moodleClient) sanitizeWarnings(warnings []moodleWarning, token string) {
	for index := range warnings {
		warnings[index].Item = redactSecrets(
			cleanText(warnings[index].Item),
			client.username,
			client.password,
			token,
		)
		warnings[index].WarningCode = cleanText(warnings[index].WarningCode)
		warnings[index].Message = redactSecrets(
			cleanText(warnings[index].Message),
			client.username,
			client.password,
			token,
		)
	}
}

func webServiceForm(token, function string) url.Values {
	return url.Values{
		"wstoken":            {token},
		"wsfunction":         {function},
		"moodlewsrestformat": {"json"},
	}
}

func (client *moodleClient) postForm(ctx context.Context, path string, form url.Values, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.baseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("create Moodle request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")

	response, err := client.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("send Moodle request: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxMoodleResponseSize+1))
	if err != nil {
		return fmt.Errorf("read Moodle response: %w", err)
	}
	if len(body) > maxMoodleResponseSize {
		return fmt.Errorf("Moodle response exceeded %d bytes", maxMoodleResponseSize)
	}

	if apiErr := client.decodeAPIError(body, form.Get("wstoken")); apiErr != nil {
		return apiErr
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("Moodle request returned HTTP status %d", response.StatusCode)
	}
	if len(body) == 0 {
		return fmt.Errorf("Moodle returned an empty response")
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode Moodle JSON response: %w", err)
	}
	return nil
}

func (client *moodleClient) decodeAPIError(body []byte, token string) error {
	var envelope apiErrorEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil
	}
	if envelope.Exception == "" && envelope.ErrorCode == "" && envelope.ErrorText == "" {
		return nil
	}

	message := envelope.Message
	if message == "" {
		message = envelope.ErrorText
	}
	return &moodleAPIError{
		Exception: cleanText(envelope.Exception),
		ErrorCode: cleanText(envelope.ErrorCode),
		Message:   redactSecrets(cleanText(message), client.username, client.password, token),
	}
}

func cleanText(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	const maximumLength = 500
	if len(value) > maximumLength {
		return value[:maximumLength] + "..."
	}
	return value
}

func redactSecrets(value string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
		}
	}
	return value
}
