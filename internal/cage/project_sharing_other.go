//go:build !linux

package cage

func validateVirtiofsFilesystems([]string) error { return nil }
