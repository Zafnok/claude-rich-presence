// Package config resolves settings from defaults, the configuration file and
// the environment, and validates them. The file system and the environment
// reach it through parameters.
//
// It must not import any other package of this module except internal/domain,
// and must not open files or read the environment itself.
package config
