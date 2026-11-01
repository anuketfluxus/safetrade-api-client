package redirect

import (
	"fmt"
	"net/http"
)

const maxRedirects = 10

func Restrict(client *http.Client, refuse func(*http.Request) bool) *http.Client {
	restricted := *client
	next := client.CheckRedirect
	restricted.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if refuse(req) {
			return fmt.Errorf("refusing to follow the redirect to %s of a request carrying credentials", req.URL.Redacted())
		}
		if next != nil {
			return next(req, via)
		}
		if len(via) >= maxRedirects {
			return fmt.Errorf("stopped after %d redirects", maxRedirects)
		}
		return nil
	}
	return &restricted
}
