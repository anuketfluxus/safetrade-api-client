package endpoint

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

func ParseBaseURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {

		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return "", fmt.Errorf("invalid base URL: %w", err)
	}
	if u.User != nil {
		return "", errors.New("invalid base URL: user info is not allowed")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("invalid base URL %q: scheme must be http or https", raw)
	}
	if u.Host == "" {
		return "", fmt.Errorf("invalid base URL %q: host is missing", raw)
	}
	if u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", fmt.Errorf("invalid base URL %q: query and fragment are not allowed", raw)
	}
	return strings.TrimRight(u.String(), "/"), nil
}
