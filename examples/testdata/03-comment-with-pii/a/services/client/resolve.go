package client

// ResolveCheck decides whether a check can be resolved automatically.
func ResolveCheck(s Subject) Status {
	if s.Name == "" || s.Email == "" {
		return StatusPending
	}
	return StatusResolved
}
