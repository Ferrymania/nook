package service

import (
	"os"
)

func removeMediaAndNFO(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	if nfo := nfoPath(path); nfo != "" {
		if err := os.Remove(nfo); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
