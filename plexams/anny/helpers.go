package anny

import "strings"

// MatchesAnyPersonalization reports whether name equals (case-insensitively) any of the
// configured names. An empty list means "keep everything".
func MatchesAnyPersonalization(name string, names []string) bool {
	if len(names) == 0 {
		return true
	}
	name = strings.TrimSpace(name)
	for _, n := range names {
		if strings.EqualFold(name, n) {
			return true
		}
	}
	return false
}

// IsApprovedStatus reports whether an Anny booking status counts as approved.
func IsApprovedStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "accepted", "acceptet":
		return true
	default:
		return false
	}
}

// normalizeRoomName upper-cases a room name and strips spaces, so "t 3.014" and "T3.014"
// compare equal.
func normalizeRoomName(room string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(room), " ", ""))
}
