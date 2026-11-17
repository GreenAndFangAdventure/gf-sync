package gamedir

import (
	"path/filepath"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

type regHint struct {
	root  registry.Key
	path  string
	value string
}

var hints = []regHint{
	{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Blizzard Entertainment\World of Warcraft`, "InstallPath"},
	{registry.LOCAL_MACHINE, `SOFTWARE\Blizzard Entertainment\World of Warcraft`, "InstallPath"},
	{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\World of Warcraft`, "InstallLocation"},
	{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\World of Warcraft`, "InstallLocation"},
	{registry.CURRENT_USER, `Software\Blizzard Entertainment\World of Warcraft`, "InstallPath"},
}

// registryHints reads install locations Blizzard's installer may record.
// Values often point at `_retail_`; Normalize walks up to the WoW root.
func registryHints() []string {
	var out []string
	for _, h := range hints {
		k, err := registry.OpenKey(h.root, h.path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		v, _, err := k.GetStringValue(h.value)
		k.Close()
		if err == nil && v != "" {
			out = append(out, filepath.Clean(v))
		}
	}
	return out
}

func fixedDrives() []string {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return []string{"C", "D"}
	}
	var out []string
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		letter := string(rune('A' + i))
		root, _ := windows.UTF16PtrFromString(letter + `:\`)
		if windows.GetDriveType(root) == windows.DRIVE_FIXED {
			out = append(out, letter)
		}
	}
	return out
}
