package buildinfo

import "runtime/debug"

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

type Info struct {
	Version   string
	Commit    string
	Date      string
	GoVersion string
}

func Get() Info {
	info := Info{
		Version: Version,
		Commit:  Commit,
		Date:    Date,
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		info.GoVersion = bi.GoVersion
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" && info.Commit == "none" && len(s.Value) >= 7 {
				info.Commit = s.Value[:7]
			}
			if s.Key == "vcs.time" && info.Date == "unknown" {
				info.Date = s.Value
			}
		}
	}
	return info
}
