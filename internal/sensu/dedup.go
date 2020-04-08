package sensu

import (
	"log"
	"sort"

	"github.com/prymitive/karma/internal/config"
	"github.com/prymitive/karma/internal/models"
	"github.com/prymitive/karma/internal/slices"
	"github.com/prymitive/karma/internal/transform"
)

// DedupAlerts will collect alert groups from all defined Sensu
// upstreams and deduplicate them, so we only return unique alerts
func DedupAlerts() []models.AlertGroup {
	uniqueGroups := map[string][]models.AlertGroup{}

	upstreams := GetSensus()
	for _, sensu := range upstreams {
		groups := sensu.Alerts()
		log.Printf("got alertgroups %+v in DedupAlerts", groups)
		for _, ag := range groups {
			if _, found := uniqueGroups[ag.ID]; !found {
				uniqueGroups[ag.ID] = []models.AlertGroup{}
			}
			uniqueGroups[ag.ID] = append(uniqueGroups[ag.ID], ag)
		}
	}

	dedupedGroups := []models.AlertGroup{}
	alertStates := map[string][]string{}
	for _, agList := range uniqueGroups {
		alerts := map[string]models.Alert{}
		for _, ag := range agList {
			for _, alert := range ag.Alerts {
				alert := alert // scopelint pin
				// remove all alerts for receiver(s) that the user doesn't
				// want to see in the UI
				if transform.StripReceivers(config.Config.Receivers.Keep, config.Config.Receivers.Strip, alert.Receiver) {
					continue
				}
				alertLFP := alert.LabelsFingerprint()
				a, found := alerts[alertLFP]
				if found {
					// if we already have an alert with the same fp then just append
					// alertmanager instances to it, this way we end up with all instances
					// for each unique alert merged into a single alert with all
					// alertmanager instances attached to it
					a.Alertmanager = append(a.Alertmanager, alert.Alertmanager...)
					// set startsAt to the earliest value we have
					if alert.StartsAt.Before(a.StartsAt) {
						a.StartsAt = alert.StartsAt
					}
					// update map
					alerts[alertLFP] = a
					// and append alert state to the slice
					alertStates[alertLFP] = append(alertStates[alertLFP], alert.State)
				} else {
					alerts[alertLFP] = models.Alert(alert)
					// seed alert state slice
					alertStates[alertLFP] = []string{alert.State}
				}
			}
		}
		// skip empty groups
		if len(alerts) == 0 {
			continue
		}
		ag := models.AlertGroup(agList[0])
		ag.Alerts = models.AlertList{}
		for _, alert := range alerts {
			log.Printf("got alert.Labels %s while looping through alerts in DedupAlerts", alert.Labels)
			alert := alert // scopelint pin
			// strip labels and annotations user doesn't want to see in the UI
			alert.Labels = transform.StripLables(config.Config.Labels.Keep, config.Config.Labels.Strip, alert.Labels)
			alert.Annotations = transform.StripAnnotations(config.Config.Annotations.Keep, config.Config.Annotations.Strip, alert.Annotations)
			// calculate final alert state based on the most important value found
			// in the list of states from all instances
			alertLFP := alert.LabelsFingerprint()
			if slices.StringInSlice(alertStates[alertLFP], models.AlertStateActive) {
				alert.State = models.AlertStateActive
			} else if slices.StringInSlice(alertStates[alertLFP], models.AlertStateSuppressed) {
				alert.State = models.AlertStateSuppressed
			} else {
				alert.State = models.AlertStateUnprocessed
			}
			// sort Alertmanager instances for every alert
			sort.Slice(alert.Alertmanager, func(i, j int) bool {
				return alert.Alertmanager[i].Name < alert.Alertmanager[j].Name
			})
			ag.Alerts = append(ag.Alerts, alert)
		}
		ag.Hash = ag.ContentFingerprint()
		dedupedGroups = append(dedupedGroups, ag)
	}

	return dedupedGroups
}

// DedupColors returns a color map merged from all Sensu upstream color
// maps
func DedupColors() models.LabelsColorMap {
	dedupedColors := models.LabelsColorMap{}

	upstreams := GetSensus()

	for _, sensu := range upstreams {
		colors := sensu.Colors()
		// map[string]map[string]LabelColors
		for labelName, valueMap := range colors {
			if _, found := dedupedColors[labelName]; !found {
				dedupedColors[labelName] = map[string]models.LabelColors{}
			}
			for labelVal, labelColors := range valueMap {
				if _, found := dedupedColors[labelName][labelVal]; !found {
					dedupedColors[labelName][labelVal] = labelColors
				}
			}
		}
	}

	return dedupedColors
}

// DedupKnownLabels returns a deduplicated slice of all known label names
func DedupKnownLabels() []string {
	dedupedLabels := map[string]bool{}
	upstreams := GetSensus()

	for _, sensu := range upstreams {
		for _, key := range sensu.KnownLabels() {
			dedupedLabels[key] = true
		}
	}

	flatLabels := []string{}
	for key := range dedupedLabels {
		flatLabels = append(flatLabels, key)
	}
	return flatLabels
}
