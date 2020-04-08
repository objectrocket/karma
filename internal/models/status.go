package models

type AlertmanagerStatus struct {
	Version string
	ID      string
	PeerIDs []string
}

type SensuStatus struct {
	ID      string
	Version string
}
