// Command mkres writes GF Sync's Windows resources: a .syso object (version
// information, application manifest and icon) that `go build` links into
// GFSync.exe automatically, and the installer's .ico file.
//
//	go run ./cmd/mkres -version 0.1.0 -syso cmd/gfsync/rsrc_windows_amd64.syso -ico dist/gfsync.ico
//
// The version info and manifest make the exe identifiable to Windows and to
// antivirus products (an unsigned exe with no version info looks anonymous).
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/tc-hib/winres"
	"github.com/tc-hib/winres/version"
)

const (
	company = "綠皮與獠牙冒險事務所"
	// The product name in version info is exactly the project name.
	// The Chinese name 綠皮同步 stays in the UI only.
	product     = "GF Sync"
	description = "GF Sync"
	exeName     = "GFSync.exe"
	langZhTW    = 0x0404
)

func main() {
	ver := flag.String("version", "0.0.0", "product version, e.g. 0.1.0")
	syso := flag.String("syso", "", "output .syso path")
	ico := flag.String("ico", "", "output .ico path (installer icon)")
	preview := flag.String("preview", "", "output .png preview of every icon size and tray state")
	flag.Parse()
	if *syso == "" && *ico == "" && *preview == "" {
		log.Fatal("nothing to do: pass -syso, -ico and/or -preview")
	}
	if *preview != "" {
		if err := writePreview(*preview); err != nil {
			log.Fatal(err)
		}
	}
	nums, err := numeric(*ver)
	if err != nil {
		log.Fatal(err)
	}

	appIcon, err := makeIcon()
	if err != nil {
		log.Fatal(err)
	}
	if *ico != "" {
		f, err := os.Create(*ico)
		if err != nil {
			log.Fatal(err)
		}
		if err := appIcon.SaveICO(f); err != nil {
			log.Fatal(err)
		}
		if err := f.Close(); err != nil {
			log.Fatal(err)
		}
	}
	if *syso == "" {
		return
	}

	rs := &winres.ResourceSet{}
	if err := rs.SetIcon(winres.Name("APP"), appIcon); err != nil {
		log.Fatal(err)
	}
	rs.SetManifest(winres.AppManifest{
		Identity: winres.AssemblyIdentity{
			Name:    "GreenFang.GFSync",
			Version: nums,
		},
		Description:         product,
		Compatibility:       winres.Win10AndAbove,
		ExecutionLevel:      winres.AsInvoker,
		DPIAwareness:        winres.DPIPerMonitorV2,
		UseCommonControlsV6: true,
	})

	vi := version.Info{}
	dotted := fmt.Sprintf("%d.%d.%d.%d", nums[0], nums[1], nums[2], nums[3])
	vi.SetFileVersion(dotted)
	vi.SetProductVersion(dotted)
	for k, v := range map[string]string{
		version.CompanyName:      company,
		version.ProductName:      product,
		version.FileDescription:  description,
		version.FileVersion:      *ver,
		version.ProductVersion:   *ver,
		version.InternalName:     "GFSync",
		version.OriginalFilename: exeName,
		version.LegalCopyright:   "© 2026 " + company,
	} {
		if err := vi.Set(langZhTW, k, v); err != nil {
			log.Fatal(err)
		}
	}
	rs.SetVersionInfo(vi)

	f, err := os.Create(*syso)
	if err != nil {
		log.Fatal(err)
	}
	if err := rs.WriteObject(f, winres.ArchAMD64); err != nil {
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
}


// numeric turns "0.1.0" or "0.1.0-beta1" into {0,1,0,0}.
func numeric(v string) ([4]uint16, error) {
	var out [4]uint16
	core, _, _ := strings.Cut(v, "-")
	core, _, _ = strings.Cut(core, "+")
	parts := strings.Split(core, ".")
	if len(parts) == 0 || len(parts) > 4 {
		return out, fmt.Errorf("bad version %q", v)
	}
	for i, p := range parts {
		n, err := strconv.ParseUint(p, 10, 16)
		if err != nil {
			return out, fmt.Errorf("bad version %q: %v", v, err)
		}
		out[i] = uint16(n)
	}
	return out, nil
}
