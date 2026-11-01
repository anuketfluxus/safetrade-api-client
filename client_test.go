package safetrade_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/anuketfluxus/safetrade-api-client"
)

func TestNewClientRejectsInvalidOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		option  safetrade.Option
		wantErr string
	}{
		{name: "relative base URL", option: safetrade.WithBaseURL("safe.trade/api/v2"), wantErr: "scheme must be http or https"},
		{name: "base URL with query", option: safetrade.WithBaseURL("https://safe.trade/api/v2?x=1"), wantErr: "query and fragment are not allowed"},
		{name: "nil HTTP client", option: safetrade.WithHTTPClient(nil), wantErr: "client is nil"},
		{name: "nil authenticator", option: safetrade.WithAuthenticator(nil), wantErr: "authenticator is nil"},
		{name: "empty API key", option: safetrade.WithAPIKey("", "secret"), wantErr: "API key and secret must not be empty"},
	}
	for _, tt := range tests {
		client, err := safetrade.NewClient(tt.option)
		if err == nil {
			t.Errorf("%s: NewClient() = %v, want error", tt.name, client)
			continue
		}
		if !strings.Contains(err.Error(), tt.wantErr) {
			t.Errorf("%s: NewClient() error = %q, want it to contain %q", tt.name, err, tt.wantErr)
		}
	}
}

func TestNewClientWithoutOptions(t *testing.T) {
	t.Parallel()

	client, err := safetrade.NewClient()
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if client.Public == nil || client.Account == nil || client.Market == nil {
		t.Fatal("NewClient() left a service unset")
	}
}

func TestPrivateEndpointWithoutCredentialsSendsNoRequest(t *testing.T) {
	t.Parallel()

	api := newFakeAPI(t)
	client, err := safetrade.NewClient(safetrade.WithBaseURL(api.baseURL()))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = client.Account.GetMember(context.Background())
	if !errors.Is(err, safetrade.ErrMissingCredentials) {
		t.Fatalf("GetMember() error = %v, want ErrMissingCredentials", err)
	}
	if want := "safetrade: GET /trade/account/members/me: credentials required"; !strings.HasPrefix(err.Error(), want) {
		t.Errorf("error = %q, want prefix %q", err, want)
	}
	if got := api.requestCount(); got != 0 {
		t.Errorf("the API received %d requests, want 0", got)
	}

	api.respond(http.StatusOK, `[]`)
	if _, err := client.Public.ListMarkets(context.Background()); err != nil {
		t.Errorf("ListMarkets() error = %v", err)
	}
}

func TestUserAgent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		option safetrade.Option
		want   string
	}{
		{name: "custom", option: safetrade.WithUserAgent("my-bot/2.0"), want: "my-bot/2.0"},
		{name: "empty omits the header", option: safetrade.WithUserAgent(""), want: ""},
	}
	for _, tt := range tests {
		api := newFakeAPI(t)
		api.respond(http.StatusOK, `[]`)
		client := api.newClient(t, tt.option)
		if _, err := client.Public.ListMarkets(context.Background()); err != nil {
			t.Fatalf("%s: ListMarkets() error = %v", tt.name, err)
		}
		if got := api.lastRequest(t).header.Get("User-Agent"); got != tt.want {
			t.Errorf("%s: User-Agent = %q, want %q", tt.name, got, tt.want)
		}
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestWithHTTPClient(t *testing.T) {
	t.Parallel()

	api := newFakeAPI(t)
	api.respond(http.StatusOK, `[]`)
	var calls atomic.Int32
	httpClient := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		return http.DefaultTransport.RoundTrip(req)
	})}
	client := api.newClient(t, safetrade.WithHTTPClient(httpClient))

	if _, err := client.Public.ListMarkets(context.Background()); err != nil {
		t.Fatalf("ListMarkets() error = %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("the custom HTTP client sent %d requests, want 1", got)
	}
}

type headerAuthenticator struct {
	err error
}

func (a headerAuthenticator) Authenticate(req *http.Request) error {
	if a.err != nil {
		return a.err
	}
	req.Header.Set("X-Custom-Auth", "token")
	return nil
}

