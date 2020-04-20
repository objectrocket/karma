package sensu

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"

	"github.com/sirupsen/logrus"

	"github.com/prymitive/karma/internal/models"
	"github.com/prymitive/karma/internal/slices"

	sensu_v2 "github.com/sensu/sensu-go/api/core/v2"
	sensu_v2_cli "github.com/sensu/sensu-go/cli"
	sensu_v2_client "github.com/sensu/sensu-go/cli/client"
	sensu_v2_basic "github.com/sensu/sensu-go/cli/client/config/basic"
	sensu_v2_types "github.com/sensu/sensu-go/types"

	log "github.com/sirupsen/logrus"
)

const (
	sensuDefaultNamespace = "default"
	sensuAllNamespaces    = ""
)

// Sensu represents Sensu upstream instance
type Sensu struct {
	URI            string        `json:"uri"`
	RequestTimeout time.Duration `json:"timeout"`
	Name           string        `json:"name"`
	ReadOnly       bool          `json:"readonly"`
	Username       string
	Password       string
	// sensuCli is the sensu cli to use to communicate with this instance of sensu
	SensuCli *sensu_v2_cli.SensuCli
	// lock protects data access while updating
	lock sync.RWMutex

	// fields for storing pulled data
	alertGroups []models.AlertGroup
	colors      models.LabelsColorMap
	knownLabels []string
	silences    map[string]models.Silence
	events      []sensu_v2.Event
	lastError   string
	status      models.SensuStatus

	eventLimit int
	namespaces []string
}

func getSensuClient(url string, timeout time.Duration) *sensu_v2_cli.SensuCli {
	conf := sensu_v2_basic.Config{
		Cluster: sensu_v2_basic.Cluster{
			APIUrl:  url,
			Timeout: timeout,
		},
		Profile: sensu_v2_basic.Profile{
			Format:    "json",
			Namespace: sensuAllNamespaces,
		},
	}

	logger := logrus.WithFields(logrus.Fields{
		"component": "cli-client",
	})
	sensuCliClient := sensu_v2_client.New(&conf)

	return &sensu_v2_cli.SensuCli{
		Client: sensuCliClient,
		Config: &conf,
		Logger: logger,
	}
}

func (s *Sensu) Pull() error {
	err := s.ensureCredentials()
	if err != nil {
		log.Errorf("failed to ensure credentials: %s", err.Error())
		return err
	}

	err = s.pullAlerts()
	if err != nil {
		log.Errorf("failed to pull sensu alerts: %s", err.Error())
		return err
	}

	return nil
}

func (s *Sensu) pullAlerts() error {
	var groups []models.AlertGroup

	start := time.Now()

	// Fetch events from API
	var header http.Header

	s.events = make([]sensu_v2.Event, 0)
	for _, namespace := range s.namespaces {
		events := make([]sensu_v2.Event, 0)
		listOptions := &sensu_v2_client.ListOptions{}
		if s.eventLimit > 0 {
			listOptions.ChunkSize = s.eventLimit
		}
		err := s.SensuCli.Client.List(sensu_v2_client.EventsPath(namespace), &events, listOptions, &header)
		if err != nil {
			log.Errorf("error listing events: %s", err)
			return err
		}
		s.events = append(s.events, events...)
	}

	s.filterHealthyEvents()
	log.Infof("[%s] Got %d sensu unhealthy event(s) in %s", s.Name, len(s.events), time.Since(start))
	knownLabelsMap := map[string]bool{}

	for _, event := range s.events {
		startsAt := event.Check.Issued
		if event.Check.LastOK != 0 {
			startsAt = event.Check.LastOK
		}
		alert := models.Alert{
			Annotations: models.AnnotationsFromMap(map[string]string{
				"description": event.Check.Output,
				"summary":     fmt.Sprintf("failing: %s", event.Check.GetName()),
			}),
			InhibitedBy: []string{},
			Labels:      event.Entity.GetObjectMeta().Labels,
			StartsAt:    time.Unix(startsAt, 0),
			State:       event.Check.GetState(),
			Sensu: []models.SensuInstance{
				{
					Name:        s.Name,
					Cluster:     s.ClusterID(),
					State:       event.Check.GetState(),
					StartsAt:    time.Unix(startsAt, 0),
					Silences:    map[string]*models.Silence{},
					SilencedBy:  []string{},
					InhibitedBy: []string{},
				},
			},
			SilencedBy: []string{},
		}
		alert.UpdateFingerprints()

		labels := event.Entity.ObjectMeta.Labels
		if labels == nil {
			labels = map[string]string{}
		}
		labels["check.name"] = event.Check.GetName()
		labels["namespace"] = event.Entity.GetNamespace()
		labels["entity.name"] = event.Entity.GetName()
		for labelKey := range labels {
			knownLabelsMap[labelKey] = true
		}

		id, err := slices.StringSliceToSHA1([]string{event.Entity.GetName(), event.Entity.GetNamespace(), event.Check.GetName(), event.Check.GetNamespace()})
		if err != nil {
			log.Errorf("error generating id from event.entity.metadata + event.check.metadata: %s", err)
			return err
		}
		group := models.AlertGroup{
			ID:       id,
			Receiver: "fake",
			Labels:   labels,
			Alerts: models.AlertList{
				alert,
			},
			AlertmanagerCount: map[string]int{
				s.Name: 1,
			},
			LatestStartsAt: time.Unix(event.Check.Issued, 0),
		}
		group.ContentFingerprint()
		// log.Warnf("got alert group: %+v", group)
		groups = append(groups, group)
	}

	s.lock.Lock()
	s.alertGroups = groups
	s.colors = models.LabelsColorMap{}
	knownLabels := []string{}
	for key := range knownLabelsMap {
		knownLabels = append(knownLabels, key)
	}
	s.silences = map[string]models.Silence{}
	s.lock.Unlock()

	return nil
}

