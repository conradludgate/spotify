package spotify

import (
	"context"

	"github.com/conradludgate/go-http"
)

// Category is used by Spotify to tag items in.  For example, on the Spotify
// player's "Browse" tab.
type Category struct {
	// A link to the Web API endpoint returning full details of the category
	Endpoint string `json:"href"`
	// The category icon, in various sizes
	Icons []Image `json:"icons"`
	// The Spotify category ID.  This isn't a base-62 Spotify ID, its just
	// a short string that describes and identifies the category (ie "party").
	ID string `json:"id"`
	// The name of the category
	Name string `json:"name"`
}

// GetCategory gets a single category used to tag items in Spotify.
//
// Supported options: Country, Locale
func (c *Client) GetCategory(ctx context.Context, id string, opts ...RequestOption) (Category, error) {
	cat := Category{}

	_, err := c.http.Get(
		http.Path("browse", "categories", id),
		http.Params(processOptions(opts...).urlParams),
	).Send(ctx, http.JSON(&cat))
	return cat, err
}

// GetCategoryPlaylists gets a list of Spotify playlists tagged with a particular category.
// Supported options: Country, Limit, Offset
func (c *Client) GetCategoryPlaylists(ctx context.Context, catID string, opts ...RequestOption) (*SimplePlaylistPage, error) {
	wrapper := struct {
		Playlists SimplePlaylistPage `json:"playlists"`
	}{}

	_, err := c.http.Get(
		http.Path("browse", "categories", catID, "playlists"),
		http.Params(processOptions(opts...).urlParams),
	).Send(ctx, http.JSON(&wrapper))
	if err != nil {
		return nil, err
	}

	return &wrapper.Playlists, nil
}

// GetCategories gets a list of categories used to tag items in Spotify
//
// Supported options: Country, Locale, Limit, Offset
func (c *Client) GetCategories(ctx context.Context, opts ...RequestOption) (*CategoryPage, error) {
	wrapper := struct {
		Categories CategoryPage `json:"categories"`
	}{}

	_, err := c.http.Get(
		http.Path("browse", "categories"),
		http.Params(processOptions(opts...).urlParams),
	).Send(ctx, http.JSON(&wrapper))
	if err != nil {
		return nil, err
	}

	return &wrapper.Categories, nil
}
