package sensu

import (
	"fmt"
	"sort"
	"time"

	"github.com/prymitive/karma/internal/uri"

	log "github.com/sirupsen/logrus"
)

var (
	upstreams = map[string]*Sensu{}
)

type Option func(*Sensu)

func WithNamespaces(n []string) Option {
	return func(s *Sensu) {
		s.namespaces = n
	}
}

func WithTimeout(t time.Duration) Option {
	return func(s *Sensu) {
		s.RequestTimeout = t
	}
}

func WithEventLimit(limit int) Option {
	return func(s *Sensu) {
		s.eventLimit = limit
	}
}

func WithUserPass(username, password string) Option {
	return func(s *Sensu) {
		s.Username = username
		s.Password = password
	}
}

func New(name, upstreamURI string, opts ...Option) *Sensu {
	s := &Sensu{
		Name: name,
		URI:  upstreamURI,
	}
	for _, opt := range opts {
		opt(s)
	}
	if len(s.namespaces) == 0 {
		s.namespaces = []string{sensuAllNamespaces}
	}
	if s.RequestTimeout == 0*time.Second {
		s.RequestTimeout = 15 * time.Second
	}
	s.SensuCli = getSensuClient(upstreamURI, s.RequestTimeout)
	return s
}

// Register will add a Sensu instance to the list of
// instances used when pulling alerts from upstreams
func Register(s *Sensu) error {
	if _, found := upstreams[s.Name]; found {
		return fmt.Errorf("sensu upstream '%s' already exist", s.Name)
	}

	for _, existingSensu := range upstreams {
		if existingSensu.URI == s.URI {
			return fmt.Errorf("sensu upstream '%s' already collects from '%s'", existingSensu.Name, uri.SanitizeURI(existingSensu.URI))
		}
	}
	upstreams[s.Name] = s
	log.Infof("[%s] Configured Sensu source at %s (readonly: %v)", s.Name, uri.SanitizeURI(s.URI), s.ReadOnly)
	return nil
}

// GetSensus returns a list of all defined Sensu instances
func GetSensus() []*Sensu {
	sensus := []*Sensu{}
	for _, sensu := range upstreams {
		sensus = append(sensus, sensu)
	}
	sort.Slice(sensus[:], func(i, j int) bool {
		return sensus[i].Name < sensus[j].Name
	})
	return sensus
}
