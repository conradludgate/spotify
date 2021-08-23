package spotify

import (
	"context"
	"errors"
	"strings"

	"github.com/conradludgate/go-http"
)

// UserHasTracks checks if one or more tracks are saved to the current user's
// "Your Music" library.
func (c *Client) UserHasTracks(ctx context.Context, ids ...ID) ([]bool, error) {
	return c.libraryContains(ctx, "tracks", ids...)
}

// UserHasAlbums checks if one or more albums are saved to the current user's
// "Your Albums" library.
func (c *Client) UserHasAlbums(ctx context.Context, ids ...ID) ([]bool, error) {
	return c.libraryContains(ctx, "albums", ids...)
}

func (c *Client) libraryContains(ctx context.Context, typ string, ids ...ID) ([]bool, error) {
	if l := len(ids); l == 0 || l > 50 {
		return nil, errors.New("spotify: supports 1 to 50 IDs per call")
	}
	var result []bool

	_, err := c.http.Get(
		http.Path("me", typ, "contains"),
		http.Param("ids", strings.Join(toStringSlice(ids), ",")),
	).Send(ctx, http.JSON(&result))
	if err != nil {
		return nil, err
	}

	return result, err
}

// AddTracksToLibrary saves one or more tracks to the current user's
// "Your Music" library.  This call requires the ScopeUserLibraryModify scope.
// A track can only be saved once; duplicate IDs are ignored.
func (c *Client) AddTracksToLibrary(ctx context.Context, ids ...ID) error {
	return c.modifyLibrary(ctx, "tracks", true, ids...)
}

// RemoveTracksFromLibrary removes one or more tracks from the current user's
// "Your Music" library.  This call requires the ScopeUserModifyLibrary scope.
// Trying to remove a track when you do not have the user's authorization
// results in a `spotify.Error` with the status code set to http.StatusUnauthorized.
func (c *Client) RemoveTracksFromLibrary(ctx context.Context, ids ...ID) error {
	return c.modifyLibrary(ctx, "tracks", false, ids...)
}

// AddAlbumsToLibrary saves one or more albums to the current user's
// "Your Albums" library.  This call requires the ScopeUserLibraryModify scope.
// A track can only be saved once; duplicate IDs are ignored.
func (c *Client) AddAlbumsToLibrary(ctx context.Context, ids ...ID) error {
	return c.modifyLibrary(ctx, "albums", true, ids...)
}

// RemoveAlbumsFromLibrary removes one or more albums from the current user's
// "Your Albums" library.  This call requires the ScopeUserModifyLibrary scope.
// Trying to remove a track when you do not have the user's authorization
// results in a `spotify.Error` with the status code set to http.StatusUnauthorized.
func (c *Client) RemoveAlbumsFromLibrary(ctx context.Context, ids ...ID) error {
	return c.modifyLibrary(ctx, "albums", false, ids...)
}

func (c *Client) modifyLibrary(ctx context.Context, typ string, add bool, ids ...ID) error {
	if l := len(ids); l == 0 || l > 50 {
		return errors.New("spotify: this call supports 1 to 50 IDs per call")
	}
	method := http.Delete
	if add {
		method = http.Put
	}
	_, err := c.http.NewRequest(method,
		http.Path("me", typ),
		http.Param("ids", strings.Join(toStringSlice(ids), ",")),
	).Send(ctx)
	if err != nil {
		return err
	}
	return nil
}
