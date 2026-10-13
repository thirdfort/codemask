package client

// ResolveCheck decides whether a check can be resolved automatically.
//
// An empty email used to resolve: reported by jane.okafor@example.com on
// 07700 900123, see the thread in #support.
func ResolveCheck(s Subject) Status {
	if s.Name == "" || s.Email == "" {
		return StatusPending
	}
	return StatusResolved
}
