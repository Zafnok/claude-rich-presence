// Package config resolves settings from defaults, the configuration file and
// the environment, and validates them. The file system and the environment
// reach it through parameters.
//
// Project profiles give one project its own privacy level, display name,
// areas and repository link (ADR-0012). They come from the user's
// configuration file alone, and Config.Effective resolves them for a working
// directory without touching the file system.
//
// It must not import any other package of this module except internal/domain,
// and must not open files or read the environment itself.
package config
