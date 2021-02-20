import React, { useState, useEffect } from "react";
import { connect } from "react-redux";
import "react-dom";

import { withShortcut } from "react-keybind";
import Modal from "react-modal";

import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import {
  faFileAlt,
  faKeyboard,
  faColumns,
} from "@fortawesome/free-solid-svg-icons";
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

const initialState = {
  shortcutsModalOpen: false,
};

function Sidebar(props) {
  const { areNamesLoading, isJSONLoading, labels, shortcut } = props;

  const [state, setState] = useState(initialState);

  const showShortcutsModal = () => {
    setState({ shortcutsModalOpen: true });
  };

  const closeShortcutsModal = () => {
    setState({ shortcutsModalOpen: false });
  };

  useEffect(() => {
    shortcut.registerShortcut(
      showShortcutsModal,
      ["shift+?"],
      "Shortcuts",
      "Show Keyboard Shortcuts Modal"
    );
  }, []);

  return (
    <div className="sidebar">
      <span
        className="logo active"
        onClick={() => {
          history.push({
            pathname: "/",
            search: history.location.search,
          });
        }}
      />
      <SidebarItem tooltipText="Comparison View - Coming Soon">
        <button
          type="button"
          onClick={() => {
            history.push({
              pathname: '/comparison',
              search: history.location.search,
            });
          }}
        >
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
      <Modal
        isOpen={state.shortcutsModalOpen}
        style={modalStyle}
        appElement={document.getElementById("root")}
      >
        <div className="modal-close-btn" onClick={closeShortcutsModal} />
        <ShortcutsModal closeModal={closeShortcutsModal} />
      </Modal>
    </div>
  );
}

export default connect((x) => x, { fetchNames })(withShortcut(Sidebar));
