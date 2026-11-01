package safetrade

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"
)

type Authenticator interface {
	Authenticate(req *http.Request) error
}

const (
	headerAPIKey    = "X-Auth-Apikey"
	headerNonce     = "X-Auth-Nonce"
	headerSignature = "X-Auth-Signature"
)

type HMACAuth struct {
	apiKey    string
	apiSecret string
	now       func() time.Time
	lastNonce atomic.Int64
}

func NewHMACAuth(apiKey, apiSecret string) (*HMACAuth, error) {
	if apiKey == "" || apiSecret == "" {
		return nil, invalidArgument("API key and secret must not be empty")
	}
	return &HMACAuth{apiKey: apiKey, apiSecret: apiSecret, now: time.Now}, nil
}

func (a *HMACAuth) Authenticate(req *http.Request) error {
	nonce := strconv.FormatInt(a.nextNonce(), 10)
	req.Header.Set(headerAPIKey, a.apiKey)
	req.Header.Set(headerNonce, nonce)
	req.Header.Set(headerSignature, a.sign(nonce))
	return nil
}

func (a *HMACAuth) String() string {
	return "HMACAuth{APIKey: " + a.apiKey + ", APISecret: [REDACTED]}"
}

func (a *HMACAuth) GoString() string {
	return a.String()
}

func (a *HMACAuth) nextNonce() int64 {
	now := a.now().UnixMilli()
	for {
		last := a.lastNonce.Load()
		next := max(now, last+1)
		if a.lastNonce.CompareAndSwap(last, next) {
			return next
		}
	}
}

func (a *HMACAuth) sign(nonce string) string {
	mac := hmac.New(sha256.New, []byte(a.apiSecret))
	mac.Write([]byte(nonce + a.apiKey))
	return hex.EncodeToString(mac.Sum(nil))
}

type BasicAuth struct {
	apiKey    string
	apiSecret string
}

func NewBasicAuth(apiKey, apiSecret string) (*BasicAuth, error) {
	if apiKey == "" || apiSecret == "" {
		return nil, invalidArgument("API key and secret must not be empty")
	}
	return &BasicAuth{apiKey: apiKey, apiSecret: apiSecret}, nil
}

func (a BasicAuth) Authenticate(req *http.Request) error {
	req.SetBasicAuth(a.apiKey, a.apiSecret)
	return nil
}

func (a BasicAuth) String() string {
	return "BasicAuth{APIKey: " + a.apiKey + ", APISecret: [REDACTED]}"
}

func (a BasicAuth) GoString() string {
	return a.String()
}
