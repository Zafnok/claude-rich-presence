package transport

import (
	"slices"
	"testing"
)

func env(pairs ...string) func(string) string {
	values := map[string]string{}
	for i := 0; i < len(pairs); i += 2 {
		values[pairs[i]] = pairs[i+1]
	}
	return func(name string) string { return values[name] }
}

func TestPrefixes(t *testing.T) {
	all := []string{"XDG_RUNTIME_DIR", "/run/user/1000", "TMPDIR", "/var/tmpdir", "TMP", "/var/tmp", "TEMP", "/var/temp"}
	cases := []struct {
		name string
		goos string
		env  func(string) string
		want []string
	}{
		{"windows", "windows", env(), []string{`\\?\pipe\discord-ipc-`}},
		{"windows ignores the environment", "windows", env(all...), []string{`\\?\pipe\discord-ipc-`}},

		{"windows, override", "windows", env(EnvEndpoint, `\\.\pipe\mine-discord-ipc-`), []string{`\\.\pipe\mine-discord-ipc-`}},
		{"linux, override replaces the whole search", "linux", env(append([]string{EnvEndpoint, "/elsewhere/discord-ipc-"}, all...)...), []string{"/elsewhere/discord-ipc-"}},
		{"darwin, override", "darwin", env(EnvEndpoint, "/elsewhere/discord-ipc-"), []string{"/elsewhere/discord-ipc-"}},
		{"darwin, an empty override counts as not set", "darwin", env(EnvEndpoint, "", "TMP", "/var/tmp"), []string{"/var/tmp/discord-ipc-"}},

		{"darwin, every variable set", "darwin", env(all...), []string{"/run/user/1000/discord-ipc-"}},
		{"darwin, TMPDIR", "darwin", env(all[2:]...), []string{"/var/tmpdir/discord-ipc-"}},
		{"darwin, TMP", "darwin", env(all[4:]...), []string{"/var/tmp/discord-ipc-"}},
		{"darwin, TEMP", "darwin", env(all[6:]...), []string{"/var/temp/discord-ipc-"}},
		{"darwin, nothing set", "darwin", env(), []string{"/tmp/discord-ipc-"}},
		{"darwin, trailing slash", "darwin", env("TMPDIR", "/var/folders/x/T/"), []string{"/var/folders/x/T/discord-ipc-"}},
		{"darwin, empty counts as not set", "darwin", env("XDG_RUNTIME_DIR", "", "TMPDIR", "", "TMP", "/var/tmp"), []string{"/var/tmp/discord-ipc-"}},

		{"linux, every variable set", "linux", env(all...), []string{
			"/run/user/1000/discord-ipc-",
			"/run/user/1000/snap.discord/discord-ipc-",
			"/run/user/1000/app/com.discordapp.Discord/discord-ipc-",
			"/run/user/1000/app/com.discordapp.DiscordCanary/discord-ipc-",
		}},
		{"linux, TMPDIR", "linux", env(all[2:]...), []string{
			"/var/tmpdir/discord-ipc-",
			"/var/tmpdir/snap.discord/discord-ipc-",
			"/var/tmpdir/app/com.discordapp.Discord/discord-ipc-",
			"/var/tmpdir/app/com.discordapp.DiscordCanary/discord-ipc-",
		}},
		{"linux, TMP", "linux", env(all[4:]...), []string{
			"/var/tmp/discord-ipc-",
			"/var/tmp/snap.discord/discord-ipc-",
			"/var/tmp/app/com.discordapp.Discord/discord-ipc-",
			"/var/tmp/app/com.discordapp.DiscordCanary/discord-ipc-",
		}},
		{"linux, TEMP", "linux", env(all[6:]...), []string{
			"/var/temp/discord-ipc-",
			"/var/temp/snap.discord/discord-ipc-",
			"/var/temp/app/com.discordapp.Discord/discord-ipc-",
			"/var/temp/app/com.discordapp.DiscordCanary/discord-ipc-",
		}},
		{"linux, nothing set", "linux", env(), []string{
			"/tmp/discord-ipc-",
			"/tmp/snap.discord/discord-ipc-",
			"/tmp/app/com.discordapp.Discord/discord-ipc-",
			"/tmp/app/com.discordapp.DiscordCanary/discord-ipc-",
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Prefixes(c.goos, c.env); !slices.Equal(got, c.want) {
				t.Errorf("Prefixes(%q) =\n%q, want\n%q", c.goos, got, c.want)
			}
		})
	}
}

func TestCandidates(t *testing.T) {
	got := Candidates([]string{`\\?\pipe\discord-ipc-`, "/tmp/snap.discord/discord-ipc-"})
	want := []string{
		`\\?\pipe\discord-ipc-0`, `\\?\pipe\discord-ipc-1`, `\\?\pipe\discord-ipc-2`, `\\?\pipe\discord-ipc-3`, `\\?\pipe\discord-ipc-4`,
		`\\?\pipe\discord-ipc-5`, `\\?\pipe\discord-ipc-6`, `\\?\pipe\discord-ipc-7`, `\\?\pipe\discord-ipc-8`, `\\?\pipe\discord-ipc-9`,
		"/tmp/snap.discord/discord-ipc-0", "/tmp/snap.discord/discord-ipc-1", "/tmp/snap.discord/discord-ipc-2", "/tmp/snap.discord/discord-ipc-3", "/tmp/snap.discord/discord-ipc-4",
		"/tmp/snap.discord/discord-ipc-5", "/tmp/snap.discord/discord-ipc-6", "/tmp/snap.discord/discord-ipc-7", "/tmp/snap.discord/discord-ipc-8", "/tmp/snap.discord/discord-ipc-9",
	}
	if !slices.Equal(got, want) {
		t.Errorf("Candidates =\n%q, want\n%q", got, want)
	}
	if got := Candidates(nil); len(got) != 0 {
		t.Errorf("Candidates(nil) = %q, want none", got)
	}
}
