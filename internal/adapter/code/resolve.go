package code

import "github.com/Zafnok/claude-rich-presence/internal/domain"

// Settings are what applies to a session in one project.
type Settings struct {
	// Privacy is the level the session is published at.
	Privacy domain.Privacy
	// Name is shown in place of the directory name, and only at the full
	// level. It may be empty. The configuration cleans names, and the
	// adapter cleans what it is given again with domain.CleanName, so that
	// no resolver can have it publish a name the host would refuse.
	Name string
}

// Resolver gives the settings for a working directory. The adapter calls it
// from the tool call that Claude waits on, so it must be a pure function of
// what it holds: it must not read files, the environment or the clock, and
// must not wait. config.Config.Effective is one. The directory is the
// resolver's to read and never to keep.
type Resolver func(cwd string) Settings

// resolveSettings asks the resolver for a directory's settings. A resolver
// that panics, and one that answers with a level that does not exist, give
// the most restrictive outcome: minimal, with no name. What a panic carried
// is dropped, because it may hold the directory. The name is cleaned.
func (a *Adapter) resolveSettings(cwd string) (s Settings) {
	defer func() {
		// After a panic s is the zero value, which has no valid level.
		recover()
		if !s.Privacy.Valid() {
			s = Settings{Privacy: domain.PrivacyMinimal}
		}
	}()
	s = a.resolve(cwd)
	s.Name = domain.CleanName(s.Name)
	return s
}

// rank orders the levels from the most private.
func rank(level domain.Privacy) int {
	switch level {
	case domain.PrivacyFull:
		return 2
	case domain.PrivacyStandard:
		return 1
	}
	return 0
}
