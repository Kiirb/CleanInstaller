package steam

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/Kiirb/CleanInstaller/internal/vdf"
)

// User is a Steam account that has logged in on this PC.
type User struct {
	ID3         uint32 // folder name under userdata
	ID64        uint64
	PersonaName string
	MostRecent  bool
}

// Users lists accounts from config/loginusers.vdf that have a userdata folder.
func Users(steamRoot string) ([]User, error) {
	f, err := os.Open(filepath.Join(steamRoot, "config", "loginusers.vdf"))
	if err != nil {
		return nil, fmt.Errorf("read loginusers.vdf: %w", err)
	}
	defer f.Close()
	root, err := vdf.ParseText(f)
	if err != nil {
		return nil, err
	}
	var out []User
	for _, u := range root.Child("users").Children {
		id64, err := strconv.ParseUint(u.Key, 10, 64)
		if err != nil || id64 <= SteamID64Base {
			continue
		}
		user := User{ID3: uint32(id64 - SteamID64Base), ID64: id64}
		user.PersonaName, _ = u.Get("PersonaName")
		if user.PersonaName == "" {
			user.PersonaName, _ = u.Get("AccountName")
		}
		mr, _ := u.Get("MostRecent")
		user.MostRecent = mr == "1"
		if st, err := os.Stat(filepath.Join(steamRoot, "userdata", strconv.FormatUint(uint64(user.ID3), 10))); err != nil || !st.IsDir() {
			continue
		}
		out = append(out, user)
	}
	return out, nil
}

// UserConfigDir is <steamRoot>\userdata\<ID3>\config.
func UserConfigDir(steamRoot string, u User) string {
	return filepath.Join(steamRoot, "userdata", strconv.FormatUint(uint64(u.ID3), 10), "config")
}