// Alerts returns a copy of all alert groups
func (s *Sensu) Alerts() []models.AlertGroup {
	s.lock.RLock()
	defer s.lock.RUnlock()

	alerts := make([]models.AlertGroup, len(s.alertGroups))
	copy(alerts, s.alertGroups)
	return alerts
}

// Colors returns a copy of all color maps
func (s *Sensu) Colors() models.LabelsColorMap {
	s.lock.RLock()
	defer s.lock.RUnlock()

	colors := models.LabelsColorMap{}
	for k, v := range s.colors {
		colors[k] = map[string]models.LabelColors{}
		for nk, nv := range v {
			colors[k][nk] = nv
		}
	}
	return colors
}

// ClusterID returns the ID (sha1) of the cluster this Sensu instance
// belongs to
func (s *Sensu) ClusterID() string {
	members := []string{s.URI}
	id, err := slices.StringSliceToSHA1(members)
	if err != nil {
		log.Errorf("slices.StringSliceToSHA1 error: %s", err)
		return s.Name
	}
	return id
}

func (s *Sensu) filterHealthyEvents() {
	unhealthyEvents := make([]sensu_v2.Event, 0)
	for _, event := range s.events {
		if event.Check.GetStatus() != 0 {
			unhealthyEvents = append(unhealthyEvents, event)
		}
	}
	s.events = unhealthyEvents
}

func (s *Sensu) ensureCredentials() (err error) {
	var (
		tokens *sensu_v2_types.Tokens
	)

	currentTokens := s.SensuCli.Config.Tokens()
	if currentTokens == nil || currentTokens.Access == "" {

		c1 := make(chan sensu_v2_types.Tokens, 1)
		go func() {
			var tokens *sensu_v2_types.Tokens
			if tokens, err = s.SensuCli.Client.CreateAccessToken(s.URI, s.Username, s.Password); err != nil {
				s.SensuCli.Logger.Errorf("create token err: %+v", err)
				return
			}
			c1 <- *tokens
		}()

		select {
		case response := <-c1:
			tokens = &response
		case <-time.After(s.RequestTimeout):
			s.SensuCli.Logger.Warnf("timeout from sensu server %s after 10 seconds", s.URI)
		}

		if tokens == nil {
			return fmt.Errorf("failed to retrieve new access token from sensu server dump: %s", spew.Sdump(s))
		}

		conf := sensu_v2_basic.Config{
			Cluster: sensu_v2_basic.Cluster{
				APIUrl:  s.URI,
				Tokens:  tokens,
				Timeout: s.RequestTimeout,
			},
			Profile: sensu_v2_basic.Profile{
				Format:    "json",
				Namespace: sensuAllNamespaces,
			},
		}

		sensuCliClient := sensu_v2_client.New(&conf)

		logger := logrus.WithFields(logrus.Fields{
			"component": "cli-client",
		})

		s.SensuCli = &sensu_v2_cli.SensuCli{
			Client: sensuCliClient,
			Config: &conf,
			Logger: logger,
		}
	}
	return nil
}

// KnownLabels returns a copy of a map with known labels
func (s *Sensu) KnownLabels() []string {
	s.lock.RLock()
	defer s.lock.RUnlock()

	knownLabels := make([]string, len(s.knownLabels))
	copy(knownLabels, s.knownLabels)

	return knownLabels
}