func TestWithAuthenticator(t *testing.T) {
	t.Parallel()

	api := newFakeAPI(t)
	api.respond(http.StatusOK, `{"uid":"ID1"}`)
	client := api.newClient(t, safetrade.WithAuthenticator(headerAuthenticator{}))

	if _, err := client.Account.GetMember(context.Background()); err != nil {
		t.Fatalf("GetMember() error = %v", err)
	}
	header := api.lastRequest(t).header
	if got := header.Get("X-Custom-Auth"); got != "token" {
		t.Errorf("X-Custom-Auth = %q, want token", got)
	}
	if got := header.Get("X-Auth-Apikey"); got != "" {
		t.Errorf("X-Auth-Apikey = %q, want none: the last authentication option wins", got)
	}
}

func TestAuthenticatorErrorSendsNoRequest(t *testing.T) {
	t.Parallel()

	api := newFakeAPI(t)
	authErr := errors.New("vault unavailable")
	client := api.newClient(t, safetrade.WithAuthenticator(headerAuthenticator{err: authErr}))

	_, err := client.Account.GetMember(context.Background())
	if !errors.Is(err, authErr) {
		t.Fatalf("GetMember() error = %v, want it to wrap %v", err, authErr)
	}
	if got := api.requestCount(); got != 0 {
		t.Errorf("the API received %d requests, want 0", got)
	}
}

func TestBasicAuthClient(t *testing.T) {
	t.Parallel()

	api := newFakeAPI(t)
	api.respond(http.StatusOK, `{"uid":"ID1"}`)
	auth, err := safetrade.NewBasicAuth("basic-key", "basic-secret")
	if err != nil {
		t.Fatalf("NewBasicAuth() error = %v", err)
	}
	client := api.newClient(t, safetrade.WithAuthenticator(auth))

	if _, err := client.Account.GetMember(context.Background()); err != nil {
		t.Fatalf("GetMember() error = %v", err)
	}
	if got, want := api.lastRequest(t).header.Get("Authorization"), "Basic YmFzaWMta2V5OmJhc2ljLXNlY3JldA=="; got != want {
		t.Errorf("Authorization = %q, want %q", got, want)
	}
}

func TestAPIErrorResponses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		status       int
		body         string
		sentinel     error
		wantMessage  string
		wantMessages []string
	}{
		{
			name:         "validation error",
			status:       http.StatusUnprocessableEntity,
			body:         `{"errors":["market.order.invalid_volume"]}`,
			sentinel:     safetrade.ErrUnprocessableEntity,
			wantMessage:  "safetrade: POST /trade/market/orders: 422 Unprocessable Entity: market.order.invalid_volume",
			wantMessages: []string{"market.order.invalid_volume"},
		},
		{
			name:        "Cloudflare challenge",
			status:      http.StatusForbidden,
			body:        "<!DOCTYPE html>\n<html><head><title>Just a moment...</title></head></html>",
			sentinel:    safetrade.ErrForbidden,
			wantMessage: "safetrade: POST /trade/market/orders: 403 Forbidden: <!DOCTYPE html> <html><head><title>Just a moment...</title></head></html>",
		},
		{
			name:        "server error without body",
			status:      http.StatusInternalServerError,
			body:        "",
			sentinel:    safetrade.ErrServerError,
			wantMessage: "safetrade: POST /trade/market/orders: 500 Internal Server Error",
		},
	}
	for _, tt := range tests {
		api := newFakeAPI(t)
		api.respond(tt.status, tt.body)
		client := api.newClient(t)

		_, err := client.Market.CreateOrder(context.Background(), safetrade.CreateOrderParams{
			Market: "btcusdt", Side: safetrade.SideBuy, Type: safetrade.OrderTypeMarket,
			Amount: decimal.RequireFromString("1"),
		})
		if !errors.Is(err, tt.sentinel) {
			t.Errorf("%s: error = %v, want it to match %v", tt.name, err, tt.sentinel)
		}
		var apiErr *safetrade.APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("%s: error %v is not an *APIError", tt.name, err)
		}
		if apiErr.StatusCode != tt.status {
			t.Errorf("%s: StatusCode = %d, want %d", tt.name, apiErr.StatusCode, tt.status)
		}
		if string(apiErr.Body) != tt.body {
			t.Errorf("%s: Body = %q, want %q", tt.name, apiErr.Body, tt.body)
		}
		if got := apiErr.Messages(); strings.Join(got, ",") != strings.Join(tt.wantMessages, ",") {
			t.Errorf("%s: Messages() = %v, want %v", tt.name, got, tt.wantMessages)
		}
		if err.Error() != tt.wantMessage {
			t.Errorf("%s: Error() = %q, want %q", tt.name, err, tt.wantMessage)
		}
	}
}

