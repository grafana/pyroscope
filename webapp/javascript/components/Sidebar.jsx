import React from "react";
import { connect } from "react-redux";
import "react-dom";

import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import {
  faFileAlt,
  faKeyboard,
  faColumns,
} from "@fortawesome/free-solid-svg-icons";
import { faGithub } from "@fortawesome/free-brands-svg-icons";
import SlackIcon from "./SlackIcon";

import { fetchNames } from "../redux/actions";

function SidebarItem(props) {
  const { children, tooltipText } = props;
  return (
    <div className="sidebar-item">
      {children}
      {
        // <div className="sidebar-external-link">
        //   <FontAwesomeIcon icon={faExternalLinkSquareAlt} />
        // </div>
      }
      <div className="sidebar-tooltip-wrapper">
        <span className="sidebar-tooltip">{tooltipText}</span>
      </div>
    </div>
  );
}

function Sidebar(props) {
  const { showShortcutsModal } = props;
  return (
    <div className="sidebar">
      <h1 className="logo active" />
      <SidebarItem tooltipText="Comparison View - Coming Soon">
        <button type="button">
          <FontAwesomeIcon icon={faColumns} />
        </button>
      </SidebarItem>
      {/* <SidebarItem tooltipText="Alerts - Coming Soon">
        <button>
          <FontAwesomeIcon icon={faBell} />
        </button>
      </SidebarItem> */}
      <div className="sidebar-space-filler" />
      <SidebarItem tooltipText="Docs" externalLink>
        <a rel="noreferrer" target="_blank" href="https://pyroscope.io/docs">
          <FontAwesomeIcon icon={faFileAlt} />
        </a>
      </SidebarItem>
      <SidebarItem tooltipText="Slack" externalLink>
        <a rel="noreferrer" target="_blank" href="https://pyroscope.io/slack">
          <SlackIcon />
        </a>
      </SidebarItem>
      <SidebarItem tooltipText="GitHub" externalLink>
        <a
          rel="noreferrer"
          target="_blank"
          href="https://github.com/pyroscope-io/pyroscope"
        >
          <FontAwesomeIcon icon={faGithub} />
        </a>
      </SidebarItem>
      <SidebarItem tooltipText="Keyboard Shortcuts">
        <button onClick={showShortcutsModal} type="button">
          <FontAwesomeIcon icon={faKeyboard} />
        </button>
      </SidebarItem>
    </div>
  );
}

export default connect((x) => x, { fetchNames })(Sidebar);
