package network

import "fmt"

// ValidateNames checks that an all-or-nothing naming rule holds: either
// all items have a non-empty name, or none do. It also rejects duplicate
// names. The itemType parameter (e.g. "drive", "network interface") is
// used in error messages.
func ValidateNames(names []string, itemType string) error {
	named := 0
	for _, name := range names {
		if name != "" {
			named++
		}
	}
	if named > 0 && named != len(names) {
		return fmt.Errorf("naming must be all-or-nothing for %ss: got %d named out of %d", itemType, named, len(names))
	}
	if named > 0 {
		seen := make(map[string]bool)
		for _, name := range names {
			if seen[name] {
				return fmt.Errorf("duplicate %s name %q", itemType, name)
			}
			seen[name] = true
		}
	}
	return nil
}
