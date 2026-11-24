package service

import (
	"errors"
	"net/http"
	"testing"

	"github.com/rafaelescrich/gira/internal/jira"
)

func TestWrapAPIErrorClassifiesAuthVsAPI(t *testing.T) {
	authSvcErr := wrapAPIError(&jira.APIError{StatusCode: http.StatusUnauthorized})
	if ExitCode(authSvcErr) != 3 {
		t.Errorf("401 -> ExitCode = %d, want 3 (auth)", ExitCode(authSvcErr))
	}

	apiSvcErr := wrapAPIError(&jira.APIError{StatusCode: http.StatusNotFound})
	if ExitCode(apiSvcErr) != 2 {
		t.Errorf("404 -> ExitCode = %d, want 2 (API)", ExitCode(apiSvcErr))
	}

	// A non-*jira.APIError still gets classified as an API error rather
	// than panicking or being dropped.
	generic := wrapAPIError(errors.New("boom"))
	if ExitCode(generic) != 2 {
		t.Errorf("generic error -> ExitCode = %d, want 2 (API)", ExitCode(generic))
	}

	if wrapAPIError(nil) != nil {
		t.Error("wrapAPIError(nil) should return nil")
	}
}

func TestErrorUnwrap(t *testing.T) {
	cause := &jira.APIError{StatusCode: http.StatusNotFound}
	svcErr := wrapAPIError(cause)

	var apiErr *jira.APIError
	if !errors.As(svcErr, &apiErr) {
		t.Fatal("errors.As should see through *service.Error to the wrapped *jira.APIError")
	}
	if apiErr != cause {
		t.Error("Unwrap() did not return the original cause")
	}
}