func TestAPIErrorBodyIsTruncated(t *testing.T) {
	t.Parallel()

	api := newFakeAPI(t)
	api.respond(http.StatusBadGateway, strings.Repeat("x", 100<<10))
	client := api.newClient(t)

	_, err := client.Public.ListMarkets(context.Background())
	var apiErr *safetrade.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %v is not an *APIError", err)
	}
	if got, want := len(apiErr.Body), 64<<10; got != want {
		t.Errorf("len(Body) = %d, want %d", got, want)
	}
}

func TestMalformedSuccessResponse(t *testing.T) {
	t.Parallel()

	api := newFakeAPI(t)
	api.respond(http.StatusOK, `<html>maintenance</html>`)
	client := api.newClient(t)

	markets, err := client.Public.ListMarkets(context.Background())
	if err == nil {
		t.Fatalf("ListMarkets() = %v, want error", markets)
	}
	if want := "safetrade: GET /trade/public/markets: decoding 200 response:"; !strings.HasPrefix(err.Error(), want) {
		t.Errorf("error = %q, want prefix %q", err, want)
	}
	if markets != nil {
		t.Errorf("ListMarkets() returned %v alongside an error, want nil", markets)
	}
}

func TestDecodeErrorKeepsTheResponseOfAnExecutedRequest(t *testing.T) {
	t.Parallel()

	api := newFakeAPI(t)
	body := `{"id":42,"market":"btcusdt","state":"wait","price":""}`
	api.respond(http.StatusCreated, body)
	client := api.newClient(t)

	_, err := client.Market.CreateOrder(context.Background(), safetrade.CreateOrderParams{
		Market: "btcusdt", Side: safetrade.SideBuy, Type: safetrade.OrderTypeLimit,
		Amount: decimal.RequireFromString("1"), Price: decimal.RequireFromString("67000"),
	})
	var decodeErr *safetrade.DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("CreateOrder() error = %v, want a *DecodeError", err)
	}
	if decodeErr.StatusCode != http.StatusCreated || string(decodeErr.Body) != body {
		t.Errorf("DecodeError = %d %s, want the 201 response and its body", decodeErr.StatusCode, decodeErr.Body)
	}
	var apiErr *safetrade.APIError
	if errors.As(err, &apiErr) {
		t.Errorf("CreateOrder() error = %v also matches *APIError, which reports a response outside the 2xx range", err)
	}
}

func TestTrailingDataAfterTheResponseIsADecodeError(t *testing.T) {
	t.Parallel()

	api := newFakeAPI(t)
	api.respond(http.StatusOK, `[] {"extra":true}`)
	client := api.newClient(t)

	_, err := client.Public.ListMarkets(context.Background())
	var decodeErr *safetrade.DecodeError
	if !errors.As(err, &decodeErr) || !strings.Contains(err.Error(), "unexpected data after the JSON value") {
		t.Errorf("ListMarkets() error = %v, want a *DecodeError about trailing data", err)
	}
}

func TestOptionNumbersKeepTheirDigits(t *testing.T) {
	t.Parallel()

	api := newFakeAPI(t)
	api.respond(http.StatusOK, `[{"id":5,"options":{"memo":18446744073709551557,"tag":"x"}}]`)
	client := api.newClient(t)

	beneficiaries, err := client.Account.ListBeneficiaries(context.Background(), safetrade.ListBeneficiariesParams{})
	if err != nil {
		t.Fatalf("ListBeneficiaries() error = %v", err)
	}
	if got, want := beneficiaries[0].Options["memo"], json.Number("18446744073709551557"); got != want {
		t.Errorf("Options[memo] = %#v, want %#v", got, want)
	}
	if got := beneficiaries[0].Options["tag"]; got != "x" {
		t.Errorf("Options[tag] = %#v, want \"x\"", got)
	}
}

