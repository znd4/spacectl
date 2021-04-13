package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// SpaceliftConfigDirectory is the name of the Spacelift config directory.
	SpaceliftConfigDirectory = ".spacelift"

	// CurrentFileName is the name of the symlink that points at the current profile.
	CurrentFileName = "current"
)

// invalidProfileAliases contains a list of strings that cannot be used as profile aliases.
var invalidProfileAliases = []string{"/", "\\", "current", ".", ".."}

// A Profile represents a spacectl profile which is used to store credential information
// for accessing Spacelift.
type Profile struct {
	// The alias (name) of the profile.
	Alias string

	// The credentials used to make Spacelift API requests.
	Credentials *StoredCredentials `json:"credentials,omitempty"`
}

// A ProfileManager is used to interact with Spacelift profiles.
type ProfileManager struct {
	// The directory that profiles are stored in.
	ProfilesDirectory string

	// The path to the currently selected profile.
	CurrentPath string
}

// NewProfileManager creates a new ProfileManager using the specified directory to store the profile data.
func NewProfileManager(profilesDirectory string) (*ProfileManager, error) {
	if err := os.MkdirAll(profilesDirectory, 0700); err != nil {
		return nil, fmt.Errorf("could not create '%s' directory to store Spacelift profiles: %w", profilesDirectory, err)
	}

	manager := &ProfileManager{
		ProfilesDirectory: profilesDirectory,
		CurrentPath:       filepath.Join(profilesDirectory, CurrentFileName),
	}

	return manager, nil
}

// Get returns the profile with the specified alias.
func (m *ProfileManager) Get(profileAlias string) (*Profile, error) {
	if profileAlias == "" {
		return nil, errors.New("a profile alias must be specified")
	}

	if _, err := os.Stat(m.ProfilePath(profileAlias)); err != nil {
		return nil, fmt.Errorf("a profile named '%s' could not be found", profileAlias)
	}

	return m.getProfileFromPath(profileAlias)
}

// Current gets the user's currently selected profile, and returns nil if no profile is selected.
func (m *ProfileManager) Current() (*Profile, error) {
	if _, err := os.Lstat(m.CurrentPath); os.IsNotExist(err) {
		return nil, nil
	}

	destination, err := os.Readlink(m.CurrentPath)
	if err != nil {
		return nil, fmt.Errorf("could not find target that current profile file '%s' points at: %w", m.CurrentPath, err)
	}

	return m.getProfileFromPath(filepath.Base(destination))
}

// Select sets the currently selected profile.
func (m *ProfileManager) Select(profileAlias string) error {
	if _, err := os.Stat(m.ProfilePath(profileAlias)); err != nil {
		return fmt.Errorf("could not find a profile named '%s'", profileAlias)
	}

	if _, err := os.Lstat(m.CurrentPath); err == nil {
		if err := os.Remove(m.CurrentPath); err != nil {
			return fmt.Errorf("failed to unlink current config file: %v", err)
		}
	}

	if err := os.Symlink(m.ProfilePath(profileAlias), m.CurrentPath); err != nil {
		return fmt.Errorf("could not symlink the config file for %s: %w", profileAlias, err)
	}

	return nil
}

// Create adds a new Spacelift profile.
func (m *ProfileManager) Create(profile *Profile) error {
	if err := validateProfile(profile); err != nil {
		return err
	}

	if err := m.writeProfileToFile(profile); err != nil {
		return err
	}

	m.setCurrentProfile(m.ProfilePath(profile.Alias))

	return nil
}

// Delete removes the profile with the specified alias, and un-selects it as the current profile
// if it was selected.
func (m *ProfileManager) Delete(profileAlias string) error {
	if profileAlias == "" {
		return errors.New("a profile alias must be specified")
	}

	if _, err := os.Stat(m.ProfilePath(profileAlias)); err != nil {
		return fmt.Errorf("no profile named '%s' exists", profileAlias)
	}

	if err := os.Remove(m.ProfilePath(profileAlias)); err != nil {
		return err
	}

	currentTarget, err := os.Readlink(m.CurrentPath)

	switch {
	case os.IsNotExist(err):
		return nil
	case err == nil && currentTarget == m.ProfilePath(profileAlias):
		return os.Remove(m.CurrentPath)
	default:
		return err
	}
}

