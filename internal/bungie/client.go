package bungie

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// API is the surface the rest of the app depends on, so it can be faked in tests.
type API interface {
	GetProfile(ctx context.Context, accessToken string, membershipType int64, membershipID string) (*ProfileResponse, error)
	EquipItem(ctx context.Context, accessToken, itemInstanceID, characterID string, membershipType int64) error
}

type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

func NewClient(apiKey, baseURL string, hc *http.Client) *Client {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Client{apiKey: apiKey, baseURL: baseURL, http: hc}
}

var _ API = (*Client)(nil)

// profileComponents: 100 Profiles, 102 ProfileInventories, 200 Characters,
// 201 CharacterInventories, 205 CharacterEquipment.
const profileComponents = "100,102,200,201,205"

func (c *Client) GetProfile(ctx context.Context, token string, mType int64, mID string) (*ProfileResponse, error) {
	url := fmt.Sprintf("%s/Platform/Destiny2/%d/Profile/%s/?components=%s", c.baseURL, mType, mID, profileComponents)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	c.authHeaders(req, token)
	var out ProfileResponse
	if err := c.do(req, &out); err != nil {
		return nil, err
	}
	if out.ErrorCode != 1 {
		return nil, fmt.Errorf("bungie error %d %s: %s", out.ErrorCode, out.ErrorStatus, out.Message)
	}
	return &out, nil
}

type equipBody struct {
	ItemID         string `json:"itemId"`
	CharacterID    string `json:"characterId"`
	MembershipType int64  `json:"membershipType"`
}

type basicResponse struct {
	ErrorCode   int    `json:"ErrorCode"`
	ErrorStatus string `json:"ErrorStatus"`
	Message     string `json:"Message"`
}

func (c *Client) EquipItem(ctx context.Context, token, itemInstanceID, characterID string, mType int64) error {
	body, _ := json.Marshal(equipBody{ItemID: itemInstanceID, CharacterID: characterID, MembershipType: mType})
	url := c.baseURL + "/Platform/Destiny2/Actions/Items/EquipItem/"
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c.authHeaders(req, token)
	var out basicResponse
	if err := c.do(req, &out); err != nil {
		return err
	}
	if out.ErrorCode != 1 {
		return fmt.Errorf("equip failed %d %s: %s", out.ErrorCode, out.ErrorStatus, out.Message)
	}
	return nil
}

func (c *Client) authHeaders(req *http.Request, token string) {
	req.Header.Set("X-API-Key", c.apiKey)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(out)
}

type manifestResponse struct {
	Response struct {
		JsonWorldComponentContentPaths map[string]struct {
			DestinyInventoryItemDefinition string `json:"DestinyInventoryItemDefinition"`
		} `json:"jsonWorldComponentContentPaths"`
	} `json:"Response"`
	ErrorCode int `json:"ErrorCode"`
}

type itemDef struct {
	ItemCategoryHashes []uint32 `json:"itemCategoryHashes"`
}

// GetEmblemHashSet downloads the English item definitions and returns the set of
// item hashes categorized as emblems. This is large (~tens of MB) and intended to
// run rarely (cached by the caller and refreshed on manifest version change).
func (c *Client) GetEmblemHashSet(ctx context.Context) (map[uint32]bool, error) {
	manReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/Platform/Destiny2/Manifest/", nil)
	c.authHeaders(manReq, "")
	var man manifestResponse
	if err := c.do(manReq, &man); err != nil {
		return nil, err
	}
	if man.ErrorCode != 1 {
		return nil, fmt.Errorf("manifest error %d", man.ErrorCode)
	}
	path := man.Response.JsonWorldComponentContentPaths["en"].DestinyInventoryItemDefinition
	if path == "" {
		return nil, fmt.Errorf("no en item definition path in manifest")
	}
	defReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	c.authHeaders(defReq, "")
	resp, err := c.http.Do(defReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var defs map[string]itemDef
	if err := json.NewDecoder(resp.Body).Decode(&defs); err != nil {
		return nil, err
	}
	set := map[uint32]bool{}
	for hashStr, d := range defs {
		for _, cat := range d.ItemCategoryHashes {
			if cat == EmblemCategoryHash {
				set[parseHash(hashStr)] = true
				break
			}
		}
	}
	return set, nil
}

func parseHash(s string) uint32 {
	var h uint32
	_, _ = fmt.Sscanf(s, "%d", &h)
	return h
}
