// Package spotify provides utilties for interfacing
// with Spotify's Web API.
package spotify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	stdhttp "net/http"
	"strconv"
	"time"

	"github.com/conradludgate/go-http"

	"golang.org/x/oauth2"
)

// Version is the version of this library.
const Version = "1.0.0"

const (
	// DateLayout can be used with time.Parse to create time.Time values
	// from Spotify date strings.  For example, PrivateUser.Birthdate
	// uses this format.
	DateLayout = "2006-01-02"
	// TimestampLayout can be used with time.Parse to create time.Time
	// values from SpotifyTimestamp strings.  It is an ISO 8601 UTC timestamp
	// with a zero offset.  For example, PlaylistTrack's AddedAt field uses
	// this format.
	TimestampLayout = "2006-01-02T15:04:05Z"

	// defaultRetryDurationS helps us fix an apparent server bug whereby we will
	// be told to retry but not be given a wait-interval.
	defaultRetryDuration = time.Second * 5
)

// Client is a client for working with the Spotify Web API.
// It is best to create this using spotify.New()
type Client struct {
	http *http.Client
}

type ClientOption func(client *Client)

type retryTransport struct {
	Base stdhttp.RoundTripper
}

func (r retryTransport) RoundTrip(req *stdhttp.Request) (*stdhttp.Response, error) {
	for {
		resp, err := r.Base.RoundTrip(req)
		if err == nil && resp.StatusCode == stdhttp.StatusTooManyRequests {
			time.Sleep(retryDuration(resp))
			continue
		}

		return resp, err
	}
}

// WithRetry configures the Spotify API client to automatically retry requests that fail due to ratelimiting.
func WithRetry() ClientOption {
	return func(client *Client) {
		baseClient := client.http.BaseClient()
		transport := baseClient.Transport
		if transport == nil {
			transport = stdhttp.DefaultTransport
		}
		baseClient.Transport = retryTransport{transport}
		client.http.Apply(http.BaseClient(baseClient))
	}
}

// WithBaseURL provides an alternative base url to use for requests to the Spotify API. This can be used to connect to a
// staging or other alternative environment.
func WithBaseURL(url string) ClientOption {
	return func(client *Client) {
		client.http.Apply(http.URLString(url))
	}
}

// WithAcceptLanguage configures the client to provide the accept language header on all requests.
func WithAcceptLanguage(lang string) ClientOption {
	return func(client *Client) {
		client.http.Apply(http.AddHeader("Accept-Language", lang))
	}
}

// New returns a client for working with the Spotify Web API.
// The provided httpClient must provide Authentication with the requests.
// The auth package may be used to generate a suitable client.
func New(httpClient *stdhttp.Client, opts ...ClientOption) *Client {
	c := &Client{
		http: http.NewClient(
			http.URLString("https://api.spotify.com/v1/"),
			http.BaseClient(httpClient),
			http.PreResponseMiddlewares(errorDecoder{}),
		),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// URI identifies an artist, album, track, or category.  For example,
// spotify:track:6rqhFgbbKwnb9MLmUQDhG6
type URI string

// ID is a base-62 identifier for an artist, track, album, etc.
// It can be found at the end of a spotify.URI.
type ID string

func (id *ID) String() string {
	return string(*id)
}

// Followers contains information about the number of people following a
// particular artist or playlist.
type Followers struct {
	// The total number of followers.
	Count uint `json:"total"`
	// A link to the Web API endpoint providing full details of the followers,
	// or the empty string if this data is not available.
	Endpoint string `json:"href"`
}

// Image identifies an image associated with an item.
type Image struct {
	// The image height, in pixels.
	Height int `json:"height"`
	// The image width, in pixels.
	Width int `json:"width"`
	// The source URL of the image.
	URL string `json:"url"`
}

// Download downloads the image and writes its data to the specified io.Writer.
func (i Image) Download(dst io.Writer) error {
	_, err := http.NewClient().Get(http.URLString(i.URL)).Send(context.Background(), http.WriteBodyTo(dst))
	return err
}

// Error represents an error returned by the Spotify Web API.
type Error struct {
	// A short description of the error.
	Message string `json:"message"`
	// The HTTP status code.
	Status int `json:"status"`
}

func (e Error) Error() string {
	return e.Message
}

type errorDecoder struct{}

func (d errorDecoder) ProcessResponse(resp *http.Response) error {
	if resp.StatusCode.Type() == http.StatusTypeSuccess {
		return nil
	}

	responseBody, err := ioutil.ReadAll(resp)
	if err != nil {
		return err
	}

	if len(responseBody) == 0 {
		return fmt.Errorf("spotify: HTTP %d: %s (body empty)", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	buf := bytes.NewBuffer(responseBody)

	var e struct {
		E Error `json:"error"`
	}
	err = json.NewDecoder(buf).Decode(&e)
	if err != nil {
		return fmt.Errorf("spotify: couldn't decode error: (%d) [%s]", len(responseBody), responseBody)
	}

	if e.E.Message == "" {
		// Some errors will result in there being a useful status-code but an
		// empty message, which will confuse the user (who only has access to
		// the message and not the code). An example of this is when we send
		// some of the arguments directly in the HTTP query and the URL ends-up
		// being too long.

		e.E.Message = fmt.Sprintf("spotify: unexpected HTTP %d: %s (empty error)",
			resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	return e.E
}

func retryDuration(resp *stdhttp.Response) time.Duration {
	raw := resp.Header.Get("Retry-After")
	if raw == "" {
		return defaultRetryDuration
	}
	seconds, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return defaultRetryDuration
	}
	return time.Duration(seconds) * time.Second
}

// NewReleases gets a list of new album releases featured in Spotify.
// Supported options: Country, Limit, Offset
func (c *Client) NewReleases(ctx context.Context, opts ...RequestOption) (albums *SimpleAlbumPage, err error) {
	var objmap map[string]*json.RawMessage
	_, err = c.http.Get(
		http.Path("browse", "new-releases"),
		http.Params(processOptions(opts...).urlParams),
	).Send(ctx, http.JSON(&objmap))
	if err != nil {
		return nil, err
	}

	var result SimpleAlbumPage
	err = json.Unmarshal(*objmap["albums"], &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// Token gets the client's current token.
func (c *Client) Token() (*oauth2.Token, error) {
	transport, ok := c.http.BaseClient().Transport.(*oauth2.Transport)
	if !ok {
		return nil, errors.New("spotify: client not backed by oauth2 transport")
	}
	t, err := transport.Source.Token()
	if err != nil {
		return nil, err
	}
	return t, nil
}