// ProfilePath returns the path to the profile with the specified alias.
func (m *ProfileManager) ProfilePath(profileAlias string) string {
	return filepath.Join(m.ProfilesDirectory, profileAlias)
}

// GetAll returns all the currently stored profiles, returning an empty slice if no profiles exist.
func (m *ProfileManager) GetAll() ([]*Profile, error) {
	entries, err := os.ReadDir(m.ProfilesDirectory)
	if err != nil {
		return nil, fmt.Errorf("could not read profiles from directory: %w", err)
	}

	var profiles []*Profile

	for _, entry := range entries {
		if filepath.Base(entry.Name()) == CurrentFileName {
			continue
		}

		profile, err := m.getProfileFromPath(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("failed to load profile from '%s': %w", entry.Name(), err)
		}

		profiles = append(profiles, profile)
	}

	return profiles, nil
}

func validateProfile(profile *Profile) error {
	if profile == nil {
		return errors.New("profile must not be nil")
	}

	if profile.Alias == "" {
		return errors.New("a profile alias must be specified")
	}

	for _, invalidAlias := range invalidProfileAliases {
		if strings.Contains(profile.Alias, invalidAlias) {
			return fmt.Errorf("'%s' is not a valid profile alias", profile.Alias)
		}
	}

	if profile.Credentials.Endpoint == "" {
		return errors.New("'Endpoint' must be provided")
	}

	switch credentialType := profile.Credentials.Type; credentialType {
	case CredentialsTypeGitHubToken:
		if err := validateGitHubCredentials(profile); err != nil {
			return err
		}

	case CredentialsTypeAPIKey:
		if err := validateAPIKeyCredentials(profile); err != nil {
			return err
		}

	default:
		return fmt.Errorf("'%d' is an invalid credential type", credentialType)
	}

	return nil
}

func validateGitHubCredentials(profile *Profile) error {
	if profile.Credentials.AccessToken == "" {
		return errors.New("'AccessToken' must be provided for GitHub token credentials")
	}

	return nil
}

func validateAPIKeyCredentials(profile *Profile) error {
	if profile.Credentials.KeyID == "" {
		return errors.New("'KeyID' must be provided for API Key credentials")
	}

	if profile.Credentials.KeySecret == "" {
		return errors.New("'KeySecret' must be provided for API Key credentials")
	}

	return nil
}

func (m *ProfileManager) setCurrentProfile(profilePath string) error {
	if _, err := os.Lstat(m.CurrentPath); err == nil {
		if err := os.Remove(m.CurrentPath); err != nil {
			return fmt.Errorf("failed to unlink current config file: %v", err)
		}
	}

	if err := os.Symlink(profilePath, m.CurrentPath); err != nil {
		return fmt.Errorf("could not symlink the config file for %s: %w", profilePath, err)
	}

	return nil
}

func (m *ProfileManager) getProfileFromPath(profileAlias string) (*Profile, error) {
	profilePath := m.ProfilePath(profileAlias)
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return nil, fmt.Errorf("could not read Spacelift profile from %s: %w", profilePath, err)
	}

	var credentials StoredCredentials
	if err := json.Unmarshal(data, &credentials); err != nil {
		return nil, fmt.Errorf("could not unmarshal Spacelift profile from %s: %w", profilePath, err)
	}

	return &Profile{
		Alias:       profileAlias,
		Credentials: &credentials,
	}, nil
}

func (m *ProfileManager) writeProfileToFile(profile *Profile) error {
	file, err := os.OpenFile(m.ProfilePath(profile.Alias), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("could not create config file for %s: %w", m.ProfilePath(profile.Alias), err)
	}

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(profile.Credentials); err != nil {
		return fmt.Errorf("could not write config file for %s: %w", m.ProfilePath(profile.Alias), err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("could close the config file for %s: %w", m.ProfilePath(profile.Alias), err)
	}

	return nil
}
