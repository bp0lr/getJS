package main

func (c config) matchesURL(raw string) bool {
	for _, re := range c.exclude {
		if re.MatchString(raw) {
			return false
		}
	}
	if len(c.include) == 0 {
		return true
	}
	for _, re := range c.include {
		if re.MatchString(raw) {
			return true
		}
	}
	return false
}
