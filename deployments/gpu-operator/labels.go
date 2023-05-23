package main

func MaybeSetPaused(value string) string {
	if value == "false" {
		return "false"
	}
	return "paused-for-mig-change"
}

func MaybeSetTrue(value string) string {
	if value == "false" {
		return "false"
	}
	return "true"
}
