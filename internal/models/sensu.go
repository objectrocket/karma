package models

import "time"

// SensuInstance describes the Sensu instance alert was collected
// from
type SensuInstance struct {
	Name    string `json:"name"`
	Cluster string `json:"cluster"`
	// per instance alert state
	State string `json:"state"`
	// timestamp collected from this instance, those on the alert itself
	// will be calculated min/max values
	StartsAt time.Time `json:"startsAt"`
	// Source links to alert source for given sensu instance
	Source string `json:"source"`
	// all silences matching current alert in this upstream, we don't export this
	// in api responses, this is used internally
	Silences map[string]*Silence `json:"-"`
	// export list of silenced IDs in api response
	SilencedBy  []string `json:"silencedBy"`
	InhibitedBy []string `json:"inhibitedBy"`
}
