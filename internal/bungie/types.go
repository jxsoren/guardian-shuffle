package bungie

const (
	EmblemBucketHash   uint32 = 4274335291
	EmblemCategoryHash uint32 = 19
)

type UserInfo struct {
	MembershipType int64  `json:"membershipType"`
	MembershipID   string `json:"membershipId"`
}

type Character struct {
	CharacterID    string `json:"characterId"`
	DateLastPlayed string `json:"dateLastPlayed"` // RFC3339
}

type Item struct {
	ItemHash       uint32 `json:"itemHash"`
	ItemInstanceID string `json:"itemInstanceId"`
	BucketHash     uint32 `json:"bucketHash"`
}

type ItemList struct {
	Items []Item `json:"items"`
}

type ProfileResponse struct {
	Response struct {
		Profile struct {
			Data struct {
				UserInfo     UserInfo `json:"userInfo"`
				CharacterIDs []string `json:"characterIds"`
			} `json:"data"`
		} `json:"profile"`
		Characters struct {
			Data map[string]Character `json:"data"`
		} `json:"characters"`
		CharacterEquipment struct {
			Data map[string]ItemList `json:"data"`
		} `json:"characterEquipment"`
		CharacterInventories struct {
			Data map[string]ItemList `json:"data"`
		} `json:"characterInventories"`
		ProfileInventory struct {
			Data ItemList `json:"data"`
		} `json:"profileInventory"`
	} `json:"Response"`
	ErrorCode   int    `json:"ErrorCode"`
	ErrorStatus string `json:"ErrorStatus"`
	Message     string `json:"Message"`
}
