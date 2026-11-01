package safetrade

import (
	"net/url"
	"strconv"
	"strings"
	"time"
)

type query url.Values

func (q query) set(key, value string) {
	if value != "" {
		url.Values(q).Set(key, value)
	}
}

func (q query) setInt(key string, value int) {
	if value != 0 {
		q.set(key, strconv.Itoa(value))
	}
}

func (q query) setUnix(key string, t time.Time) {
	if !t.IsZero() {
		q.set(key, strconv.FormatInt(t.Unix(), 10))
	}
}

func setList[T ~string](q query, key string, values []T) {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			parts = append(parts, string(value))
		}
	}
	q.set(key, strings.Join(parts, ","))
}

func (q query) setPage(page, limit int) {
	q.setInt("page", page)
	q.setInt("limit", limit)
}

func (q query) setSort(orderBy string, ordering Ordering) {
	q.set("order_by", orderBy)
	q.set("ordering", string(ordering))
}

func pathSegment(name, value string) (string, error) {
	if value == "" {
		return "", invalidArgument("%s must not be empty", name)
	}
	if value == "." || value == ".." {
		return "", invalidArgument("%s %q is not a valid identifier", name, value)
	}
	return url.PathEscape(value), nil
}

func idSegment(name string, id int64) (string, error) {
	if id <= 0 {
		return "", invalidArgument("%s must be positive, got %d", name, id)
	}
	return strconv.FormatInt(id, 10), nil
}
