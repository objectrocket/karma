import React, { Component } from "react";
import PropTypes from "prop-types";

import { observer } from "mobx-react";

import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { faVolumeMute } from "@fortawesome/free-solid-svg-icons/faVolumeMute";

import { APIAlert, APIGroup } from "Models/API";
import { AlertStore } from "Stores/AlertStore";
import { SilenceFormStore } from "Stores/SilenceFormStore";
import { BorderClassMap } from "Common/Colors";
import { StaticLabels } from "Common/Query";
import { FilteringLabel } from "Components/Labels/FilteringLabel";
import { TooltipWrapper } from "Components/TooltipWrapper";
import { RenderNonLinkAnnotation, RenderLinkAnnotation } from "../Annotation";
import { AlertMenu } from "./AlertMenu";
import { RenderSilence } from "../Silences";

const Alert = observer(
  class Alert extends Component {
    static propTypes = {
      group: APIGroup.isRequired,
      alert: APIAlert.isRequired,
      showAlertmanagers: PropTypes.bool.isRequired,
      showReceiver: PropTypes.bool.isRequired,
      afterUpdate: PropTypes.func.isRequired,
      alertStore: PropTypes.instanceOf(AlertStore).isRequired,
      silenceFormStore: PropTypes.instanceOf(SilenceFormStore).isRequired,
      setIsMenuOpen: PropTypes.func.isRequired,
    };

    render() {
      const {
        group,
        alert,
        showAlertmanagers,
        showReceiver,
        afterUpdate,
        alertStore,
        silenceFormStore,
        setIsMenuOpen,
      } = this.props;

      let classNames = [
        "components-grid-alertgrid-alertgroup-alert",
        "list-group-item bg-transparent",
        "pl-1 pr-0 py-0",
        "my-1",
        "rounded-0",
        "border-left-1 border-right-0 border-top-0 border-bottom-0",
        BorderClassMap[alert.state] || "border-default",
      ];

      const silences = {};
      var upstream = null;
      if (alert.alertmanager != null) {
        upstream = alert.alertmanager
      } else if (alert.sensu != null) {
        upstream = alert.sensu
      }
      if (upstream == null) {
        return
      }
      for (const us of upstream) {
        if (!silences[us.cluster]) {
          silences[us.cluster] = {
            alertmanager: us,
            silences: [
              ...new Set(
                us.silencedBy.filter(
                  (silenceID) =>
                    !(
                      group.shared.silences[us.cluster] &&
                      group.shared.silences[us.cluster].includes(silenceID)
                    )
                )
              ),
            ],
          };
        }
      }

      return (
        <li
          className={classNames.join(" ")}
          onMouseEnter={this.toggleHover}
          onMouseLeave={this.toggleHover}
        >
          <div>
            {alert.annotations
              .filter((a) => a.isLink === false)
              .map((a) => (
                <RenderNonLinkAnnotation
                  key={a.name}
                  name={a.name}
                  value={a.value}
                  visible={a.visible}
                  afterUpdate={afterUpdate}
                  alertStore={alertStore}
                />
              ))}
          </div>
          <AlertMenu
            group={group}
            alert={alert}
            alertStore={alertStore}
            silenceFormStore={silenceFormStore}
            setIsMenuOpen={setIsMenuOpen}
          />
          {upstream
            .map((us) => us.inhibitedBy.length)
            .reduce((sum, x) => sum + x) > 0 ? (
              <TooltipWrapper title="This alert is inhibited by other alerts">
                <span className="badge badge-light components-label">
                  <FontAwesomeIcon className="text-success" icon={faVolumeMute} />
                </span>
              </TooltipWrapper>
            ) : null}
          {Object.entries(alert.labels).map(([name, value]) => (
            <FilteringLabel
              key={name}
              name={name}
              value={value}
              alertStore={alertStore}
            />
          ))}
          {showAlertmanagers
            ? upstream.map(us => (
              <FilteringLabel
                key={us.name}
                name={StaticLabels.AlertManager}
                value={us.name}
                alertStore={alertStore}
              />
            ))
            : null}
          {showReceiver ? (
            <FilteringLabel
              name={StaticLabels.Receiver}
              value={alert.receiver}
              alertStore={alertStore}
            />
          ) : null}
          {alert.annotations
            .filter((a) => a.isLink === true)
            .map((a) => (
              <RenderLinkAnnotation
                key={a.name}
                name={a.name}
                value={a.value}
              />
            ))}
          {Object.entries(silences).map(([cluster, clusterSilences]) =>
            clusterSilences.silences.map((silenceID) =>
              RenderSilence(
                alertStore,
                silenceFormStore,
                afterUpdate,
                cluster,
                silenceID
              )
            )
          )}
        </li>
      );
    }
  }
);

export { Alert };
