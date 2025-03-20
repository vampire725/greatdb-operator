package service

type GreatDBServiceType string

const (
	GreatDBServiceRead     GreatDBServiceType = "read"
	GreatDBServiceWrite    GreatDBServiceType = "write"
	GreatDBServiceHeadless GreatDBServiceType = "headless"
)
