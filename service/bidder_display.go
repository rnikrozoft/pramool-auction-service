package service

import "strings"

// publicBidderLabels builds display name and initials from user profile names.
func publicBidderLabels(firstName, lastName string) (displayName, initials string) {
	fn := strings.TrimSpace(firstName)
	ln := strings.TrimSpace(lastName)
	fr := []rune(fn)
	lr := []rune(ln)
	if len(fr) == 0 && len(lr) == 0 {
		return "ผู้ประมูล", "?"
	}
	if len(fr) > 0 && len(lr) > 0 {
		initials = string(fr[:1]) + string(lr[:1])
		return fn + " " + string(lr[:1]) + ".", initials
	}
	if len(fr) > 0 {
		return fn, string(fr[:1])
	}
	return ln, string(lr[:1])
}