func redirectingServer(t *testing.T, target string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target+r.URL.RequestURI(), http.StatusTemporaryRedirect)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestAuthenticatedRequestsDoNotFollowRedirects(t *testing.T) {
	t.Parallel()

	api := newFakeAPI(t)
	api.respond(http.StatusOK, `{"uid":"ID1"}`)
	redirector := redirectingServer(t, api.server.URL)
	client, err := safetrade.NewClient(
		safetrade.WithBaseURL(redirector.URL+"/api/v2"),
		safetrade.WithAPIKey(testAPIKey, testAPISecret),
	)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = client.Account.GetMember(context.Background())
	if err == nil || !strings.Contains(err.Error(), "refusing to follow the redirect") {
		t.Fatalf("GetMember() error = %v, want a refused redirect", err)
	}
	if got := api.requestCount(); got != 0 {
		t.Errorf("the redirect target received %d requests carrying credentials, want 0", got)
	}

	err = client.Market.CancelOrders(context.Background(), safetrade.CancelOrdersParams{Market: "btcusdt"})
	var apiErr *safetrade.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusTemporaryRedirect {
		t.Errorf("CancelOrders() error = %v, want the 307 response", err)
	}
	if got := api.requestCount(); got != 0 {
		t.Errorf("the redirect target received %d requests carrying credentials, want 0", got)
	}

	api.respond(http.StatusOK, `[]`)
	if _, err := client.Public.ListMarkets(context.Background()); err != nil {
		t.Errorf("ListMarkets() through a redirect error = %v", err)
	}
	if got := api.requestCount(); got != 1 {
		t.Errorf("the redirect target received %d requests, want 1", got)
	}
}

func TestRequestBodiesCannotBeRewound(t *testing.T) {
	t.Parallel()

	api := newFakeAPI(t)
	api.respond(http.StatusCreated, `{"id":1}`)
	var rewindable, sent atomic.Int32
	httpClient := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		sent.Add(1)
		if req.GetBody != nil {
			rewindable.Add(1)
		}
		return http.DefaultTransport.RoundTrip(req)
	})}
	client := api.newClient(t, safetrade.WithHTTPClient(httpClient))
	ctx := context.Background()
	one := decimal.RequireFromString("1")

	_, err := client.Market.CreateOrder(ctx, safetrade.CreateOrderParams{
		Market: "btcusdt", Side: safetrade.SideBuy, Type: safetrade.OrderTypeLimit, Amount: one, Price: one,
	})
	if err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}
	if _, err := client.Account.CreateWithdrawal(ctx, safetrade.CreateWithdrawalParams{Currency: "usdt", Amount: one}); err != nil {
		t.Fatalf("CreateWithdrawal() error = %v", err)
	}
	if got := sent.Load(); got != 2 {
		t.Fatalf("the transport sent %d requests, want 2", got)
	}
	if got := rewindable.Load(); got != 0 {
		t.Errorf("%d requests had a rewindable body, want none", got)
	}
}

