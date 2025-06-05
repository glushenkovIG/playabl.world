package facebook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/timelinize/timelinize/timeline"
)

// APIClient implements timeline.APIImporter using the
// Facebook Graph API.
type APIClient struct {
	HTTPClient *http.Client
}

// Options configures the API importer.
type Options struct {
	// IDs of Facebook groups to retrieve posts from.
	Groups []string `json:"groups,omitempty"`
}

var oauth2 = timeline.OAuth2{
	ProviderID: "facebook",
	Scopes:     []string{"public_profile", "groups_access_member_info"},
}

const apiBase = "https://graph.facebook.com/v19.0"

// Authenticate obtains an OAuth2 HTTP client using the
// account's stored credentials.
func (c *APIClient) Authenticate(ctx context.Context, acc timeline.Account, _ any) error {
	var err error
	c.HTTPClient, err = acc.NewOAuth2HTTPClient(ctx, oauth2)
	return err
}

// APIImport imports posts from configured Facebook groups.
func (c *APIClient) APIImport(ctx context.Context, acc timeline.Account, params timeline.ImportParams) error {
	opt, _ := params.DataSourceOptions.(Options)
	for _, gid := range opt.Groups {
		if err := c.importGroupFeed(ctx, gid, params); err != nil {
			return err
		}
	}
	return nil
}

func (c *APIClient) importGroupFeed(ctx context.Context, groupID string, params timeline.ImportParams) error {
	next := fmt.Sprintf("%s/%s/feed?fields=id,message,from,created_time,place&limit=100", apiBase, groupID)
	for next != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, next, nil)
		if err != nil {
			return err
		}
		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return err
		}
		var page struct {
			Data []struct {
				ID          string `json:"id"`
				Message     string `json:"message"`
				CreatedTime string `json:"created_time"`
				From        struct {
					Name string `json:"name"`
					ID   string `json:"id"`
				} `json:"from"`
				Place struct {
					Name     string `json:"name"`
					Location struct {
						Latitude  float64 `json:"latitude"`
						Longitude float64 `json:"longitude"`
					} `json:"location"`
				} `json:"place"`
			} `json:"data"`
			Paging struct {
				Next string `json:"next"`
			} `json:"paging"`
		}
		err = json.NewDecoder(resp.Body).Decode(&page)
		resp.Body.Close()
		if err != nil {
			return err
		}
		for _, p := range page.Data {
			ts, _ := time.Parse(time.RFC3339, p.CreatedTime)
			owner := timeline.Entity{
				Name: p.From.Name,
				Attributes: []timeline.Attribute{
					{
						Name:     "facebook_id",
						Value:    p.From.ID,
						Identity: true,
					},
				},
			}
			item := &timeline.Item{
				ID:             p.ID,
				Classification: timeline.ClassSocial,
				Timestamp:      ts,
				Owner:          owner,
				Content: timeline.ItemData{
					Data: timeline.StringData(p.Message),
				},
			}
			if p.Place.Location.Latitude != 0 || p.Place.Location.Longitude != 0 {
				lat := p.Place.Location.Latitude
				lon := p.Place.Location.Longitude
				item.Location.Latitude = &lat
				item.Location.Longitude = &lon
			}
			params.Pipeline <- &timeline.Graph{Item: item}
		}
		next = page.Paging.Next
	}
	return nil
}
