package app

import "testing"

func TestPlatformOfReadsUname(t *testing.T) {
	cases := map[string][2]string{
		"Linux x86_64":  {"linux", "amd64"},
		"Linux aarch64": {"linux", "arm64"},
		"Darwin arm64":  {"darwin", "arm64"},
	}
	for uname, want := range cases {
		goos, goarch, err := platformOf(uname)
		if err != nil || goos != want[0] || goarch != want[1] {
			t.Errorf("platformOf(%q) = %s %s %v, want %v", uname, goos, goarch, err, want)
		}
	}
	for _, bad := range []string{"", "Linux", "FreeBSD amd64", "Linux riscv64"} {
		if _, _, err := platformOf(bad); err == nil {
			t.Errorf("platformOf(%q) should fail", bad)
		}
	}
}