func TestCreateOrderValidatesTheFieldsAgainstTheType(t *testing.T) {
	t.Parallel()

	api := newFakeAPI(t)
	client := api.newClient(t)
	d := decimal.RequireFromString
	valid := safetrade.CreateOrderParams{Market: "btcusdt", Side: safetrade.SideBuy}

	tests := []struct {
		name    string
		modify  func(*safetrade.CreateOrderParams)
		wantErr string
	}{
		{name: "no market", modify: func(p *safetrade.CreateOrderParams) { p.Market = "" }, wantErr: "market must not be empty"},
		{name: "no side", modify: func(p *safetrade.CreateOrderParams) { p.Side = "" }, wantErr: "side must not be empty"},
		{
			name: "mis-cased side",
			modify: func(p *safetrade.CreateOrderParams) {
				p.Side, p.Type, p.Amount = "Buy", safetrade.OrderTypeMarket, d("1")
			},
			wantErr: `side must be buy or sell, got "Buy"`,
		},
		{name: "no type", modify: func(*safetrade.CreateOrderParams) {}, wantErr: "type must not be empty"},
		{

			name:    "mis-cased limit type without a price",
			modify:  func(p *safetrade.CreateOrderParams) { p.Type, p.Amount = "Limit", d("1") },
			wantErr: `unknown order type "Limit", want limit, stop_limit, market, or market_quote`,
		},
		{
			name:    "limit type with a trailing space",
			modify:  func(p *safetrade.CreateOrderParams) { p.Type, p.Amount, p.Price = "limit ", d("1"), d("2") },
			wantErr: `unknown order type "limit "`,
		},
		{
			name:    "undocumented type",
			modify:  func(p *safetrade.CreateOrderParams) { p.Type, p.Amount = "iceberg", d("1") },
			wantErr: `unknown order type "iceberg"`,
		},
		{
			name: "limit with a price rounded to zero",
			modify: func(p *safetrade.CreateOrderParams) {
				p.Type, p.Amount, p.Price = safetrade.OrderTypeLimit, d("1000000"), d("0.004").Round(2)
			},
			wantErr: "a limit order needs a positive price",
		},
		{
			name:    "limit without amount",
			modify:  func(p *safetrade.CreateOrderParams) { p.Type, p.Price = safetrade.OrderTypeLimit, d("1") },
			wantErr: "a limit order needs a positive amount",
		},
		{
			name: "limit with a stop price",
			modify: func(p *safetrade.CreateOrderParams) {
				p.Type, p.Amount, p.Price, p.StopPrice = safetrade.OrderTypeLimit, d("1"), d("2"), d("3")
			},
			wantErr: "a limit order must not set stop price",
		},
		{
			name: "limit with a total",
			modify: func(p *safetrade.CreateOrderParams) {
				p.Type, p.Amount, p.Price, p.Total = safetrade.OrderTypeLimit, d("1"), d("2"), d("2")
			},
			wantErr: "a limit order must not set total",
		},
		{
			name: "stop-limit without a stop price",
			modify: func(p *safetrade.CreateOrderParams) {
				p.Type, p.Amount, p.Price = safetrade.OrderTypeStopLimit, d("1"), d("2")
			},
			wantErr: "a stop_limit order needs a positive stop price",
		},
		{
			name: "market with a price",
			modify: func(p *safetrade.CreateOrderParams) {
				p.Type, p.Amount, p.Price = safetrade.OrderTypeMarket, d("1"), d("2")
			},
			wantErr: "a market order must not set price",
		},
		{
			name: "market quote with an amount",
			modify: func(p *safetrade.CreateOrderParams) {
				p.Type, p.Total, p.Amount = safetrade.OrderTypeMarketQuote, d("100"), d("1")
			},
			wantErr: "a market_quote order must not set amount",
		},
		{
			name: "negative amount",
			modify: func(p *safetrade.CreateOrderParams) {
				p.Type, p.Amount = safetrade.OrderTypeMarket, d("-1")
			},
			wantErr: "amount must not be negative",
		},
		{
			name: "negative max price",
			modify: func(p *safetrade.CreateOrderParams) {
				p.Type, p.Amount, p.MaxPrice = safetrade.OrderTypeMarket, d("1"), d("-5")
			},
			wantErr: "max price must not be negative",
		},
	}
	for _, tt := range tests {
		params := valid
		tt.modify(&params)
		_, err := client.Market.CreateOrder(context.Background(), params)
		if !errors.Is(err, safetrade.ErrInvalidArgument) || !strings.Contains(err.Error(), tt.wantErr) {
			t.Errorf("%s: CreateOrder() error = %v, want ErrInvalidArgument saying %q", tt.name, err, tt.wantErr)
		}
	}
	if got := api.requestCount(); got != 0 {
		t.Errorf("the API received %d requests, want 0", got)
	}
}

