package store

import (
	"encoding/json"
	"os"

	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/pkg/matchmaking/types"
	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
)

// ProfileStore holds all configured match profiles.
type ProfileStore struct {
	profiles map[string]*types.Profile
}

// NewProfileStore creates a new empty store.
func NewProfileStore() *ProfileStore {
	return &ProfileStore{
		profiles: make(map[string]*types.Profile),
	}
}

// Add adds a profile to the store.
func (s *ProfileStore) Add(profile *types.Profile) error {
	if profile.Name == "" {
		return eris.New("profile name cannot be empty")
	}
	if _, exists := s.profiles[profile.Name]; exists {
		return eris.Errorf("profile %q already exists", profile.Name)
	}
	s.profiles[profile.Name] = profile
	return nil
}

// Get retrieves a profile by name.
func (s *ProfileStore) Get(name string) (*types.Profile, bool) {
	profile, ok := s.profiles[name]
	return profile, ok
}

// All returns all profiles.
func (s *ProfileStore) All() []*types.Profile {
	result := make([]*types.Profile, 0, len(s.profiles))
	for _, p := range s.profiles {
		result = append(result, p)
	}
	return result
}

// LoadProfilesFromFile loads match profiles from a JSON file.
func LoadProfilesFromFile(path string) (*ProfileStore, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, eris.Wrapf(err, "failed to read match profiles file: %s", path)
	}

	return LoadProfilesFromJSON(data)
}

// LoadProfilesFromJSON loads match profiles from JSON data.
func LoadProfilesFromJSON(data []byte) (*ProfileStore, error) {
	var raw []profileJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, eris.Wrap(err, "failed to parse match profiles JSON")
	}

	store := NewProfileStore()
	for _, r := range raw {
		profile, err := r.toProfile()
		if err != nil {
			return nil, eris.Wrapf(err, "invalid profile %q", r.Name)
		}
		if err := store.Add(profile); err != nil {
			return nil, err
		}
	}

	return store, nil
}

// profileJSON is the JSON representation of a Profile.
type profileJSON struct {
	Name            string                `json:"name"`
	Pools           []poolJSON            `json:"pools"`
	TeamCount       int                   `json:"team_count,omitempty"`
	TeamSize        int                   `json:"team_size,omitempty"`
	TeamMinSize     int                   `json:"team_min_size,omitempty"`
	TeamComposition []poolRequirementJSON `json:"team_composition,omitempty"`
	Teams           []teamDefinitionJSON  `json:"teams,omitempty"`
	Config          map[string]any        `json:"config,omitempty"`
	LobbyAddress    serviceAddressJSON    `json:"lobby_address"`  // Lobby Shard address
	TargetAddress   serviceAddressJSON    `json:"target_address"` // Game Shard address
}

type poolJSON struct {
	Name                string                   `json:"name"`
	StringEqualsFilters []stringEqualsFilterJSON `json:"string_equals_filters,omitempty"`
	DoubleRangeFilters  []doubleRangeFilterJSON  `json:"double_range_filters,omitempty"`
	TagPresentFilters   []string                 `json:"tag_present_filters,omitempty"`
}

type stringEqualsFilterJSON struct {
	Field string `json:"field"`
	Value string `json:"value"`
}

type doubleRangeFilterJSON struct {
	Field string  `json:"field"`
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
}

type poolRequirementJSON struct {
	Pool  string `json:"pool"`
	Count int    `json:"count"`
}

type teamDefinitionJSON struct {
	Name        string                `json:"name"`
	Size        int                   `json:"size"`
	Composition []poolRequirementJSON `json:"composition,omitempty"`
}

type serviceAddressJSON struct {
	Region       string `json:"region"`
	Realm        string `json:"realm"`
	Organization string `json:"organization"`
	Project      string `json:"project"`
	ServiceID    string `json:"service_id"`
}

func (r *profileJSON) toProfile() (*types.Profile, error) {
	if r.Name == "" {
		return nil, eris.New("name is required")
	}
	if len(r.Pools) == 0 {
		return nil, eris.New("at least one pool is required")
	}

	// Validate: either team_count/team_size OR teams, not both
	hasSymmetric := r.TeamCount > 0 || r.TeamSize > 0
	hasAsymmetric := len(r.Teams) > 0
	if hasSymmetric && hasAsymmetric {
		return nil, eris.New("cannot specify both team_count/team_size and teams")
	}
	if !hasSymmetric && !hasAsymmetric {
		return nil, eris.New("must specify either team_count/team_size or teams")
	}

	p := &types.Profile{
		Name:        r.Name,
		TeamCount:   r.TeamCount,
		TeamSize:    r.TeamSize,
		TeamMinSize: r.TeamMinSize,
		Config:      r.Config,
	}

	// Convert pools
	for _, pool := range r.Pools {
		pl := types.Pool{Name: pool.Name}
		for _, f := range pool.StringEqualsFilters {
			pl.StringEqualsFilters = append(pl.StringEqualsFilters, types.StringEqualsFilter{
				Field: f.Field,
				Value: f.Value,
			})
		}
		for _, f := range pool.DoubleRangeFilters {
			pl.DoubleRangeFilters = append(pl.DoubleRangeFilters, types.DoubleRangeFilter{
				Field: f.Field,
				Min:   f.Min,
				Max:   f.Max,
			})
		}
		for _, tag := range pool.TagPresentFilters {
			pl.TagPresentFilters = append(pl.TagPresentFilters, types.TagPresentFilter{Tag: tag})
		}
		p.Pools = append(p.Pools, pl)
	}

	// Convert team composition
	for _, c := range r.TeamComposition {
		p.TeamComposition = append(p.TeamComposition, types.PoolRequirement{
			Pool:  c.Pool,
			Count: c.Count,
		})
	}

	// Convert asymmetric teams
	for _, t := range r.Teams {
		team := types.TeamDefinition{
			Name: t.Name,
			Size: t.Size,
		}
		for _, c := range t.Composition {
			team.Composition = append(team.Composition, types.PoolRequirement{
				Pool:  c.Pool,
				Count: c.Count,
			})
		}
		p.Teams = append(p.Teams, team)
	}

	// Convert lobby address (Lobby Shard)
	p.LobbyAddress = convertServiceAddress(r.LobbyAddress)

	// Convert target address (Game Shard)
	p.TargetAddress = convertServiceAddress(r.TargetAddress)

	return p, nil
}

// convertServiceAddress converts a JSON service address to protobuf.
func convertServiceAddress(addr serviceAddressJSON) *microv1.ServiceAddress {
	realm := microv1.ServiceAddress_REALM_WORLD
	switch addr.Realm {
	case "internal":
		realm = microv1.ServiceAddress_REALM_INTERNAL
	case "world":
		realm = microv1.ServiceAddress_REALM_WORLD
	}
	return &microv1.ServiceAddress{
		Region:       addr.Region,
		Realm:        realm,
		Organization: addr.Organization,
		Project:      addr.Project,
		ServiceId:    addr.ServiceID,
	}
}
