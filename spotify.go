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
	"net/http"
	"strconv"
	"time"
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

	// rateLimitExceededStatusCode is the code that the server returns when our
	// request frequency is too high.
	rateLimitExceededStatusCode = 429
)

// Client is a client for working with the Spotify Web API.
// It is best to create this using spotify.New()
type Client struct {
	http    *http.Client
	baseURL string

	autoRetry      bool
	acceptLanguage string
}

type ClientOption func(client *Client)

// WithHTTPClient provides a http client to be used when making requests to the Spotify API
func WithHTTPClient(c *http.Client) ClientOption {
	return func(client *Client) {
		client.http = c
	}
}

// WithRetry configures the Spotify API client to automatically retry requests that fail due to ratelimiting.
func WithRetry(shouldRetry bool) ClientOption {
	return func(client *Client) {
		client.autoRetry = shouldRetry
	}
}

// WithBaseURL provides an alternative base url to use for requests to the Spotify API. This can be used to connect to a
// staging or other alternative environment.
func WithBaseURL(url string) ClientOption {
	return func(client *Client) {
		client.baseURL = url
	}
}

// WithAcceptLanguage configures the client to provide the accept language header on all requests.
func WithAcceptLanguage(lang string) ClientOption {
	return func(client *Client) {
		client.acceptLanguage = lang
	}
}

// New returns a client for working with the Spotify Web API.
// You must provide some form of a HTTP client with WithHTTPClient that attaches authorisation
// to the requests or requests will fail. You may use the built-in auth client to generate one.
func New(opts ...ClientOption) *Client {
	c := &Client{
		http:    http.DefaultClient,
		baseURL: "https://api.spotify.com/v1/",
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
	resp, err := http.Get(i.URL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// TODO: get Content-Type from header?
	if resp.StatusCode != http.StatusOK {
		return errors.New("Couldn't download image - HTTP" + strconv.Itoa(resp.StatusCode))
	}
	_, err = io.Copy(dst, resp.Body)
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

// decodeError decodes an Error from an io.Reader.
func (c *Client) decodeError(resp *http.Response) error {
	responseBody, err := ioutil.ReadAll(resp.Body)
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

// shouldRetry determines whether the status code indicates that the
// previous operation should be retried at a later time
func shouldRetry(status int) bool {
	return status == http.StatusAccepted || status == http.StatusTooManyRequests
}

// isFailure determines whether the code indicates failure
func isFailure(code int, validCodes []int) bool {
	for _, item := range validCodes {
		if item == code {
			return false
		}
	}
	return true
}

// `execute` executes a non-GET request. `needsStatus` describes other HTTP
// status codes that will be treated as success. Note that we allow all 200s
// even if there are additional success codes that represent success.
func (c *Client) execute(req *http.Request, result interface{}, needsStatus ...int) error {
	if c.acceptLanguage != "" {
		req.Header.Set("Accept-Language", c.acceptLanguage)
	}
	for {
		resp, err := c.http.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if c.autoRetry && shouldRetry(resp.StatusCode) {
			time.Sleep(retryDuration(resp))
			continue
		}
		if resp.StatusCode == http.StatusNoContent {
			return nil
		}
		if (resp.StatusCode >= 300 ||
			resp.StatusCode < 200) &&
			isFailure(resp.StatusCode, needsStatus) {
			return c.decodeError(resp)
		}

		if result != nil {
			if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
				return err
			}
		}
		break
	}
	return nil
}

func retryDuration(resp *http.Response) time.Duration {
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

func (c *Client) get(ctx context.Context, url string, result interface{}) error {
	for {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if c.acceptLanguage != "" {
			req.Header.Set("Accept-Language", c.acceptLanguage)
		}
		if err != nil {
			return err
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return err
		}

		defer resp.Body.Close()

		if resp.StatusCode == rateLimitExceededStatusCode && c.autoRetry {
			time.Sleep(retryDuration(resp))
			continue
		}
		if resp.StatusCode == http.StatusNoContent {
			return nil
		}
		if resp.StatusCode != http.StatusOK {
			return c.decodeError(resp)
		}

		err = json.NewDecoder(resp.Body).Decode(result)
		if err != nil {
			return err
		}

		break
	}

	return nil
}

// NewReleases gets a list of new album releases featured in Spotify.
// Supported options: Country, Limit, Offset
func (c *Client) NewReleases(ctx context.Context, opts ...RequestOption) (albums *SimpleAlbumPage, err error) {
	spotifyURL := c.baseURL + "browse/new-releases"
	if params := processOptions(opts...).urlParams.Encode(); params != "" {
		spotifyURL += "?" + params
	}

	var objmap map[string]*json.RawMessage
	err = c.get(ctx, spotifyURL, &objmap)
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