func TestMayHaveTakenEffect(t *testing.T) {
	t.Parallel()

	order := safetrade.CreateOrderParams{
		Market: "btcusdt", Side: safetrade.SideBuy, Type: safetrade.OrderTypeLimit,
		Amount: decimal.RequireFromString("1"), Price: decimal.RequireFromString("67000"),
	}

	answered := func(status int, body string) func(*testing.T) error {
		return func(t *testing.T) error {
			api := newFakeAPI(t)
			api.respond(status, body)
			_, err := api.newClient(t).Market.CreateOrder(context.Background(), order)
			return err
		}
	}

	unsent := func(ctx context.Context, params safetrade.CreateOrderParams, opts ...safetrade.Option) func(*testing.T) error {
		return func(t *testing.T) error {
			api := newFakeAPI(t)
			client, err := safetrade.NewClient(append([]safetrade.Option{safetrade.WithBaseURL(api.baseURL())}, opts...)...)
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			_, err = client.Market.CreateOrder(ctx, params)
			if got := api.requestCount(); got != 0 {
				t.Errorf("the API received %d requests, want 0", got)
			}
			return err
		}
	}
	withCredentials := safetrade.WithAPIKey(testAPIKey, testAPISecret)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	zeroPrice := order
	zeroPrice.Price = decimal.Zero

	tests := []struct {
		name string
		err  func(*testing.T) error
		want bool
	}{
		{name: "success", err: answered(http.StatusCreated, `{"id":1}`), want: true},
		{name: "refused", err: answered(http.StatusUnprocessableEntity, `{"errors":["market.order.invalid_volume"]}`), want: false},
		{name: "rate limited", err: answered(http.StatusTooManyRequests, ""), want: false},
		{name: "internal server error", err: answered(http.StatusInternalServerError, ""), want: true},
		{name: "gateway timeout", err: answered(http.StatusGatewayTimeout, "<html>Gateway time-out</html>"), want: true},
		{name: "unreadable success", err: answered(http.StatusCreated, `{"id":`), want: true},
		{name: "invalid argument", err: unsent(context.Background(), zeroPrice, withCredentials), want: false},
		{name: "missing credentials", err: unsent(context.Background(), order), want: false},
		{
			name: "failed authentication",
			err:  unsent(context.Background(), order, safetrade.WithAuthenticator(headerAuthenticator{err: errors.New("vault sealed")})),
			want: false,
		},
		{
			name: "context done before the call",
			err: func(t *testing.T) error {
				err := unsent(canceled, order, withCredentials)(t)
				if !errors.Is(err, context.Canceled) {
					t.Errorf("CreateOrder() error = %v, want context.Canceled", err)
				}
				return err
			},
			want: false,
		},
		{name: "timeout in flight", err: orderTimingOut(order), want: true},
		{name: "redirected", err: orderRedirected(order), want: true},
		{name: "unrelated error", err: func(*testing.T) error { return errors.New("unrelated") }, want: true},
	}
	for _, tt := range tests {
		err := tt.err(t)
		if got := safetrade.MayHaveTakenEffect(err); got != tt.want {
			t.Errorf("%s: MayHaveTakenEffect(%v) = %t, want %t", tt.name, err, got, tt.want)
		}
	}
}

func orderTimingOut(order safetrade.CreateOrderParams) func(*testing.T) error {
	return func(t *testing.T) error {
		release := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-release:
			}
		}))
		t.Cleanup(server.Close)
		t.Cleanup(func() { close(release) })

		client, err := safetrade.NewClient(safetrade.WithBaseURL(server.URL), safetrade.WithAPIKey(testAPIKey, testAPISecret))
		if err != nil {
			t.Fatalf("NewClient() error = %v", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		_, err = client.Market.CreateOrder(ctx, order)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("CreateOrder() error = %v, want context.DeadlineExceeded", err)
		}
		return err
	}
}

func orderRedirected(order safetrade.CreateOrderParams) func(*testing.T) error {
	return func(t *testing.T) error {
		api := newFakeAPI(t)
		redirector := redirectingServer(t, api.server.URL)
		client, err := safetrade.NewClient(safetrade.WithBaseURL(redirector.URL+"/api/v2"), safetrade.WithAPIKey(testAPIKey, testAPISecret))
		if err != nil {
			t.Fatalf("NewClient() error = %v", err)
		}
		_, err = client.Market.CreateOrder(context.Background(), order)
		var apiErr *safetrade.APIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusTemporaryRedirect {
			t.Errorf("CreateOrder() error = %v, want the 307 response", err)
		}
		return err
	}
}

func TestContextCancellation(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })

	client, err := safetrade.NewClient(safetrade.WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = client.Public.ListMarkets(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ListMarkets() error = %v, want context.DeadlineExceeded", err)
	}
}

