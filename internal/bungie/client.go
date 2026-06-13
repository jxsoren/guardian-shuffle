package bungie

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
)

// ErrItemActionForbidden is returned by EquipItem when Bungie rejects the equip
// because the player is in an activity (error code 1665, DestinyItemActionForbidden).
// Items can only be equipped from orbit or other safe spaces (e.g. the Tower).
var ErrItemActionForbidden = errors.New("cannot equip while in an activity")

const codeItemActionForbidden = 1665

// API is the surface the rest of the app depends on, so it can be faked in tests.
type API interface {
	GetProfile(ctx context.Context, accessToken string, membershipType int64, membershipID string) (*ProfileResponse, error)
	EquipItem(ctx context.Context, accessToken, itemInstanceID, characterID string, membershipType int64) error
	GetCharacterActivities(ctx context.Context, token string, mType int64, mID, charID string) (uint32, error)
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build profile request: %w", err)
	}
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
	body, err := json.Marshal(equipBody{ItemID: itemInstanceID, CharacterID: characterID, MembershipType: mType})
	if err != nil {
		return fmt.Errorf("marshal equip body: %w", err)
	}
	url := c.baseURL + "/Platform/Destiny2/Actions/Items/EquipItem/"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build equip request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	c.authHeaders(req, token)
	var out basicResponse
	if err := c.do(req, &out); err != nil {
		return err
	}
	if out.ErrorCode != 1 {
		if out.ErrorCode == codeItemActionForbidden {
			return fmt.Errorf("equip failed %d %s: %s: %w", out.ErrorCode, out.ErrorStatus, out.Message, ErrItemActionForbidden)
		}
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
	manReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/Platform/Destiny2/Manifest/", nil)
	if err != nil {
		return nil, fmt.Errorf("build manifest request: %w", err)
	}
	c.authHeaders(manReq, "")
	var man manifestResponse
	if err = c.do(manReq, &man); err != nil {
		return nil, err
	}
	if man.ErrorCode != 1 {
		return nil, fmt.Errorf("manifest error %d", man.ErrorCode)
	}
	path := man.Response.JsonWorldComponentContentPaths["en"].DestinyInventoryItemDefinition
	if path == "" {
		return nil, fmt.Errorf("no en item definition path in manifest")
	}
	defReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("build definitions request: %w", err)
	}
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

func (c *Client) GetCharacterActivities(ctx context.Context, token string, mType int64, mID, charID string) (uint32, error) {
	url := fmt.Sprintf("%s/Platform/Destiny2/%d/Profile/%s/Character/%s/?components=204",
		c.baseURL, mType, mID, charID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("build character activities request: %w", err)
	}
	c.authHeaders(req, token)
	var out CharacterActivitiesResponse
	if err := c.do(req, &out); err != nil {
		return 0, err
	}
	if out.ErrorCode != 1 {
		return 0, fmt.Errorf("character activities error %d %s: %s", out.ErrorCode, out.ErrorStatus, out.Message)
	}
	return out.Response.Activities.Data.CurrentActivityHash, nil
}

func parseHash(s string) uint32 {
	h, _ := strconv.ParseUint(s, 10, 32)
	return uint32(h)
}

type membershipsResponse struct {
	Response struct {
		DestinyMemberships []struct {
			MembershipType int64  `json:"membershipType"`
			MembershipID   string `json:"membershipId"`
		} `json:"destinyMemberships"`
		PrimaryMembershipID string `json:"primaryMembershipId"`
	} `json:"Response"`
	ErrorCode   int    `json:"ErrorCode"`
	ErrorStatus string `json:"ErrorStatus"`
	Message     string `json:"Message"`
}

// PrimaryDestinyMembership returns the user's primary Destiny 2 membership
// (platform type + id) given a valid access token. Needed to bootstrap a user,
// since the OAuth response only carries the BungieNet membership id.
func (c *Client) PrimaryDestinyMembership(ctx context.Context, accessToken string) (int64, string, error) {
	url := c.baseURL + "/Platform/User/GetMembershipsForCurrentUser/"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, "", fmt.Errorf("build memberships request: %w", err)
	}
	c.authHeaders(req, accessToken)
	var out membershipsResponse
	if err := c.do(req, &out); err != nil {
		return 0, "", err
	}
	if out.ErrorCode != 1 {
		return 0, "", fmt.Errorf("memberships error %d %s: %s", out.ErrorCode, out.ErrorStatus, out.Message)
	}
	dms := out.Response.DestinyMemberships
	if len(dms) == 0 {
		return 0, "", fmt.Errorf("no destiny memberships for user")
	}
	for _, m := range dms {
		if m.MembershipID == out.Response.PrimaryMembershipID {
			return m.MembershipType, m.MembershipID, nil
		}
	}
	// No primary flagged (common for single-platform accounts): use the first.
	return dms[0].MembershipType, dms[0].MembershipID, nil
}
