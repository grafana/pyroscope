import React, { useState, useEffect } from "react";
import { connect } from "react-redux";
import "react-dom";

import { withShortcut } from "react-keybind";
import Modal from "react-modal";
import clsx from "clsx";

import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import {
  faFileAlt,
  faKeyboard,
  faColumns,
  faBell,
  faSignOutAlt,
  faChartBar,
} from "@fortawesome/free-solid-svg-icons";
import { faWindowMaximize } from "@fortawesome/free-regular-svg-icons";
import { faGithub } from "@fortawesome/free-brands-svg-icons";
import ShortcutsModal from "./ShortcutsModal";
import SlackIcon from "./SlackIcon";

import { fetchNames } from "../redux/actions";
import history from "../util/history";

const modalStyle = {
  overlay: {
    backgroundColor: "rgba(0,0,0,0.75)",
  },
  content: {
    background: "#222",
    border: "1px solid #111",
  },
};

function SidebarItem(props) {
  const { children, tooltipText } = props;
  return (
    <div className="sidebar-item">
      {children}
      <div className="sidebar-tooltip-wrapper">
        {tooltipText && <span className="sidebar-tooltip">{tooltipText}</span>}
      </div>
    </div>
  );
}

function signOut() {
  const form = document.createElement("form");

  form.method = "POST";
  form.action = "/logout";

  document.body.appendChild(form);

  form.submit();
}

const initialState = {
  shortcutsModalOpen: false,
  currentRoute: "/",
};

function Sidebar(props) {
  const { shortcut } = props;

  const [state, setState] = useState(initialState);

  const showShortcutsModal = () => {
    setState({ shortcutsModalOpen: true });
  };

  const closeShortcutsModal = () => {
    setState({ shortcutsModalOpen: false });
  };

  const updateRoute = (newRoute) => {
    history.push({
      pathname: newRoute,
      search: history.location.search,
    });

    setState({ currentRoute: newRoute });
  };

  useEffect(() => {
    shortcut.registerShortcut(
      showShortcutsModal,
      ["shift+?"],
      "Shortcuts",
      "Show Keyboard Shortcuts Modal"
    );

    // console.log('history: ', history.location.pathname);
    setState({ currentRoute: history.location.pathname });
  }, []);

  return (
    <div className="sidebar">
      <span className="logo" onClick={() => updateRoute("/")} />
      <SidebarItem tooltipText="Single View">
        <button
          className={clsx({ "active-route": state.currentRoute === "/" })}
          type="button"
          data-testid="sidebar-root"
          onClick={() => updateRoute("/")}
        >
          <FontAwesomeIcon icon={faWindowMaximize} />
        </button>
      </SidebarItem>
      <SidebarItem tooltipText="Comparison View">
        <button
          className={clsx({
            "active-route": state.currentRoute === "/comparison",
          })}
          data-testid="sidebar-comparison"
          type="button"
          onClick={() => updateRoute("/comparison")}
        >
          <FontAwesomeIcon icon={faColumns} />
        </button>
      </SidebarItem>
      <SidebarItem tooltipText="Diff View">
        <button
          className={clsx({
            "active-route": state.currentRoute === "/comparison-diff",
          })}
          type="button"
          data-testid="sidebar-comparison-diff"
          onClick={() => updateRoute("/comparison-diff")}
        >
          <FontAwesomeIcon icon={faChartBar} />
        </button>
      </SidebarItem>
      <SidebarItem tooltipText="Alerts - Coming Soon">
        <button type="button">
          <FontAwesomeIcon icon={faBell} />
        </button>
      </SidebarItem>
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
        <button
          onClick={showShortcutsModal}
          type="button"
          id="tests-shortcuts-btn"
        >
          <FontAwesomeIcon icon={faKeyboard} />
        </button>
      </SidebarItem>
      <SidebarItem tooltipText="Sign Out">
        <button type="button" onClick={() => signOut()}>
          <FontAwesomeIcon icon={faSignOutAlt} />
        </button>
      </SidebarItem>
      <Modal
        isOpen={state.shortcutsModalOpen}
        style={modalStyle}
        appElement={document.getElementById("root")}
        ariaHideApp={false}
      >
        <div className="modal-close-btn" onClick={closeShortcutsModal} />
        <ShortcutsModal closeModal={closeShortcutsModal} />
      </Modal>
    </div>
  );
}

export default connect((x) => x, { fetchNames })(withShortcut(Sidebar));