func TestInvalidArgumentsSendNoRequest(t *testing.T) {
	t.Parallel()

	api := newFakeAPI(t)
	client := api.newClient(t)
	ctx := context.Background()
	one := decimal.RequireFromString("1")

	calls := map[string]func() error{
		"GetCurrency empty":       func() error { _, err := client.Public.GetCurrency(ctx, ""); return err },
		"GetCurrency dot":         func() error { _, err := client.Public.GetCurrency(ctx, "."); return err },
		"GetPriceHistory dot-dot": func() error { _, err := client.Public.GetPriceHistory(ctx, "..", ""); return err },
		"GetMarket empty":         func() error { _, err := client.Public.GetMarket(ctx, ""); return err },
		"GetDepth empty":          func() error { _, err := client.Public.GetDepth(ctx, "", 0); return err },
		"ListKLines empty":        func() error { _, err := client.Public.ListKLines(ctx, "", safetrade.KLineParams{}); return err },
		"ListMarketTrades empty": func() error {
			_, err := client.Public.ListMarketTrades(ctx, "", safetrade.ListMarketTradesParams{})
			return err
		},
		"GetOrderDistribution no period": func() error { _, err := client.Public.GetOrderDistribution(ctx, ""); return err },
		"GetTicker empty":                func() error { _, err := client.Public.GetTicker(ctx, ""); return err },
		"GetTradeDistribution no period": func() error { _, err := client.Public.GetTradeDistribution(ctx, "", "btcusdt"); return err },
		"GetDepositAddress no currency":  func() error { _, err := client.Account.GetDepositAddress(ctx, "", "tron"); return err },
		"GetDepositAddress no network":   func() error { _, err := client.Account.GetDepositAddress(ctx, "usdt", ""); return err },
		"GetDeposit empty":               func() error { _, err := client.Account.GetDeposit(ctx, ""); return err },
		"CreateWithdrawal no currency": func() error {
			_, err := client.Account.CreateWithdrawal(ctx, safetrade.CreateWithdrawalParams{Amount: one})
			return err
		},
		"CreateWithdrawal no amount": func() error {
			_, err := client.Account.CreateWithdrawal(ctx, safetrade.CreateWithdrawalParams{Currency: "usdt"})
			return err
		},
		"CreateWithdrawal negative amount": func() error {
			_, err := client.Account.CreateWithdrawal(ctx, safetrade.CreateWithdrawalParams{
				Currency: "usdt", Amount: decimal.RequireFromString("-1"),
			})
			return err
		},
		"GenerateWithdrawalCode no currency": func() error {
			return client.Account.GenerateWithdrawalCode(ctx, safetrade.GenerateWithdrawalCodeParams{
				BlockchainKey: "tron", Address: "T", Amount: one, Channel: safetrade.CodeChannelEmail,
			})
		},
		"GenerateWithdrawalCode no blockchain key": func() error {
			return client.Account.GenerateWithdrawalCode(ctx, safetrade.GenerateWithdrawalCodeParams{
				Currency: "usdt", Address: "T", Amount: one, Channel: safetrade.CodeChannelEmail,
			})
		},
		"GenerateWithdrawalCode no address": func() error {
			return client.Account.GenerateWithdrawalCode(ctx, safetrade.GenerateWithdrawalCodeParams{
				Currency: "usdt", BlockchainKey: "tron", Amount: one, Channel: safetrade.CodeChannelEmail,
			})
		},
		"GenerateWithdrawalCode no channel": func() error {
			return client.Account.GenerateWithdrawalCode(ctx, safetrade.GenerateWithdrawalCodeParams{
				Currency: "usdt", BlockchainKey: "tron", Address: "T", Amount: one,
			})
		},
		"GenerateWithdrawalCode no amount": func() error {
			return client.Account.GenerateWithdrawalCode(ctx, safetrade.GenerateWithdrawalCodeParams{
				Currency: "usdt", BlockchainKey: "tron", Address: "T", Channel: safetrade.CodeChannelEmail,
			})
		},
		"GetOrder zero":     func() error { _, err := client.Market.GetOrder(ctx, 0); return err },
		"CancelOrder minus": func() error { _, err := client.Market.CancelOrder(ctx, -1); return err },
		"GetTrade zero":     func() error { _, err := client.Market.GetTrade(ctx, 0); return err },
		"CancelOrders no market": func() error {
			return client.Market.CancelOrders(ctx, safetrade.CancelOrdersParams{})
		},
		"CancelOrders unknown side": func() error {
			return client.Market.CancelOrders(ctx, safetrade.CancelOrdersParams{Market: "btcusdt", Side: "bid"})
		},
	}
	for name, call := range calls {
		if err := call(); !errors.Is(err, safetrade.ErrInvalidArgument) {
			t.Errorf("%s: error = %v, want ErrInvalidArgument", name, err)
		}
	}
	if got := api.requestCount(); got != 0 {
		t.Errorf("the API received %d requests, want 0", got)
	}
}
